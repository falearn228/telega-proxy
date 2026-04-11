package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	AppName    = "TgFyneProxy"
	ConfigFile = "config.json"
)

var (
	configDirMu       sync.RWMutex
	configDirOverride string
)

type AppConfig struct {
	Host          string         `json:"host"`
	Port          int            `json:"port"`
	Secret        string         `json:"secret"`
	DCMap         map[int]string `json:"dc_map"`
	Verbose       bool           `json:"verbose"`
	BufferKB      int            `json:"buf_kb"`
	PoolSize      int            `json:"pool_size"`
	ConnectViaWS  bool           `json:"connect_via_ws"`
	PreferIPv6    bool           `json:"prefer_ipv6"`
	ConnectTimout int            `json:"connect_timeout_sec"`
	Autostart     bool           `json:"autostart,omitempty"`
}

func Default() AppConfig {
	return AppConfig{
		Host:          "127.0.0.1",
		Port:          1443,
		Secret:        MustGenerateSecret(),
		DCMap:         DefaultDCMap(),
		Verbose:       false,
		BufferKB:      256,
		PoolSize:      4,
		ConnectViaWS:  true,
		PreferIPv6:    false,
		ConnectTimout: 10,
	}
}

func DefaultDCMap() map[int]string {
	return map[int]string{
		2: "149.154.167.220",
		4: "149.154.167.220",
	}
}

func MustGenerateSecret() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}

func ConfigDir() (string, error) {
	configDirMu.RLock()
	override := configDirOverride
	configDirMu.RUnlock()
	if override != "" {
		return override, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, AppName), nil
}

func SetConfigDirOverride(dir string) {
	configDirMu.Lock()
	defer configDirMu.Unlock()
	configDirOverride = filepath.Clean(strings.TrimSpace(dir))
}

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ConfigFile), nil
}

func Load() (AppConfig, error) {
	cfg := Default()
	path, err := ConfigPath()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if isExpandedDefaultDCMap(cfg.DCMap) {
		cfg.DCMap = DefaultDCMap()
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func isExpandedDefaultDCMap(dcMap map[int]string) bool {
	if len(dcMap) != 6 {
		return false
	}
	return dcMap[1] == "149.154.175.50" &&
		dcMap[2] == "149.154.167.220" &&
		dcMap[3] == "149.154.175.100" &&
		(dcMap[4] == "149.154.167.91" || dcMap[4] == "149.154.167.220") &&
		dcMap[5] == "149.154.171.5" &&
		dcMap[203] == "91.105.192.100"
}

func Save(cfg AppConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, ConfigFile)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func Parse(data []byte) (AppConfig, error) {
	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func Marshal(cfg AppConfig) ([]byte, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return json.MarshalIndent(cfg, "", "  ")
}

func (c AppConfig) Validate() error {
	if c.Host == "" {
		return errors.New("host is required")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}
	if len(c.Secret) != 32 {
		return errors.New("secret must be 16 bytes in hex form")
	}
	if _, err := hex.DecodeString(c.Secret); err != nil {
		return fmt.Errorf("invalid secret: %w", err)
	}
	if c.BufferKB <= 0 {
		return errors.New("buf_kb must be positive")
	}
	if c.PoolSize <= 0 {
		return errors.New("pool_size must be positive")
	}
	if c.ConnectTimout <= 0 {
		return errors.New("connect_timeout_sec must be positive")
	}
	for dc, ip := range c.DCMap {
		if dc == 0 {
			return errors.New("dc id must not be zero")
		}
		if strings.TrimSpace(ip) == "" {
			return fmt.Errorf("dc %d has empty target", dc)
		}
	}
	return nil
}

func ParseDCMap(raw string) (map[int]string, error) {
	result := map[int]string{}
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid dc rule: %q", line)
		}
		dc, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("invalid dc id in %q: %w", line, err)
		}
		result[dc] = strings.TrimSpace(parts[1])
	}
	if len(result) == 0 {
		return nil, errors.New("at least one dc rule is required")
	}
	return result, nil
}

func FormatDCMap(dcMap map[int]string) string {
	keys := make([]int, 0, len(dcMap))
	for dc := range dcMap {
		keys = append(keys, dc)
	}
	sort.Ints(keys)

	lines := make([]string, 0, len(keys))
	for _, dc := range keys {
		lines = append(lines, fmt.Sprintf("%d:%s", dc, dcMap[dc]))
	}
	return strings.Join(lines, "\n")
}

func TelegramProxyLink(cfg AppConfig) string {
	host := cfg.Host
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}

	values := url.Values{}
	values.Set("server", host)
	values.Set("port", strconv.Itoa(cfg.Port))
	values.Set("secret", "dd"+cfg.Secret)
	return "tg://proxy?" + values.Encode()
}
