package main

import (
	"log"

	"github.com/falearn/tg-fyne-proxy/internal/core"
	"github.com/falearn/tg-fyne-proxy/internal/ui"
)

func main() {
	controller, err := core.NewController()
	if err != nil {
		log.Fatal(err)
	}

	app := ui.NewApp(controller)
	app.Run()
}
