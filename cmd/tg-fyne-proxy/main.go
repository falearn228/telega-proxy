package main

import (
	"log"
	"os"

	"github.com/falearn/tg-fyne-proxy/internal/core"
	"github.com/falearn/tg-fyne-proxy/internal/platform"
	"github.com/falearn/tg-fyne-proxy/internal/ui"
)

func main() {
	controller, err := core.NewController()
	if err != nil {
		log.Fatal(err)
	}

	autostart := platform.IsAutostartLaunch(os.Args[1:]) || controller.Snapshot().Config.Autostart
	app := ui.NewApp(controller, autostart)
	app.Run()
}
