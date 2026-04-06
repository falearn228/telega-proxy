package mobilebridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/falearn/tg-fyne-proxy/internal/config"
	"github.com/falearn/tg-fyne-proxy/internal/core"
)

type snapshotPayload struct {
	Config     config.AppConfig `json:"config"`
	Running    bool             `json:"running"`
	ListenAddr string           `json:"listen_addr"`
	TGLink     string           `json:"tg_link"`
	Stats      any              `json:"stats"`
	Diag       any              `json:"diag"`
	Logs       string           `json:"logs"`
	LastError  string           `json:"last_error"`
}

var (
	mu         sync.Mutex
	ctrl       *core.Controller
	storageDir string
)

func ConfigureStorageDir(dir string) error {
	dir = filepath.Clean(strings.TrimSpace(dir))
	if dir == "." || dir == "" {
		return errors.New("storage dir is required")
	}

	mu.Lock()
	defer mu.Unlock()

	if ctrl != nil && ctrl.Snapshot().Running && storageDir != dir {
		return errors.New("cannot switch storage dir while proxy is running")
	}

	config.SetConfigDirOverride(dir)
	storageDir = dir
	if ctrl == nil {
		return nil
	}

	snapshot := ctrl.Snapshot()
	if !snapshot.Running {
		ctrl = nil
	}
	return nil
}

func StorageDir() string {
	mu.Lock()
	defer mu.Unlock()
	return storageDir
}

func LoadConfigJSON() (string, error) {
	controller, err := ensureController()
	if err != nil {
		return "", err
	}
	data, err := config.Marshal(controller.Snapshot().Config)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func SaveConfigJSON(raw string) error {
	cfg, err := config.Parse([]byte(raw))
	if err != nil {
		return err
	}
	controller, err := ensureController()
	if err != nil {
		return err
	}
	return controller.Apply(cfg, true)
}

func GenerateSecret() string {
	return config.MustGenerateSecret()
}

func StartProxy() error {
	controller, err := ensureController()
	if err != nil {
		return err
	}
	if controller.Snapshot().Running {
		return nil
	}
	return controller.Start()
}

func StopProxy() error {
	controller, err := ensureController()
	if err != nil {
		return err
	}
	return controller.Stop()
}

func IsRunning() bool {
	controller, err := ensureController()
	if err != nil {
		return false
	}
	return controller.Snapshot().Running
}

func SnapshotJSON() (string, error) {
	controller, err := ensureController()
	if err != nil {
		return "", err
	}
	snapshot := controller.Snapshot()
	payload := snapshotPayload{
		Config:     snapshot.Config,
		Running:    snapshot.Running,
		ListenAddr: snapshot.ListenAddr,
		TGLink:     snapshot.TGLink,
		Stats:      snapshot.Stats,
		Diag:       snapshot.Diag,
		Logs:       snapshot.Logs,
		LastError:  snapshot.LastError,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func ValidateConfigJSON(raw string) error {
	_, err := config.Parse([]byte(raw))
	return err
}

func ensureController() (*core.Controller, error) {
	mu.Lock()
	defer mu.Unlock()
	if storageDir == "" {
		return nil, fmt.Errorf("storage dir is not configured")
	}
	if ctrl != nil {
		return ctrl, nil
	}
	controller, err := core.NewController()
	if err != nil {
		return nil, err
	}
	ctrl = controller
	return ctrl, nil
}
