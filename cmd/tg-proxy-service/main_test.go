package main

import (
	"os"
	"testing"

	"github.com/falearn/tg-fyne-proxy/internal/config"
)

func TestLoadOrCreateConfigPersistsDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := loadOrCreateConfig()
	if err != nil {
		t.Fatal(err)
	}
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("config permissions = %o, want 600", got)
	}

	reloaded, err := loadOrCreateConfig()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Secret != cfg.Secret {
		t.Fatal("secret changed after reloading config")
	}
}
