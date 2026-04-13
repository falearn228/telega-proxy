package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func (s *Service) dialUpstream(ctx context.Context, network, targetAddr string) (net.Conn, error) {
	timeout := time.Duration(s.cfg.ConnectTimout) * time.Second
	dialer := &net.Dialer{Timeout: timeout}
	if !s.cfg.UseRelay {
		return dialer.DialContext(ctx, network, targetAddr)
	}

	relayAddr := strings.TrimSpace(s.cfg.RelayAddr)
	conn, err := dialer.DialContext(ctx, network, relayAddr)
	if err != nil {
		return nil, fmt.Errorf("dial relay %s: %w", relayAddr, err)
	}
	deadline := time.Now().Add(timeout)
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := writeConnectRequest(conn, targetAddr); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("connect via relay %s to %s: %w", relayAddr, targetAddr, err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read relay response from %s: %w", relayAddr, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = conn.Close()
		return nil, fmt.Errorf("relay %s CONNECT %s failed: %s", relayAddr, targetAddr, resp.Status)
	}
	_ = conn.SetDeadline(time.Time{})
	return conn, nil
}

func writeConnectRequest(w io.Writer, targetAddr string) error {
	_, err := fmt.Fprintf(
		w,
		"CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Connection: Keep-Alive\r\n\r\n",
		targetAddr,
		targetAddr,
	)
	return err
}
