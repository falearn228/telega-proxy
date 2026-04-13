package proxy

import (
	"context"
	"crypto/cipher"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

var dcWSOverrides = map[int]int{
	203: 2,
}

func (s *Service) connectTelegramWS(dc int, isMedia bool, targetIP string) (*RawWebSocket, error) {
	key := wsPoolKey{dc: dc, isMedia: isMedia}
	if s.wsState != nil {
		if err := s.wsState.SkipReason(key); err != nil {
			return nil, err
		}
	}
	if s.wsPool != nil {
		if ws := s.wsPool.Get(key); ws != nil {
			s.logger("dc=%d media=%t ws pool hit via %s", dc, isMedia, targetIP)
			s.wsPool.Warmup(key, func() (*RawWebSocket, error) {
				return s.dialTelegramWS(dc, isMedia, targetIP)
			})
			return ws, nil
		}
	}

	ws, err := s.dialTelegramWS(dc, isMedia, targetIP)
	if err == nil && s.wsPool != nil {
		s.wsPool.Warmup(key, func() (*RawWebSocket, error) {
			return s.dialTelegramWS(dc, isMedia, targetIP)
		})
	}
	return ws, err
}

func (s *Service) dialTelegramWS(dc int, isMedia bool, targetIP string) (*RawWebSocket, error) {
	key := wsPoolKey{dc: dc, isMedia: isMedia}
	if s.wsState != nil {
		if err := s.wsState.SkipReason(key); err != nil {
			return nil, err
		}
	}

	allRedirects := true
	sawRedirect := false
	var lastErr error = io.EOF

	for _, domain := range wsDomains(dc, isMedia) {
		ws, err := ConnectRawWebSocketWithDial(targetIP, domain, time.Duration(s.cfg.ConnectTimout)*time.Second, s.dialUpstream)
		if err == nil {
			if s.wsState != nil {
				s.wsState.Clear(key)
			}
			return ws, nil
		}
		lastErr = err
		var hsErr *WSHandshakeError
		if errors.As(err, &hsErr) && hsErr.IsRedirect() {
			sawRedirect = true
			s.logger("dc=%d media=%t ws redirect from %s to %s", dc, isMedia, domain, hsErr.Location)
			continue
		}
		allRedirects = false
		s.logger("dc=%d media=%t ws connect failed via %s: %v", dc, isMedia, domain, err)
	}

	if sawRedirect && allRedirects {
		if s.wsState != nil {
			s.wsState.MarkBlacklisted(key)
		}
		return nil, fmt.Errorf("ws blacklisted after redirects for dc=%d media=%t", dc, isMedia)
	}
	if s.wsState != nil {
		s.wsState.MarkCooldown(key, wsFailCooldown)
	}
	return nil, fmt.Errorf("ws cooldown enabled for dc=%d media=%t: %w", dc, isMedia, lastErr)
}

func (s *Service) warmupWSPool() {
	if s.wsPool == nil {
		return
	}
	for dc := range s.cfg.DCMap {
		targetIP := s.cfg.DCMap[dc]
		for _, isMedia := range []bool{false, true} {
			dcCopy := dc
			targetIPCopy := targetIP
			isMediaCopy := isMedia
			key := wsPoolKey{dc: dc, isMedia: isMedia}
			s.wsPool.Warmup(key, func() (*RawWebSocket, error) {
				return s.dialTelegramWS(dcCopy, isMediaCopy, targetIPCopy)
			})
		}
	}
}

func wsDomains(dc int, isMedia bool) []string {
	if override, ok := dcWSOverrides[dc]; ok {
		dc = override
	}
	if isMedia {
		return []string{
			"kws" + itoa(dc) + "-1.web.telegram.org",
			"kws" + itoa(dc) + ".web.telegram.org",
		}
	}
	return []string{
		"kws" + itoa(dc) + ".web.telegram.org",
		"kws" + itoa(dc) + "-1.web.telegram.org",
	}
}

func (s *Service) bridgeWS(client io.ReadWriteCloser, ws *RawWebSocket, clientDec, clientEnc, tgEnc, tgDec cipher.Stream, splitter *MessageSplitter) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 2)
	var upBytes atomic.Uint64
	var downBytes atomic.Uint64
	var upPackets atomic.Uint64
	var downPackets atomic.Uint64
	started := time.Now()
	defer func() {
		s.logger(
			"ws session closed: up=%d bytes (%d pkts) down=%d bytes (%d pkts) in %.1fs",
			upBytes.Load(),
			upPackets.Load(),
			downBytes.Load(),
			downPackets.Load(),
			time.Since(started).Seconds(),
		)
	}()

	go func() {
		buf := make([]byte, s.cfg.BufferKB*1024)
		for {
			n, err := client.Read(buf)
			if n > 0 {
				plain := make([]byte, n)
				clientDec.XORKeyStream(plain, buf[:n])
				toTelegram := make([]byte, n)
				tgEnc.XORKeyStream(toTelegram, plain)
				var parts [][]byte
				if splitter != nil {
					parts = splitter.Split(toTelegram)
				} else {
					parts = [][]byte{toTelegram}
				}
				if len(parts) == 0 {
					continue
				}
				if len(parts) > 1 {
					if err := ws.SendBatch(parts); err != nil {
						errCh <- err
						return
					}
				} else if err := ws.Send(parts[0]); err != nil {
					errCh <- err
					return
				}
				s.stats.up.Add(uint64(n))
				upBytes.Add(uint64(n))
				upPackets.Add(uint64(len(parts)))
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					if splitter != nil {
						if tail := splitter.Flush(); len(tail) > 0 {
							_ = ws.SendBatch(tail)
							upPackets.Add(uint64(len(tail)))
						}
					}
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
	}()

	go func() {
		for {
			payload, err := ws.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					errCh <- nil
					return
				}
				errCh <- err
				return
			}
			plain := make([]byte, len(payload))
			tgDec.XORKeyStream(plain, payload)
			out := make([]byte, len(payload))
			clientEnc.XORKeyStream(out, plain)
			if _, err := client.Write(out); err != nil {
				errCh <- err
				return
			}
			s.stats.down.Add(uint64(len(payload)))
			downBytes.Add(uint64(len(payload)))
			downPackets.Add(1)
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()

	err := <-errCh
	cancel()
	return err
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	n := v
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
