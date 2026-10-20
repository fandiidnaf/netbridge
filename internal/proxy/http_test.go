package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestHelperEndpoints(t *testing.T) {
	server := NewHTTPServer(ServerConfig{
		Host:      "127.0.0.1",
		HTTPPort:  9991,
		SOCKSPort: 1081,
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	server.config.HTTPPort = port
	server.httpServer.Addr = listener.Addr().String()

	go func() {
		_ = server.httpServer.Serve(listener)
	}()
	defer func() {
		_ = server.Shutdown(context.Background())
	}()

	time.Sleep(50 * time.Millisecond)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// 1. Test /proxy.pac
	resp, err := http.Get(baseURL + "/proxy.pac")
	if err != nil {
		t.Fatalf("GET /proxy.pac failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "FindProxyForURL") {
		t.Errorf("expected PAC function in body, got: %s", string(body))
	}

	// 2. Test /setup.sh
	respSetup, err := http.Get(baseURL + "/setup.sh")
	if err != nil {
		t.Fatalf("GET /setup.sh failed: %v", err)
	}
	defer respSetup.Body.Close()
	setupBody, _ := io.ReadAll(respSetup.Body)
	if !strings.Contains(string(setupBody), "networksetup") {
		t.Errorf("expected networksetup in setup script, got: %s", string(setupBody))
	}

	// 3. Test /restore.sh
	respRestore, err := http.Get(baseURL + "/restore.sh")
	if err != nil {
		t.Fatalf("GET /restore.sh failed: %v", err)
	}
	defer respRestore.Body.Close()
	restoreBody, _ := io.ReadAll(respRestore.Body)
	if !strings.Contains(string(restoreBody), "setwebproxystate") {
		t.Errorf("expected setwebproxystate in restore script, got: %s", string(restoreBody))
	}

	// 4. Test /status
	respStatus, err := http.Get(baseURL + "/status")
	if err != nil {
		t.Fatalf("GET /status failed: %v", err)
	}
	defer respStatus.Body.Close()
	if respStatus.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", respStatus.StatusCode)
	}
}

func TestHTTPForwarding(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Echo", "netbridge-ok")
		_, _ = w.Write([]byte("hello from upstream target"))
	}))
	defer backend.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	server := NewHTTPServer(ServerConfig{
		Host:     "127.0.0.1",
		HTTPPort: listener.Addr().(*net.TCPAddr).Port,
	})
	server.httpServer.Addr = listener.Addr().String()

	go func() {
		_ = server.httpServer.Serve(listener)
	}()
	defer func() {
		_ = server.Shutdown(context.Background())
	}()

	time.Sleep(50 * time.Millisecond)

	req, err := http.NewRequest(http.MethodGet, backend.URL+"/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	proxyURL, _ := url.Parse("http://" + listener.Addr().String())
	proxyClient := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: 5 * time.Second,
	}

	resp, err := proxyClient.Do(req)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello from upstream target" {
		t.Errorf("unexpected body: %q", string(body))
	}
	if resp.Header.Get("X-Custom-Echo") != "netbridge-ok" {
		t.Errorf("expected custom header to pass through")
	}
}

func TestHTTPConnectTunnel(t *testing.T) {
	// Mock TCP echo server
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
				_, _ = c.Write(append([]byte("ECHO:"), buf[:n]...))
			}(conn)
		}
	}()

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start proxy listener: %v", err)
	}
	defer proxyListener.Close()

	server := NewHTTPServer(ServerConfig{
		Host:     "127.0.0.1",
		HTTPPort: proxyListener.Addr().(*net.TCPAddr).Port,
	})
	server.httpServer.Addr = proxyListener.Addr().String()

	go func() {
		_ = server.httpServer.Serve(proxyListener)
	}()
	defer func() {
		_ = server.Shutdown(context.Background())
	}()

	time.Sleep(50 * time.Millisecond)

	// Dial the proxy and issue CONNECT
	proxyConn, err := net.Dial("tcp", proxyListener.Addr().String())
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer proxyConn.Close()

	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", targetListener.Addr().String(), targetListener.Addr().String())
	if _, err := proxyConn.Write([]byte(connectReq)); err != nil {
		t.Fatalf("failed to write CONNECT request: %v", err)
	}

	respBuf := make([]byte, 1024)
	n, err := proxyConn.Read(respBuf)
	if err != nil {
		t.Fatalf("failed to read CONNECT response: %v", err)
	}
	if !strings.Contains(string(respBuf[:n]), "200 Connection Established") {
		t.Fatalf("unexpected CONNECT response: %s", string(respBuf[:n]))
	}

	// Send raw data through the established tunnel
	testPayload := "hello-tunnel"
	if _, err := proxyConn.Write([]byte(testPayload)); err != nil {
		t.Fatalf("failed to write payload through tunnel: %v", err)
	}

	dataBuf := make([]byte, 1024)
	n, err = proxyConn.Read(dataBuf)
	if err != nil {
		t.Fatalf("failed to read data from tunnel: %v", err)
	}
	if string(dataBuf[:n]) != "ECHO:hello-tunnel" {
		t.Errorf("expected 'ECHO:hello-tunnel', got %q", string(dataBuf[:n]))
	}
}


