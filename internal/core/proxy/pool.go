package proxy

import (
	"io"
	"sync"
	"time"
)

type wsPoolKey struct {
	dc      int
	isMedia bool
}

type pooledWS struct {
	ws      *RawWebSocket
	created time.Time
}

type wsPool struct {
	mu        sync.Mutex
	idle      map[wsPoolKey][]pooledWS
	refilling map[wsPoolKey]bool
	maxAge    time.Duration
	maxSize   int
}

type WSPoolEntry struct {
	DC        int  `json:"dc"`
	IsMedia   bool `json:"is_media"`
	Idle      int  `json:"idle"`
	Refilling bool `json:"refilling"`
}

func newWSPool(maxSize int) *wsPool {
	if maxSize <= 0 {
		maxSize = 1
	}
	return &wsPool{
		idle:      make(map[wsPoolKey][]pooledWS),
		refilling: make(map[wsPoolKey]bool),
		maxAge:    2 * time.Minute,
		maxSize:   maxSize,
	}
}

func (p *wsPool) Get(key wsPoolKey) *RawWebSocket {
	p.mu.Lock()
	defer p.mu.Unlock()

	bucket := p.idle[key]
	now := time.Now()
	for len(bucket) > 0 {
		item := bucket[0]
		bucket = bucket[1:]
		if now.Sub(item.created) > p.maxAge || item.ws == nil || item.ws.IsClosed() {
			if item.ws != nil {
				_ = item.ws.Close()
			}
			continue
		}
		p.idle[key] = bucket
		return item.ws
	}
	p.idle[key] = bucket
	return nil
}

func (p *wsPool) Warmup(key wsPoolKey, dial func() (*RawWebSocket, error)) {
	p.mu.Lock()
	if p.refilling[key] {
		p.mu.Unlock()
		return
	}
	if len(p.idle[key]) >= p.maxSize {
		p.mu.Unlock()
		return
	}
	p.refilling[key] = true
	p.mu.Unlock()

	go func() {
		defer func() {
			p.mu.Lock()
			delete(p.refilling, key)
			p.mu.Unlock()
		}()

		for {
			p.mu.Lock()
			current := len(p.idle[key])
			if current >= p.maxSize {
				p.mu.Unlock()
				return
			}
			p.mu.Unlock()

			ws, err := dial()
			if err != nil {
				if err != io.EOF {
					return
				}
				return
			}

			p.mu.Lock()
			if len(p.idle[key]) < p.maxSize {
				p.idle[key] = append(p.idle[key], pooledWS{ws: ws, created: time.Now()})
				p.mu.Unlock()
				continue
			}
			p.mu.Unlock()
			_ = ws.Close()
			return
		}
	}()
}

func (p *wsPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, bucket := range p.idle {
		for _, item := range bucket {
			if item.ws != nil {
				_ = item.ws.Close()
			}
		}
	}
	p.idle = make(map[wsPoolKey][]pooledWS)
	p.refilling = make(map[wsPoolKey]bool)
}

func (p *wsPool) Snapshot() []WSPoolEntry {
	p.mu.Lock()
	defer p.mu.Unlock()

	result := make([]WSPoolEntry, 0, len(p.idle))
	for key, bucket := range p.idle {
		result = append(result, WSPoolEntry{
			DC:        key.dc,
			IsMedia:   key.isMedia,
			Idle:      len(bucket),
			Refilling: p.refilling[key],
		})
	}
	for key, refilling := range p.refilling {
		if _, ok := p.idle[key]; ok {
			continue
		}
		result = append(result, WSPoolEntry{
			DC:        key.dc,
			IsMedia:   key.isMedia,
			Idle:      0,
			Refilling: refilling,
		})
	}
	return result
}
