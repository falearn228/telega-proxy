package ui

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"github.com/skip2/go-qrcode"

	"github.com/falearn/tg-fyne-proxy/internal/config"
	"github.com/falearn/tg-fyne-proxy/internal/core"
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

	hostEntry    *widget.Entry
	portEntry    *widget.Entry
	secretEntry  *widget.Entry
	dcEntry      *widget.Entry
	bufferEntry  *widget.Entry
	poolEntry    *widget.Entry
	timeoutEntry *widget.Entry
	verboseCheck *widget.Check
	wsCheck      *widget.Check

	statusLabel *widget.Label
	statsLabel  *widget.Label
	linkEntry   *widget.Entry
	logEntry    *widget.Entry
	infoText    *widget.RichText
	tabs        *container.AppTabs
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

	form := widget.NewForm(
		widget.NewFormItem("Host", a.hostEntry),
		widget.NewFormItem("Port", a.portEntry),
		widget.NewFormItem("Secret", a.secretEntry),
		widget.NewFormItem("DC map", a.dcEntry),
		widget.NewFormItem("Buffer KB", a.bufferEntry),
		widget.NewFormItem("WS pool size", a.poolEntry),
		widget.NewFormItem("Connect timeout, sec", a.timeoutEntry),
		widget.NewFormItem("", a.verboseCheck),
		widget.NewFormItem("", a.wsCheck),
	)

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

	regenerateButton := widget.NewButton("New secret", func() {
		a.secretEntry.SetText(config.MustGenerateSecret())
	})

	startButton := widget.NewButton("Start proxy", func() {
		cfg, err := a.readForm()
		if err != nil {
			a.showError(err)
			return
		}
		if err := a.controller.Apply(cfg, true); err != nil {
			a.showError(err)
			return
		}
		if err := a.controller.Start(); err != nil {
			a.showError(err)
			return
		}
		a.refresh()
	})

	stopButton := widget.NewButton("Stop proxy", func() {
		if err := a.controller.Stop(); err != nil {
			a.showError(err)
			return
		}
		a.refresh()
	})

	copyLinkButton := widget.NewButton("Copy tg:// link", func() {
		snapshot := a.controller.Snapshot()
		a.window.Clipboard().SetContent(snapshot.TGLink)
	})

	exportButton := widget.NewButton("Export config", a.exportConfig)
	importButton := widget.NewButton("Import config", a.importConfig)
	qrButton := widget.NewButton("Show QR", a.showQR)

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

	actionRow1 := container.NewGridWithColumns(2, saveButton, regenerateButton)
	actionRow2 := container.NewGridWithColumns(2, startButton, stopButton)
	actionRow3 := container.NewGridWithColumns(2, exportButton, importButton)
	actionRow4 := container.NewGridWithColumns(3, copyLinkButton, qrButton, openInTelegramButton)

	var actionPanel fyne.CanvasObject
	if a.mobileMode {
		actionPanel = container.NewVBox(actionRow1, actionRow2, actionRow3, actionRow4)
	} else {
		actionPanel = container.NewVBox(
			container.NewHBox(saveButton, regenerateButton, layout.NewSpacer(), startButton, stopButton),
			container.NewHBox(exportButton, importButton, copyLinkButton, qrButton, openInTelegramButton),
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

	logPane := container.NewBorder(
		widget.NewLabel("Log"),
		nil,
		nil,
		nil,
		container.NewVScroll(a.logEntry),
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
	lifecycle := a.fyneApp.Lifecycle()

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
	a.logEntry.SetText(snapshot.Logs)
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
