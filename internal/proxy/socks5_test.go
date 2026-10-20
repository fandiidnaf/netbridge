package proxy

import (
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

func TestSOCKSServerEcho(t *testing.T) {
	// Mock TCP echo target
	targetListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start target listener: %v", err)
	}
	defer targetListener.Close()

	go func() {
		for {
			conn, err := targetListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				n, _ := c.Read(buf)
				_, _ = c.Write(append([]byte("SOCKS_ECHO:"), buf[:n]...))
			}(conn)
		}
	}()

	metrics := NewMetrics()
	socksServer := NewSOCKSServer("127.0.0.1", 0, metrics)
	go func() {
		_ = socksServer.Start()
	}()
	defer socksServer.Close()

	// Wait for listener to bind
	var socksAddr net.Addr
	for i := 0; i < 20; i++ {
		socksAddr = socksServer.Addr()
		if socksAddr != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if socksAddr == nil {
		t.Fatal("SOCKS server failed to start listener")
	}

	conn, err := net.Dial("tcp", socksAddr.String())
	if err != nil {
		t.Fatalf("failed to dial SOCKS server: %v", err)
	}
	defer conn.Close()

	// 1. Send SOCKS5 handshake (version 5, 1 method: 0x00 no auth)
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatalf("failed to write handshake: %v", err)
	}
	handshakeResp := make([]byte, 2)
	if _, err := io.ReadFull(conn, handshakeResp); err != nil {
		t.Fatalf("failed to read handshake response: %v", err)
	}
	if handshakeResp[0] != 0x05 || handshakeResp[1] != 0x00 {
		t.Fatalf("unexpected handshake response: %v", handshakeResp)
	}

	// 2. Send CONNECT request to target (IPv4: 127.0.0.1, port)
	targetPort := targetListener.Addr().(*net.TCPAddr).Port
	connectReq := []byte{
		0x05, 0x01, 0x00, 0x01,
		127, 0, 0, 1,
		byte(targetPort >> 8), byte(targetPort & 0xFF),
	}
	if _, err := conn.Write(connectReq); err != nil {
		t.Fatalf("failed to write connect request: %v", err)
	}

	replyBuf := make([]byte, 10)
	if _, err := io.ReadFull(conn, replyBuf); err != nil {
		t.Fatalf("failed to read connect reply: %v", err)
	}
	if replyBuf[1] != 0x00 {
		t.Fatalf("SOCKS connect failed with reply code: %d", replyBuf[1])
	}

	// 3. Send test data
	testPayload := []byte("ping-socks")
	if _, err := conn.Write(testPayload); err != nil {
		t.Fatalf("failed to send payload: %v", err)
	}

	respBuf := make([]byte, 1024)
	n, err := conn.Read(respBuf)
	if err != nil {
		t.Fatalf("failed to read payload echo: %v", err)
	}
	expected := "SOCKS_ECHO:ping-socks"
	if string(respBuf[:n]) != expected {
		t.Errorf("expected %q, got %q", expected, string(respBuf[:n]))
	}

	// Check metrics
	if metrics.TotalRequests == 0 {
		t.Error("expected metrics TotalRequests to be recorded")
	}
}

func TestSOCKSAddressFormat(t *testing.T) {
	host := "127.0.0.1"
	port := 1080
	s := NewSOCKSServer(host, port, nil)
	expectedAddr := fmt.Sprintf("%s:%d", host, port)
	if s.addr != expectedAddr {
		t.Errorf("expected addr %q, got %q", expectedAddr, s.addr)
	}
}
