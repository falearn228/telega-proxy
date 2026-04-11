package ui

import (
	"fmt"
	"io"
	"net/url"
	"runtime"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"github.com/skip2/go-qrcode"

	"github.com/falearn/tg-fyne-proxy/internal/config"
	"github.com/falearn/tg-fyne-proxy/internal/core"
	"github.com/falearn/tg-fyne-proxy/internal/platform"
)

const (
	prefSelectedTab    = "ui.selected_tab"
	prefWarnBackground = "ui.warn_background"
)

type App struct {
	fyneApp    fyne.App
	window     fyne.Window
	controller *core.Controller
	mobileMode bool

	hostEntry      *widget.Entry
	portEntry      *widget.Entry
	secretEntry    *widget.Entry
	dcEntry        *widget.Entry
	bufferEntry    *widget.Entry
	poolEntry      *widget.Entry
	timeoutEntry   *widget.Entry
	verboseCheck   *widget.Check
	wsCheck        *widget.Check
	autostartCheck *widget.Check

	statusLabel *widget.Label
	statsLabel  *widget.Label
	linkEntry   *widget.Entry
	logEntry    *widget.Entry
	logScroll   *container.Scroll
	infoText    *widget.RichText
	tabs        *container.AppTabs

	startButton *widget.Button
	stopButton  *widget.Button
	proxyBusy   bool
	quitting    bool
}

func NewApp(controller *core.Controller) *App {
	fyneApp := app.NewWithID("github.com/falearn/tg-fyne-proxy")
	window := fyneApp.NewWindow("TG Fyne Proxy")

	mobileMode := fyne.CurrentDevice().IsMobile()
	if mobileMode {
		window.Resize(fyne.NewSize(420, 800))
	} else {
		window.Resize(fyne.NewSize(920, 720))
	}

	a := &App{
		fyneApp:    fyneApp,
		window:     window,
		controller: controller,
		mobileMode: mobileMode,
	}
	a.buildUI()
	a.setupLifecycle()
	return a
}

func (a *App) Run() {
	a.startRefreshLoop()
	a.window.ShowAndRun()
}

func (a *App) buildUI() {
	snapshot := a.controller.Snapshot()
	cfg := snapshot.Config

	a.hostEntry = widget.NewEntry()
	a.hostEntry.SetText(cfg.Host)

	a.portEntry = widget.NewEntry()
	a.portEntry.SetText(strconv.Itoa(cfg.Port))

	a.secretEntry = widget.NewEntry()
	a.secretEntry.SetText(cfg.Secret)

	a.dcEntry = widget.NewMultiLineEntry()
	a.dcEntry.SetText(config.FormatDCMap(cfg.DCMap))

	a.bufferEntry = widget.NewEntry()
	a.bufferEntry.SetText(strconv.Itoa(cfg.BufferKB))

	a.poolEntry = widget.NewEntry()
	a.poolEntry.SetText(strconv.Itoa(cfg.PoolSize))

	a.timeoutEntry = widget.NewEntry()
	a.timeoutEntry.SetText(strconv.Itoa(cfg.ConnectTimout))

	a.verboseCheck = widget.NewCheck("Verbose logging", nil)
	a.verboseCheck.SetChecked(cfg.Verbose)

	a.wsCheck = widget.NewCheck("Connect via Telegram WebSocket", nil)
	a.wsCheck.SetChecked(cfg.ConnectViaWS)
	if platform.SupportsAutostart() {
		a.autostartCheck = widget.NewCheck("Start with Windows", nil)
		a.autostartCheck.SetChecked(cfg.Autostart)
	}

	a.statusLabel = widget.NewLabel("")
	a.statsLabel = widget.NewLabel("")
	a.linkEntry = widget.NewMultiLineEntry()
	a.linkEntry.Disable()
	a.linkEntry.Wrapping = fyne.TextWrapWord
	a.logEntry = widget.NewMultiLineEntry()
	a.logEntry.Disable()
	a.logEntry.Wrapping = fyne.TextWrapWord

	infoMarkdown := "MTProto-only local proxy with `telegram-wss` and TCP fallback."
	if a.mobileMode {
		infoMarkdown += "\n\nAndroid note: without native foreground-service glue the OS may suspend background traffic."
	}
	a.infoText = widget.NewRichTextFromMarkdown(infoMarkdown)

	formItems := []*widget.FormItem{
		widget.NewFormItem("Host", a.hostEntry),
		widget.NewFormItem("Port", a.portEntry),
		widget.NewFormItem("Secret", a.secretEntry),
		widget.NewFormItem("DC map", a.dcEntry),
		widget.NewFormItem("Buffer KB", a.bufferEntry),
		widget.NewFormItem("WS pool size", a.poolEntry),
		widget.NewFormItem("Connect timeout, sec", a.timeoutEntry),
		widget.NewFormItem("", a.verboseCheck),
		widget.NewFormItem("", a.wsCheck),
	}
	if a.autostartCheck != nil {
		formItems = append(formItems, widget.NewFormItem("", a.autostartCheck))
	}
	form := widget.NewForm(formItems...)

	saveButton := widget.NewButton("Save", func() {
		cfg, err := a.readForm()
		if err != nil {
			a.showError(err)
			return
		}
		if err := a.controller.Apply(cfg, true); err != nil {
			a.showError(err)
			return
		}
		a.refresh()
	})

	a.startButton = widget.NewButton("Start proxy", func() {
		a.startProxy()
	})

	a.stopButton = widget.NewButton("Stop proxy", func() {
		a.stopProxy()
	})

	clearLogsButton := widget.NewButton("Clear logs", func() {
		a.controller.ClearLogs()
		a.refresh()
	})

	openInTelegramButton := widget.NewButton("Open in Telegram", func() {
		snapshot := a.controller.Snapshot()
		u, err := url.Parse(snapshot.TGLink)
		if err != nil {
			a.showError(err)
			return
		}
		if err := a.fyneApp.OpenURL(u); err != nil {
			a.showError(err)
		}
	})

	var moreButton *widget.Button
	moreButton = widget.NewButton("More", func() {
		menu := fyne.NewMenu("",
			fyne.NewMenuItem("Import config", a.importConfig),
			fyne.NewMenuItem("Export config", a.exportConfig),
			fyne.NewMenuItem("New secret", func() {
				a.secretEntry.SetText(config.MustGenerateSecret())
			}),
			fyne.NewMenuItem("Copy tg:// link", func() {
				snapshot := a.controller.Snapshot()
				a.window.Clipboard().SetContent(snapshot.TGLink)
			}),
			fyne.NewMenuItem("Show QR", a.showQR),
		)
		widget.ShowPopUpMenuAtRelativePosition(
			menu,
			a.window.Canvas(),
			fyne.NewPos(0, moreButton.Size().Height),
			moreButton,
		)
	})

	actionRow1 := container.NewGridWithColumns(2, saveButton, a.startButton)
	actionRow2 := container.NewGridWithColumns(2, a.stopButton, openInTelegramButton)
	actionRow3 := container.NewGridWithColumns(1, moreButton)

	var actionPanel fyne.CanvasObject
	if a.mobileMode {
		actionPanel = container.NewVBox(actionRow1, actionRow2, actionRow3)
	} else {
		actionPanel = container.NewVBox(
			container.NewHBox(saveButton, a.startButton, a.stopButton, openInTelegramButton, layout.NewSpacer(), moreButton),
		)
	}

	configPane := container.NewBorder(
		container.NewVBox(a.infoText, widget.NewSeparator()),
		actionPanel,
		nil,
		nil,
		container.NewVScroll(form),
	)

	statusPane := container.NewVScroll(container.NewVBox(
		widget.NewLabel("Status"),
		a.statusLabel,
		widget.NewSeparator(),
		widget.NewLabel("Telegram link"),
		a.linkEntry,
		widget.NewSeparator(),
		widget.NewLabel("Stats"),
		a.statsLabel,
	))

	a.logScroll = container.NewVScroll(a.logEntry)
	logPane := container.NewBorder(
		container.NewHBox(widget.NewLabel("Log"), layout.NewSpacer(), clearLogsButton),
		nil,
		nil,
		nil,
		a.logScroll,
	)

	if a.mobileMode {
		a.tabs = container.NewAppTabs(
			container.NewTabItem("Config", configPane),
			container.NewTabItem("Status", statusPane),
			container.NewTabItem("Log", logPane),
		)
		a.tabs.SetTabLocation(container.TabLocationBottom)
		a.tabs.OnSelected = func(_ *container.TabItem) {
			a.fyneApp.Preferences().SetInt(prefSelectedTab, a.tabs.SelectedIndex())
		}
		index := a.fyneApp.Preferences().IntWithFallback(prefSelectedTab, 0)
		a.tabs.SelectIndex(index)
		a.window.SetContent(a.tabs)
	} else {
		rightTabs := container.NewAppTabs(
			container.NewTabItem("Status", statusPane),
			container.NewTabItem("Log", logPane),
		)
		rightTabs.OnSelected = func(_ *container.TabItem) {
			a.fyneApp.Preferences().SetInt(prefSelectedTab, rightTabs.SelectedIndex())
		}
		index := a.fyneApp.Preferences().IntWithFallback(prefSelectedTab, 0)
		rightTabs.SelectIndex(index)
		content := container.NewHSplit(configPane, rightTabs)
		content.SetOffset(0.48)
		a.window.SetContent(content)
		a.tabs = rightTabs
	}

	a.refresh()
}

func (a *App) setupLifecycle() {
	a.setupTray()
	lifecycle := a.fyneApp.Lifecycle()

	if !a.mobileMode && (runtime.GOOS == "windows" || runtime.GOOS == "linux") {
		a.window.SetCloseIntercept(func() {
			if a.quitting {
				return
			}
			a.window.Hide()
		})
	}

	lifecycle.SetOnStarted(func() {
		fyne.Do(a.refresh)
	})

	lifecycle.SetOnEnteredForeground(func() {
		fyne.Do(a.refresh)
	})

	lifecycle.SetOnExitedForeground(func() {
		if !a.mobileMode {
			return
		}
		if !a.controller.Snapshot().Running {
			return
		}
		if !a.fyneApp.Preferences().BoolWithFallback(prefWarnBackground, true) {
			return
		}
		a.fyneApp.SendNotification(fyne.NewNotification(
			"TG Fyne Proxy",
			"App moved to background. Android may suspend proxy traffic without a native foreground service.",
		))
	})

	lifecycle.SetOnStopped(func() {
		if a.tabs != nil {
			a.fyneApp.Preferences().SetInt(prefSelectedTab, a.tabs.SelectedIndex())
		}
	})
}

func (a *App) setupTray() {
	if a.mobileMode || (runtime.GOOS != "windows" && runtime.GOOS != "linux") {
		return
	}
	desktopApp, ok := a.fyneApp.(desktop.App)
	if !ok {
		return
	}

	desktopApp.SetSystemTrayMenu(fyne.NewMenu("TG Fyne Proxy",
		fyne.NewMenuItem("Show", func() {
			a.window.Show()
			a.window.RequestFocus()
			a.refresh()
		}),
		fyne.NewMenuItem("Start proxy", a.startProxy),
		fyne.NewMenuItem("Stop proxy", a.stopProxy),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quit", a.quitApp),
	))
}

func (a *App) startProxy() {
	cfg, err := a.readForm()
	if err != nil {
		a.showError(err)
		return
	}
	a.proxyBusy = true
	a.refresh()
	go func() {
		err := a.controller.Apply(cfg, true)
		if err == nil {
			err = a.controller.Start()
		}
		fyne.Do(func() {
			a.proxyBusy = false
			if err != nil {
				a.showError(err)
			}
			a.refresh()
		})
	}()
}

func (a *App) stopProxy() {
	a.proxyBusy = true
	a.refresh()
	go func() {
		err := a.controller.Stop()
		fyne.Do(func() {
			a.proxyBusy = false
			if err != nil {
				a.showError(err)
			}
			a.refresh()
		})
	}()
}

func (a *App) quitApp() {
	a.quitting = true
	go func() {
		_ = a.controller.Stop()
		fyne.Do(func() {
			a.fyneApp.Quit()
		})
	}()
}

func (a *App) startRefreshLoop() {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			fyne.Do(a.refresh)
		}
	}()
}

func (a *App) refresh() {
	snapshot := a.controller.Snapshot()
	state := "stopped"
	if snapshot.Running {
		state = fmt.Sprintf("running on %s", snapshot.ListenAddr)
	}
	if snapshot.LastError != "" {
		state += "\nlast error: " + snapshot.LastError
	}
	if a.proxyBusy {
		a.startButton.Disable()
		a.stopButton.Disable()
	} else if snapshot.Running {
		a.startButton.Disable()
		a.stopButton.Enable()
	} else {
		a.startButton.Enable()
		a.stopButton.Disable()
	}
	a.statusLabel.SetText(state)
	a.linkEntry.SetText(snapshot.TGLink)
	a.statsLabel.SetText(fmt.Sprintf(
		"total: %d\nactive: %d\nerrors: %d\nws: %d\ntcp: %d\nws errors: %d\nup: %d bytes\ndown: %d bytes",
		snapshot.Stats.ConnectionsTotal,
		snapshot.Stats.ConnectionsActive,
		snapshot.Stats.ConnectionsErrored,
		snapshot.Stats.ConnectionsWS,
		snapshot.Stats.ConnectionsTCP,
		snapshot.Stats.WSErrors,
		snapshot.Stats.BytesUp,
		snapshot.Stats.BytesDown,
	))
	if a.logEntry.Text != snapshot.Logs {
		a.logEntry.SetText(snapshot.Logs)
		if a.logScroll != nil {
			a.logScroll.ScrollToBottom()
		}
	}
}

func (a *App) readForm() (config.AppConfig, error) {
	cfg := config.Default()
	cfg.Host = a.hostEntry.Text

	port, err := strconv.Atoi(a.portEntry.Text)
	if err != nil {
		return cfg, fmt.Errorf("invalid port: %w", err)
	}
	cfg.Port = port
	cfg.Secret = a.secretEntry.Text

	dcMap, err := config.ParseDCMap(a.dcEntry.Text)
	if err != nil {
		return cfg, err
	}
	cfg.DCMap = dcMap

	bufferKB, err := strconv.Atoi(a.bufferEntry.Text)
	if err != nil {
		return cfg, fmt.Errorf("invalid buffer size: %w", err)
	}
	cfg.BufferKB = bufferKB

	poolSize, err := strconv.Atoi(a.poolEntry.Text)
	if err != nil {
		return cfg, fmt.Errorf("invalid pool size: %w", err)
	}
	cfg.PoolSize = poolSize

	timeoutSec, err := strconv.Atoi(a.timeoutEntry.Text)
	if err != nil {
		return cfg, fmt.Errorf("invalid timeout: %w", err)
	}
	cfg.ConnectTimout = timeoutSec
	cfg.Verbose = a.verboseCheck.Checked
	cfg.ConnectViaWS = a.wsCheck.Checked
	if a.autostartCheck != nil {
		cfg.Autostart = a.autostartCheck.Checked
	}
	cfg.PreferIPv6 = false
	return cfg, cfg.Validate()
}

func (a *App) populateForm(cfg config.AppConfig) {
	a.hostEntry.SetText(cfg.Host)
	a.portEntry.SetText(strconv.Itoa(cfg.Port))
	a.secretEntry.SetText(cfg.Secret)
	a.dcEntry.SetText(config.FormatDCMap(cfg.DCMap))
	a.bufferEntry.SetText(strconv.Itoa(cfg.BufferKB))
	a.poolEntry.SetText(strconv.Itoa(cfg.PoolSize))
	a.timeoutEntry.SetText(strconv.Itoa(cfg.ConnectTimout))
	a.verboseCheck.SetChecked(cfg.Verbose)
	a.wsCheck.SetChecked(cfg.ConnectViaWS)
	if a.autostartCheck != nil {
		a.autostartCheck.SetChecked(cfg.Autostart)
	}
}

func (a *App) exportConfig() {
	saveDialog := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil {
			a.showError(err)
			return
		}
		if w == nil {
			return
		}
		defer w.Close()

		cfg, err := a.readForm()
		if err != nil {
			a.showError(err)
			return
		}
		data, err := config.Marshal(cfg)
		if err != nil {
			a.showError(err)
			return
		}
		if _, err := w.Write(data); err != nil {
			a.showError(err)
		}
	}, a.window)
	saveDialog.SetFileName("tg-fyne-proxy-config.json")
	saveDialog.Show()
}

func (a *App) importConfig() {
	openDialog := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil {
			a.showError(err)
			return
		}
		if r == nil {
			return
		}
		defer r.Close()

		data, err := io.ReadAll(r)
		if err != nil {
			a.showError(err)
			return
		}
		cfg, err := config.Parse(data)
		if err != nil {
			a.showError(err)
			return
		}
		a.populateForm(cfg)
		if err := a.controller.Apply(cfg, true); err != nil {
			a.showError(err)
			return
		}
		a.refresh()
	}, a.window)
	openDialog.SetFilter(storage.NewExtensionFileFilter([]string{".json"}))
	openDialog.Show()
}

func (a *App) showQR() {
	snapshot := a.controller.Snapshot()
	png, err := qrcode.Encode(snapshot.TGLink, qrcode.Medium, 320)
	if err != nil {
		a.showError(err)
		return
	}

	resource := fyne.NewStaticResource("tg-proxy-link.png", png)
	image := canvas.NewImageFromResource(resource)
	image.FillMode = canvas.ImageFillContain
	image.SetMinSize(fyne.NewSize(320, 320))

	linkText := widget.NewMultiLineEntry()
	linkText.SetText(snapshot.TGLink)
	linkText.Disable()
	linkText.Wrapping = fyne.TextWrapBreak

	content := container.NewVBox(
		widget.NewLabel("Scan in Telegram or copy the link below."),
		image,
		linkText,
	)
	dialog.NewCustom("Telegram Proxy QR", "Close", content, a.window).Show()
}

func (a *App) showError(err error) {
	dialog.ShowError(err, a.window)
}
