//go:build windows

package platform

import (
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	autostartName = "TgFyneProxy"
	runKeyPath    = `Software\Microsoft\Windows\CurrentVersion\Run`
)

func SupportsAutostart() bool {
	return true
}

func IsAutostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	value, _, err := k.GetStringValue(autostartName)
	if err != nil {
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		return strings.TrimSpace(value) != ""
	}
	return value == autostartCommand(exe) || value == legacyAutostartCommand(exe)
}

func SetAutostart(enabled bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	if !enabled {
		if err := k.DeleteValue(autostartName); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue(autostartName, autostartCommand(exe))
}
