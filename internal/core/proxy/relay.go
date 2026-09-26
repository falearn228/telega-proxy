package proxy

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Service) dialUpstream(ctx context.Context, network, targetAddr string) (net.Conn, error) {
	timeout := time.Duration(s.cfg.ConnectTimeout) * time.Second
	dialer := &net.Dialer{Timeout: timeout}
	proxyURL, err := s.proxyForTarget(targetAddr)
	if err != nil {
		return nil, err
	}
	if proxyURL == nil {
		return dialer.DialContext(ctx, network, targetAddr) // коннект без прокси к DC
	}
	if proxyURL.Scheme != "http" {
		return nil, fmt.Errorf("unsupported upstream proxy scheme %q (HTTP CONNECT required)", proxyURL.Scheme)
	}

	proxyPort := proxyURL.Port()
	if proxyPort == "" {
		proxyPort = "80"
	}
	proxyAddr := net.JoinHostPort(proxyURL.Hostname(), proxyPort)
	conn, err := dialer.DialContext(ctx, network, proxyAddr) // коннект через прокси к DC
	if err != nil {
		return nil, fmt.Errorf("dial upstream proxy %s: %w", proxyAddr, err)
	}
	deadline := time.Now().Add(timeout)
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := writeConnectRequest(conn, targetAddr, proxyURL.User); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("connect via upstream proxy %s to %s: %w", proxyAddr, targetAddr, err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read upstream proxy response from %s: %w", proxyAddr, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = conn.Close()
		return nil, fmt.Errorf("upstream proxy %s CONNECT %s failed: %s", proxyAddr, targetAddr, resp.Status)
	}
	_ = conn.SetDeadline(time.Time{})
	return conn, nil
}

func (s *Service) proxyForTarget(targetAddr string) (*url.URL, error) {
	if s.cfg.UseRelay {
		return &url.URL{Scheme: "http", Host: strings.TrimSpace(s.cfg.RelayAddr)}, nil
	}
	return http.ProxyFromEnvironment(&http.Request{URL: &url.URL{Scheme: "https", Host: targetAddr}})
}

func writeConnectRequest(w io.Writer, targetAddr string, user *url.Userinfo) error {
	if _, err := fmt.Fprintf(w, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Connection: Keep-Alive\r\n", targetAddr, targetAddr); err != nil {
		return err
	}
	if user != nil {
		password, _ := user.Password()
		encoded := base64.StdEncoding.EncodeToString([]byte(user.Username() + ":" + password))
		if _, err := fmt.Fprintf(w, "Proxy-Authorization: Basic %s\r\n", encoded); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "\r\n")
	return err
}
