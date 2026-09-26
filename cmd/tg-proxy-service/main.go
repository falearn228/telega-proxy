package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/falearn/tg-fyne-proxy/internal/config"
	"github.com/falearn/tg-fyne-proxy/internal/core/proxy"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := loadOrCreateConfig()
	if err != nil {
		return err
	}

	service := proxy.NewService(cfg, log.Printf)
	if err := service.Start(); err != nil {
		return fmt.Errorf("start proxy: %w", err)
	}

	<-ctx.Done()
	return service.Stop()
}

func loadOrCreateConfig() (config.AppConfig, error) {
	path, err := config.ConfigPath()
	if err != nil {
		return config.AppConfig{}, fmt.Errorf("config path: %w", err)
	}
	_, err = os.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return config.AppConfig{}, fmt.Errorf("stat config: %w", err)
	}
	missing := errors.Is(err, os.ErrNotExist)

	cfg, err := config.Load()
	if err != nil {
		return cfg, fmt.Errorf("load config: %w", err)
	}
	if missing {
		if err := config.Save(cfg); err != nil {
			return cfg, fmt.Errorf("save default config: %w", err)
		}
		log.Printf("created config at %s", path)
	}
	return cfg, nil
}
