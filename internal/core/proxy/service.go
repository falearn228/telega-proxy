package proxy

import (
	"context"
	"crypto/cipher"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/falearn/tg-fyne-proxy/internal/config"
)

var defaultDCIPs = map[int]string{
	1:   "149.154.175.50",
	2:   "149.154.167.51",
	3:   "149.154.175.100",
	4:   "149.154.167.91",
	5:   "149.154.171.5",
	203: "91.105.192.100",
}

type Stats struct {
	ConnectionsTotal   uint64
	ConnectionsActive  uint64
	ConnectionsErrored uint64
	ConnectionsWS      uint64
	ConnectionsTCP     uint64
	WSErrors           uint64
	BytesUp            uint64
	BytesDown          uint64
}

type Diagnostics struct {
	LastUpstreamMode string              `json:"last_upstream_mode"`
	WSState          []WSDiagnosticEntry `json:"ws_state"`
	WSPool           []WSPoolEntry       `json:"ws_pool"`
}

type Logger func(format string, args ...any)

type Service struct {
	cfg      config.AppConfig
	logger   Logger
	wsPool   *wsPool
	wsState  *wsTransportState
	mu       sync.RWMutex
	listener net.Listener
	cancel   context.CancelFunc
	conns    map[net.Conn]struct{}
	wg       sync.WaitGroup
	running  atomic.Bool
	stats    statsCounter
	lastMode atomic.Value
}

type statsCounter struct {
	total   atomic.Uint64
	active  atomic.Uint64
	errored atomic.Uint64
	ws      atomic.Uint64
	tcp     atomic.Uint64
	wsErr   atomic.Uint64
	up      atomic.Uint64
	down    atomic.Uint64
}

func NewService(cfg config.AppConfig, logger Logger) *Service {
	if logger == nil {
		logger = func(string, ...any) {}
	}
	return &Service{
		cfg:     cfg,
		logger:  logger,
		wsPool:  newWSPool(cfg.PoolSize),
		wsState: newWSTransportState(),
		conns:   make(map[net.Conn]struct{}),
	}
}

func (s *Service) Start() error {
	if s.running.Load() {
		return errors.New("proxy already running")
	}

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.listener = ln
	s.cancel = cancel
	s.mu.Unlock()
	s.running.Store(true)
	s.logger("proxy started on %s", addr)
	if s.cfg.ConnectViaWS {
		s.warmupWSPool()
	}

	s.wg.Go(func() {
		s.acceptLoop(ctx, ln)
	})
	return nil
}

func (s *Service) Stop() error {
	if !s.running.Load() {
		return nil
	}

	s.mu.Lock()
	ln := s.listener
	cancel := s.cancel
	conns := make([]net.Conn, 0, len(s.conns))
	for conn := range s.conns {
		conns = append(conns, conn)
	}
	s.listener = nil
	s.cancel = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if ln != nil {
		_ = ln.Close()
	}
	for _, conn := range conns {
		_ = conn.Close()
	}
	s.wg.Wait()
	if s.wsPool != nil {
		s.wsPool.Close()
	}
	s.running.Store(false)
	s.logger("proxy stopped")
	return nil
}

func (s *Service) Running() bool {
	return s.running.Load()
}

func (s *Service) ListenAddr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *Service) Stats() Stats {
	return Stats{
		ConnectionsTotal:   s.stats.total.Load(),
		ConnectionsActive:  s.stats.active.Load(),
		ConnectionsErrored: s.stats.errored.Load(),
		ConnectionsWS:      s.stats.ws.Load(),
		ConnectionsTCP:     s.stats.tcp.Load(),
		WSErrors:           s.stats.wsErr.Load(),
		BytesUp:            s.stats.up.Load(),
		BytesDown:          s.stats.down.Load(),
	}
}

func (s *Service) Diagnostics() Diagnostics {
	diag := Diagnostics{}
	if mode, ok := s.lastMode.Load().(string); ok {
		diag.LastUpstreamMode = mode
	}
	if s.wsState != nil {
		diag.WSState = s.wsState.Snapshot()
	}
	if s.wsPool != nil {
		diag.WSPool = s.wsPool.Snapshot()
	}
	return diag
}

func (s *Service) acceptLoop(ctx context.Context, ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			s.stats.errored.Add(1)
			s.logger("accept error: %v", err)
			continue
		}

		s.stats.total.Add(1)
		s.stats.active.Add(1)
		s.trackConn(conn)
		s.wg.Go(func() {
			defer s.stats.active.Add(^uint64(0))
			defer s.untrackConn(conn)
			if err := s.handleConn(ctx, conn); err != nil {
				s.stats.errored.Add(1)
				s.logger("connection error: %v", err)
			}
		})
	}
}

func (s *Service) trackConn(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conns[conn] = struct{}{}
}

func (s *Service) untrackConn(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, conn)
}

func (s *Service) handleConn(ctx context.Context, client net.Conn) error {
	defer client.Close()
	_ = client.SetDeadline(time.Time{})

	helloBuf := make([]byte, handshakeLen)
	if _, err := io.ReadFull(client, helloBuf); err != nil {
		return fmt.Errorf("read client handshake: %w", err)
	}

	hello, err := ParseClientHello(helloBuf, s.cfg.Secret)
	if err != nil {
		return err
	}

	clientDec, clientEnc, err := NewClientDecryptor(hello.ClientDecBytes, s.cfg.Secret)
	if err != nil {
		return err
	}

	targetHost, configured := s.resolveTarget(hello.DC)
	if s.cfg.ConnectViaWS && configured {
		wsConn, err := s.connectTelegramWS(hello.DC, hello.IsMedia, targetHost)
		if err == nil {
			defer wsConn.Close()
			relayInit, err := GenerateRelayInit(hello.Protocol, hello.DC, hello.IsMedia)
			if err != nil {
				return fmt.Errorf("build ws relay init: %w", err)
			}
			tgEnc, tgDec, err := NewRelayCryptors(relayInit)
			if err != nil {
				return err
			}
			if err := wsConn.Send(relayInit); err != nil {
				return fmt.Errorf("write ws relay init: %w", err)
			}
			s.stats.ws.Add(1)
			s.lastMode.Store("telegram-wss")
			s.logger("dc=%d media=%t protocol=%s upstream=wss mode=telegram-wss via %s", hello.DC, hello.IsMedia, hello.Protocol, targetHost)
			splitter := NewMessageSplitter(relayInit, hello.Protocol)
			return s.bridgeWS(client, wsConn, clientDec, clientEnc, tgEnc, tgDec, splitter)
		} else {
			s.stats.wsErr.Add(1)
			s.logger("dc=%d media=%t ws connect failed, fallback to tcp: %v", hello.DC, hello.IsMedia, err)
		}
	} else if s.cfg.ConnectViaWS {
		s.logger("dc=%d media=%t not in config -> fallback", hello.DC, hello.IsMedia)
	}

	relayInit, err := GenerateRelayInit(hello.Protocol, hello.DC, hello.IsMedia)
	if err != nil {
		return fmt.Errorf("build relay init: %w", err)
	}
	tgEnc, tgDec, err := NewRelayCryptors(relayInit)
	if err != nil {
		return err
	}

	targetAddr := net.JoinHostPort(targetHost, "443")
	server, err := s.dialUpstream(ctx, "tcp", targetAddr)
	if err != nil {
		return fmt.Errorf("dial target %s: %w", targetAddr, err)
	}
	defer server.Close()
	if _, err := server.Write(relayInit); err != nil {
		return fmt.Errorf("write relay init: %w", err)
	}

	s.stats.tcp.Add(1)
	mode := "tcp-direct"
	if s.cfg.UseRelay {
		mode = "tcp-relay"
	}
	s.lastMode.Store(mode)
	s.logger("dc=%d media=%t protocol=%s upstream=%s mode=%s", hello.DC, hello.IsMedia, hello.Protocol, targetAddr, mode)
	return s.bridge(client, server, clientDec, clientEnc, tgEnc, tgDec)
}

func (s *Service) resolveTarget(dc int) (string, bool) {
	if target, ok := s.cfg.DCMap[dc]; ok && target != "" {
		return target, true
	}
	if target, ok := defaultDCIPs[dc]; ok {
		return target, false
	}
	return defaultDCIPs[2], false
}

func (s *Service) bridge(client net.Conn, server net.Conn, clientDec, clientEnc, tgEnc, tgDec cipher.Stream) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 2)
	copyStream := func(dst net.Conn, src net.Conn, dec cipher.Stream, enc cipher.Stream, counter *atomic.Uint64) {
		buf := make([]byte, s.cfg.BufferKB*1024)
		for {
			n, err := src.Read(buf)
			if n > 0 {
				plain := make([]byte, n)
				dec.XORKeyStream(plain, buf[:n])
				out := make([]byte, n)
				enc.XORKeyStream(out, plain)
				written, werr := dst.Write(out)
				counter.Add(uint64(written))
				if werr != nil {
					errCh <- werr
					return
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					errCh <- nil
					return
				}
				errCh <- err
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}

	go copyStream(server, client, clientDec, tgEnc, &s.stats.up)
	go copyStream(client, server, tgDec, clientEnc, &s.stats.down)

	err := <-errCh
	cancel()
	return err
}
