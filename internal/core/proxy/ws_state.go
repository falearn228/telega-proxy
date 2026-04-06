package proxy

import (
	"fmt"
	"sync"
	"time"
)

const wsFailCooldown = 30 * time.Second

type wsTransportState struct {
	mu            sync.RWMutex
	blacklist     map[wsPoolKey]bool
	cooldownUntil map[wsPoolKey]time.Time
}

type WSDiagnosticEntry struct {
	DC            int    `json:"dc"`
	IsMedia       bool   `json:"is_media"`
	State         string `json:"state"`
	CooldownUntil string `json:"cooldown_until,omitempty"`
}

func newWSTransportState() *wsTransportState {
	return &wsTransportState{
		blacklist:     make(map[wsPoolKey]bool),
		cooldownUntil: make(map[wsPoolKey]time.Time),
	}
}

func (s *wsTransportState) SkipReason(key wsPoolKey) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.blacklist[key] {
		return fmt.Errorf("ws blacklisted for dc=%d media=%t", key.dc, key.isMedia)
	}
	if until, ok := s.cooldownUntil[key]; ok && time.Now().Before(until) {
		return fmt.Errorf("ws cooldown active until %s for dc=%d media=%t", until.Format(time.RFC3339), key.dc, key.isMedia)
	}
	return nil
}

func (s *wsTransportState) MarkBlacklisted(key wsPoolKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blacklist[key] = true
	delete(s.cooldownUntil, key)
}

func (s *wsTransportState) MarkCooldown(key wsPoolKey, d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cooldownUntil[key] = time.Now().Add(d)
}

func (s *wsTransportState) Clear(key wsPoolKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.blacklist, key)
	delete(s.cooldownUntil, key)
}

func (s *wsTransportState) Snapshot() []WSDiagnosticEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]WSDiagnosticEntry, 0, len(s.blacklist)+len(s.cooldownUntil))
	for key := range s.blacklist {
		result = append(result, WSDiagnosticEntry{
			DC:      key.dc,
			IsMedia: key.isMedia,
			State:   "blacklisted",
		})
	}
	now := time.Now()
	for key, until := range s.cooldownUntil {
		if now.After(until) {
			continue
		}
		result = append(result, WSDiagnosticEntry{
			DC:            key.dc,
			IsMedia:       key.isMedia,
			State:         "cooldown",
			CooldownUntil: until.Format(time.RFC3339),
		})
	}
	return result
}
