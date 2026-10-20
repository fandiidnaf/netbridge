package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ServerConfig defines initialization parameters for the HTTP proxy server.
type ServerConfig struct {
	Host      string
	HTTPPort  int
	SOCKSPort int
}

// HTTPServer handles both standard HTTP forwarding and HTTPS CONNECT tunneling,
// as well as serving client provisioning endpoints.
type HTTPServer struct {
	config     ServerConfig
	httpServer *http.Server
	metrics    *Metrics
	transport  *http.Transport
}

// NewHTTPServer builds a configured proxy server instance.
func NewHTTPServer(cfg ServerConfig) *HTTPServer {
	if cfg.Host == "" {
		cfg.Host = "0.0.0.0"
	}
	if cfg.HTTPPort <= 0 {
		cfg.HTTPPort = 8888
	}

	transport := &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          200,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	s := &HTTPServer{
		config:    cfg,
		metrics:   NewMetrics(),
		transport: transport,
	}

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.HTTPPort),
		Handler:      s,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return s
}

// ServeHTTP routes incoming proxy and helper requests.
func (s *HTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}

	if r.Method == http.MethodConnect {
		s.metrics.RecordConnection(clientIP)
		defer s.metrics.ReleaseConnection()
		s.handleConnect(w, r)
		return
	}

	switch r.URL.Path {
	case "/proxy.pac":
		s.handlePAC(w, r)
		return
	case "/setup.sh":
		s.handleSetupScript(w, r)
		return
	case "/restore.sh":
		s.handleRestoreScript(w, r)
		return
	case "/status":
		s.handleStatusJSON(w, r)
		return
	}

	s.metrics.RecordConnection(clientIP)
	defer s.metrics.ReleaseConnection()
	s.handleStandardHTTP(w, r)
}

// Start begins listening on the designated network address.
func (s *HTTPServer) Start() error {
	listener, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	return s.httpServer.Serve(listener)
}

// Shutdown gracefully stops the proxy server.
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Metrics returns the active metrics instance.
func (s *HTTPServer) Metrics() *Metrics {
	return s.metrics
}

func (s *HTTPServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "webserver does not support hijacking", http.StatusInternalServerError)
		return
	}

	targetConn, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
	if err != nil {
		http.Error(w, fmt.Sprintf("target unreachable: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer targetConn.Close()

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, fmt.Sprintf("hijacking failed: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer clientConn.Close()

	_, err = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		return
	}

	s.tunnel(clientConn, targetConn)
}

func (s *HTTPServer) handleStandardHTTP(w http.ResponseWriter, r *http.Request) {
	if !r.URL.IsAbs() {
		http.Error(w, "this is an http/https proxy server. connect to target urls via proxy settings or visit /setup.sh", http.StatusBadRequest)
		return
	}

	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""
	removeHopByHopHeaders(outReq.Header)

	resp, err := s.transport.RoundTrip(outReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("forwarding error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	removeHopByHopHeaders(resp.Header)
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	written, _ := io.Copy(w, resp.Body)
	s.metrics.AddBytes(0, uint64(written))
}

func (s *HTTPServer) tunnel(c1, c2 net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	pipe := func(dst, src net.Conn, isUpload bool) {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		n, _ := io.CopyBuffer(dst, src, buf)
		if isUpload {
			s.metrics.AddBytes(uint64(n), 0)
		} else {
			s.metrics.AddBytes(0, uint64(n))
		}
		if tc, ok := dst.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}

	go pipe(c2, c1, true)
	go pipe(c1, c2, false)

	wg.Wait()
}

func (s *HTTPServer) handlePAC(w http.ResponseWriter, r *http.Request) {
	host := s.determineHost(r)
	pacContent := fmt.Sprintf(`function FindProxyForURL(url, host) {
    if (isPlainHostName(host) ||
        shExpMatch(host, "*.local") ||
        isInNet(dnsResolve(host), "10.0.0.0", "255.0.0.0") ||
        isInNet(dnsResolve(host), "172.16.0.0", "255.240.0.0") ||
        isInNet(dnsResolve(host), "192.168.0.0", "255.255.0.0") ||
        isInNet(dnsResolve(host), "127.0.0.0", "255.0.0.0")) {
        return "DIRECT";
    }
    return "PROXY %s:%d; DIRECT";
}
`, host, s.config.HTTPPort)

	w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(pacContent))
}

func (s *HTTPServer) handleSetupScript(w http.ResponseWriter, r *http.Request) {
	host := s.determineHost(r)
	script := fmt.Sprintf(`#!/usr/bin/env bash
set -e

HOST="%s"
HTTP_PORT="%d"
SOCKS_PORT="%d"

echo "Configuring NetBridge proxy on macOS client..."

if command -v networksetup >/dev/null 2>&1; then
    SERVICES=$(networksetup -listallnetworkservices | tail -n +2)
    FOUND=0

    while IFS= read -r SERVICE; do
        if [ -z "$SERVICE" ]; then continue; fi
        if [[ "$SERVICE" == *"*"* ]]; then continue; fi

        echo "Applying proxy to service: $SERVICE"
        networksetup -setwebproxy "$SERVICE" "$HOST" "$HTTP_PORT"
        networksetup -setsecurewebproxy "$SERVICE" "$HOST" "$HTTP_PORT"
        if [ "$SOCKS_PORT" -gt 0 ]; then
            networksetup -setsocksfirewallproxy "$SERVICE" "$HOST" "$SOCKS_PORT"
        fi
        FOUND=1
    done <<< "$SERVICES"

    if [ "$FOUND" -eq 1 ]; then
        echo "Proxy setup completed successfully!"
        echo "Traffic is now routed through $HOST:$HTTP_PORT"
        echo "To disable, run: curl -fsSL http://$HOST:$HTTP_PORT/restore.sh | bash"
    fi
else
    export http_proxy="http://$HOST:$HTTP_PORT"
    export https_proxy="http://$HOST:$HTTP_PORT"
    echo "Exported shell proxy variables for http/https to $HOST:$HTTP_PORT"
fi
`, host, s.config.HTTPPort, s.config.SOCKSPort)

	w.Header().Set("Content-Type", "text/x-shellscript")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(script))
}

func (s *HTTPServer) handleRestoreScript(w http.ResponseWriter, r *http.Request) {
	script := `#!/usr/bin/env bash
set -e

echo "Restoring macOS network proxy settings..."

if command -v networksetup >/dev/null 2>&1; then
    SERVICES=$(networksetup -listallnetworkservices | tail -n +2)
    while IFS= read -r SERVICE; do
        if [ -z "$SERVICE" ]; then continue; fi
        if [[ "$SERVICE" == *"*"* ]]; then continue; fi

        echo "Disabling proxy on: $SERVICE"
        networksetup -setwebproxystate "$SERVICE" off || true
        networksetup -setsecurewebproxystate "$SERVICE" off || true
        networksetup -setsocksfirewallproxystate "$SERVICE" off || true
    done <<< "$SERVICES"

    echo "Network proxies restored to default."
else
    unset http_proxy https_proxy all_proxy
    echo "Cleared shell proxy variables."
fi
`
	w.Header().Set("Content-Type", "text/x-shellscript")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(script))
}

func (s *HTTPServer) handleStatusJSON(w http.ResponseWriter, _ *http.Request) {
	status := map[string]interface{}{
		"status":          "running",
		"http_port":       s.config.HTTPPort,
		"socks_port":      s.config.SOCKSPort,
		"total_requests":  s.metrics.TotalRequests,
		"active_conns":    s.metrics.ActiveConns,
		"bytes_in":        s.metrics.BytesIn,
		"bytes_out":       s.metrics.BytesOut,
		"recent_clients":  s.metrics.ActiveClientCount(10 * time.Minute),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func (s *HTTPServer) determineHost(r *http.Request) string {
	if s.config.Host != "0.0.0.0" && s.config.Host != "" {
		return s.config.Host
	}
	hostHeader := r.Host
	if host, _, err := net.SplitHostPort(hostHeader); err == nil {
		return host
	}
	return hostHeader
}

func removeHopByHopHeaders(header http.Header) {
	hopHeaders := []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
	}
	for _, h := range hopHeaders {
		header.Del(h)
	}
	if c := header.Get("Connection"); c != "" {
		for _, f := range strings.Split(c, ",") {
			header.Del(strings.TrimSpace(f))
		}
	}
}
