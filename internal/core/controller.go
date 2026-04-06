package core

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/falearn/tg-fyne-proxy/internal/config"
	"github.com/falearn/tg-fyne-proxy/internal/core/proxy"
)

type Snapshot struct {
	Config     config.AppConfig
	Running    bool
	ListenAddr string
	TGLink     string
	Stats      proxy.Stats
	Diag       proxy.Diagnostics
	Logs       string
	LastError  string
}

type Controller struct {
	mu        sync.RWMutex
	cfg       config.AppConfig
	service   *proxy.Service
	logLines  []string
	lastError string
}

func NewController() (*Controller, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	c := &Controller{cfg: cfg}
	c.service = proxy.NewService(cfg, c.appendLog)
	return c, nil
}

func (c *Controller) Snapshot() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return Snapshot{
		Config:     c.cfg,
		Running:    c.service.Running(),
		ListenAddr: c.service.ListenAddr(),
		TGLink:     config.TelegramProxyLink(c.cfg),
		Stats:      c.service.Stats(),
		Diag:       c.service.Diagnostics(),
		Logs:       strings.Join(c.logLines, "\n"),
		LastError:  c.lastError,
	}
}

func (c *Controller) Apply(cfg config.AppConfig, persist bool) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if persist {
		if err := config.Save(cfg); err != nil {
			return err
		}
	}

	c.mu.RLock()
	oldService := c.service
	wasRunning := oldService.Running()
	c.mu.RUnlock()

	if wasRunning {
		if err := oldService.Stop(); err != nil {
			return err
		}
	}

	c.mu.Lock()
	c.cfg = cfg
	c.service = proxy.NewService(cfg, c.appendLog)
	c.lastError = ""
	c.mu.Unlock()

	if wasRunning {
		if err := c.Start(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) Start() error {
	c.mu.Lock()
	service := c.service
	c.lastError = ""
	c.mu.Unlock()

	if err := service.Start(); err != nil {
		c.mu.Lock()
		c.lastError = err.Error()
		c.mu.Unlock()
		return err
	}
	return nil
}

func (c *Controller) Stop() error {
	c.mu.RLock()
	service := c.service
	c.mu.RUnlock()
	return service.Stop()
}

func (c *Controller) appendLog(format string, args ...any) {
	line := fmt.Sprintf("%s  %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logLines = append(c.logLines, line)
	if len(c.logLines) > 200 {
		c.logLines = c.logLines[len(c.logLines)-200:]
	}
}
