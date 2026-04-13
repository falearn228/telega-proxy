package proxy

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/falearn/tg-fyne-proxy/internal/config"
)

func TestDialUpstreamUsesHTTPConnectRelay(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer ln.Close()

	reqCh := make(chan *http.Request, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()

		req, err := http.ReadRequest(bufio.NewReader(conn))
		if err != nil {
			errCh <- err
			return
		}
		reqCh <- req
		_, _ = fmt.Fprint(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		time.Sleep(50 * time.Millisecond)
	}()

	cfg := config.Default()
	cfg.UseRelay = true
	cfg.RelayAddr = ln.Addr().String()
	service := NewService(cfg, nil)

	conn, err := service.dialUpstream(context.Background(), "tcp", "149.154.167.220:443")
	if err != nil {
		t.Fatalf("dialUpstream() error = %v", err)
	}
	defer conn.Close()

	select {
	case req := <-reqCh:
		if req.Method != "CONNECT" {
			t.Fatalf("method = %s, want CONNECT", req.Method)
		}
		if req.Host != "149.154.167.220:443" {
			t.Fatalf("host = %s, want target", req.Host)
		}
	case err := <-errCh:
		t.Fatalf("relay server error = %v", err)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for CONNECT request")
	}
}
