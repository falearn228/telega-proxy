package proxy

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type WSHandshakeError struct {
	StatusCode int
	StatusLine string
	Headers    http.Header
	Location   string
}

func (e *WSHandshakeError) Error() string {
	return fmt.Sprintf("websocket handshake failed: %s", e.StatusLine)
}

func (e *WSHandshakeError) IsRedirect() bool {
	switch e.StatusCode {
	case 301, 302, 303, 307, 308:
		return true
	default:
		return false
	}
}

type RawWebSocket struct {
	conn   net.Conn
	reader *bufio.Reader
	closed bool
}

func ConnectRawWebSocket(ip, domain string, timeout time.Duration) (*RawWebSocket, error) {
	return ConnectRawWebSocketWithDial(ip, domain, timeout, func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialer := &net.Dialer{Timeout: timeout}
		return dialer.DialContext(ctx, network, addr)
	})
}

func ConnectRawWebSocketWithDial(ip, domain string, timeout time.Duration, dial func(context.Context, string, string) (net.Conn, error)) (*RawWebSocket, error) {
	addr := net.JoinHostPort(ip, "443")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rawConn, err := dial(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	conn := tls.Client(rawConn, &tls.Config{
		ServerName:         domain,
		InsecureSkipVerify: true,
	})
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}

	wsKeyRaw := make([]byte, 16)
	if _, err := rand.Read(wsKeyRaw); err != nil {
		_ = conn.Close()
		return nil, err
	}
	wsKey := base64.StdEncoding.EncodeToString(wsKeyRaw)

	req := strings.Join([]string{
		"GET /apiws HTTP/1.1",
		"Host: " + domain,
		"Upgrade: websocket",
		"Connection: Upgrade",
		"Sec-WebSocket-Key: " + wsKey,
		"Sec-WebSocket-Version: 13",
		"Sec-WebSocket-Protocol: binary",
		"Origin: https://web.telegram.org",
		"User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"",
		"",
	}, "\r\n")

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, err := io.WriteString(conn, req); err != nil {
		_ = conn.Close()
		return nil, err
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		defer resp.Body.Close()
		_ = conn.Close()
		return nil, &WSHandshakeError{
			StatusCode: resp.StatusCode,
			StatusLine: resp.Status,
			Headers:    resp.Header.Clone(),
			Location:   resp.Header.Get("Location"),
		}
	}
	_ = conn.SetDeadline(time.Time{})

	return &RawWebSocket{conn: conn, reader: reader}, nil
}

func (ws *RawWebSocket) Send(payload []byte) error {
	if ws.closed {
		return io.ErrClosedPipe
	}
	frame, err := buildWSFrame(0x2, payload, true)
	if err != nil {
		return err
	}
	_, err = ws.conn.Write(frame)
	return err
}

func (ws *RawWebSocket) SendBatch(parts [][]byte) error {
	for _, part := range parts {
		if err := ws.Send(part); err != nil {
			return err
		}
	}
	return nil
}

func (ws *RawWebSocket) Recv() ([]byte, error) {
	for {
		opcode, payload, err := ws.readFrame()
		if err != nil {
			return nil, err
		}
		switch opcode {
		case 0x8:
			ws.closed = true
			_ = ws.Close()
			return nil, io.EOF
		case 0x9:
			if frame, err := buildWSFrame(0xA, payload, true); err == nil {
				_, _ = ws.conn.Write(frame)
			}
		case 0xA:
		case 0x1, 0x2:
			return payload, nil
		}
	}
}

func (ws *RawWebSocket) Close() error {
	if ws.closed {
		return nil
	}
	ws.closed = true
	frame, err := buildWSFrame(0x8, nil, true)
	if err == nil {
		_, _ = ws.conn.Write(frame)
	}
	return ws.conn.Close()
}

func (ws *RawWebSocket) IsClosed() bool {
	return ws == nil || ws.closed
}

func (ws *RawWebSocket) readFrame() (byte, []byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(ws.reader, header); err != nil {
		return 0, nil, err
	}
	opcode := header[0] & 0x0F
	payloadLen := uint64(header[1] & 0x7F)

	switch payloadLen {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(ws.reader, ext[:]); err != nil {
			return 0, nil, err
		}
		payloadLen = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(ws.reader, ext[:]); err != nil {
			return 0, nil, err
		}
		payloadLen = binary.BigEndian.Uint64(ext[:])
	}

	var maskKey [4]byte
	if header[1]&0x80 != 0 {
		if _, err := io.ReadFull(ws.reader, maskKey[:]); err != nil {
			return 0, nil, err
		}
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(ws.reader, payload); err != nil {
		return 0, nil, err
	}
	if header[1]&0x80 != 0 {
		xorMask(payload, maskKey[:])
	}
	return opcode, payload, nil
}

func buildWSFrame(opcode byte, payload []byte, masked bool) ([]byte, error) {
	first := byte(0x80) | opcode
	header := []byte{first}
	payloadLen := len(payload)

	switch {
	case payloadLen < 126:
		header = append(header, byte(payloadLen))
	case payloadLen <= 0xFFFF:
		header = append(header, 126, 0, 0)
		binary.BigEndian.PutUint16(header[len(header)-2:], uint16(payloadLen))
	default:
		header = append(header, 127, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(header[len(header)-8:], uint64(payloadLen))
	}

	if !masked {
		return append(header, payload...), nil
	}

	maskKey := make([]byte, 4)
	if _, err := rand.Read(maskKey); err != nil {
		return nil, err
	}
	header[1] |= 0x80

	maskedPayload := append([]byte(nil), payload...)
	xorMask(maskedPayload, maskKey)

	frame := make([]byte, 0, len(header)+4+len(maskedPayload))
	frame = append(frame, header...)
	frame = append(frame, maskKey...)
	frame = append(frame, maskedPayload...)
	return frame, nil
}

func xorMask(data, mask []byte) {
	for i := range data {
		data[i] ^= mask[i%4]
	}
}
