// Package proxy implements HTTP, HTTPS CONNECT, and SOCKS5 proxy engines.
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
	case "/connect", "/":
		s.handleConnectPortal(w, r)
		return
	case "/api/ping":
		s.handlePingJSON(w, r)
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

func (s *HTTPServer) handlePingJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok","message":"Connected to NetBridge Gateway"}`))
}

func (s *HTTPServer) handleConnectPortal(w http.ResponseWriter, r *http.Request) {
	host := s.determineHost(r)
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>NetBridge Setup Portal</title>
<style>
:root {
  --bg: #0f172a;
  --card: #1e293b;
  --card-border: #334155;
  --text: #f8fafc;
  --muted: #94a3b8;
  --accent: #38bdf8;
  --accent-hover: #0ea5e9;
  --success: #22c55e;
}
* { box-sizing: border-box; margin: 0; padding: 0; }
body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  background-color: var(--bg);
  color: var(--text);
  line-height: 1.5;
  padding: 20px 16px;
}
.container { max-width: 520px; margin: 0 auto; }
.header { text-align: center; margin-bottom: 24px; }
.title { font-size: 22px; font-weight: 700; margin-bottom: 4px; }
.badge { display: inline-block; background: rgba(34, 197, 94, 0.2); color: var(--success); padding: 4px 10px; border-radius: 999px; font-size: 13px; font-weight: 600; margin-top: 6px; }
.notice { background: rgba(56, 189, 248, 0.1); border: 1px solid rgba(56, 189, 248, 0.3); border-radius: 10px; padding: 14px; margin-bottom: 20px; font-size: 14px; color: #bae6fd; }
.card { background: var(--card); border: 1px solid var(--card-border); border-radius: 12px; padding: 18px; margin-bottom: 20px; }
.card-title { font-size: 16px; font-weight: 600; margin-bottom: 12px; }
.param-row { display: flex; justify-content: space-between; align-items: center; padding: 10px 0; border-bottom: 1px solid var(--card-border); }
.param-row:last-child { border-bottom: none; }
.param-label { font-size: 14px; color: var(--muted); }
.param-value { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 15px; font-weight: 600; }
.btn-copy { background: rgba(56, 189, 248, 0.15); color: var(--accent); border: 1px solid var(--accent); border-radius: 6px; padding: 6px 12px; font-size: 12px; font-weight: 600; cursor: pointer; min-height: 36px; min-width: 60px; }
.btn-copy:active { background: var(--accent); color: #000; }
.tabs { display: flex; gap: 8px; margin-bottom: 14px; }
.tab-btn { flex: 1; padding: 10px; border: 1px solid var(--card-border); background: #1e293b; color: var(--text); border-radius: 8px; font-size: 14px; font-weight: 600; cursor: pointer; text-align: center; }
.tab-btn.active { background: var(--accent); color: #0f172a; border-color: var(--accent); }
.guide-step { display: flex; gap: 12px; margin-bottom: 14px; align-items: flex-start; }
.step-num { background: rgba(255, 255, 255, 0.1); width: 26px; height: 26px; border-radius: 50%%; display: flex; align-items: center; justify-content: center; font-size: 13px; font-weight: 700; flex-shrink: 0; }
.step-text { font-size: 14px; color: #cbd5e1; }
.step-text strong { color: #fff; }
.test-btn { width: 100%%; background: var(--accent); color: #0f172a; border: none; border-radius: 8px; padding: 12px; font-size: 15px; font-weight: 700; cursor: pointer; min-height: 44px; margin-top: 10px; }
.test-result { margin-top: 10px; font-size: 13px; text-align: center; }
</style>
</head>
<body>
<div class="container">
  <div class="header">
    <h1 class="title">NetBridge Gateway</h1>
    <div class="badge">Online and Ready</div>
  </div>

  <div class="notice">
    <strong>Notice:</strong> Your device must stay connected to the same Wi-Fi network as the Host Mac for traffic forwarding to function.
  </div>

  <div class="card">
    <div class="card-title">Proxy Connection Parameters</div>
    <div class="param-row">
      <div>
        <div class="param-label">Server / Host</div>
        <div class="param-value" id="val-host">%s</div>
      </div>
      <button class="btn-copy" onclick="copyText('val-host', this)">Copy</button>
    </div>
    <div class="param-row">
      <div>
        <div class="param-label">Port</div>
        <div class="param-value" id="val-port">%d</div>
      </div>
      <button class="btn-copy" onclick="copyText('val-port', this)">Copy</button>
    </div>
    <div class="param-row">
      <div>
        <div class="param-label">SOCKS5 Port</div>
        <div class="param-value" id="val-socks">%d</div>
      </div>
      <button class="btn-copy" onclick="copyText('val-socks', this)">Copy</button>
    </div>
  </div>

  <div class="card">
    <div class="card-title">Step-by-Step Device Setup</div>
    <div class="tabs">
      <button class="tab-btn active" onclick="selectTab('ios', this)">iOS (iPhone)</button>
      <button class="tab-btn" onclick="selectTab('android', this)">Android</button>
      <button class="tab-btn" onclick="selectTab('windows', this)">Windows</button>
    </div>

    <div id="guide-ios">
      <div class="guide-step">
        <div class="step-num">1</div>
        <div class="step-text">Open <strong>Settings > Wi-Fi</strong>, and tap the blue <strong>(i)</strong> icon next to your connected network.</div>
      </div>
      <div class="guide-step">
        <div class="step-num">2</div>
        <div class="step-text">Scroll down to <strong>HTTP PROXY</strong> and select <strong>Configure Proxy > Manual</strong>.</div>
      </div>
      <div class="guide-step">
        <div class="step-num">3</div>
        <div class="step-text">Paste the Server (<strong>%s</strong>) and Port (<strong>%d</strong>) from above, then tap <strong>Save</strong>.</div>
      </div>
    </div>

    <div id="guide-android" style="display:none;">
      <div class="guide-step">
        <div class="step-num">1</div>
        <div class="step-text">Open <strong>Settings > Network & internet > Wi-Fi</strong>, then tap the gear or edit icon on your connected network.</div>
      </div>
      <div class="guide-step">
        <div class="step-num">2</div>
        <div class="step-text">Expand <strong>Advanced options</strong>, find <strong>Proxy</strong>, and switch it to <strong>Manual</strong>.</div>
      </div>
      <div class="guide-step">
        <div class="step-num">3</div>
        <div class="step-text">Enter Proxy hostname (<strong>%s</strong>) and Proxy port (<strong>%d</strong>), then tap <strong>Save</strong>.</div>
      </div>
    </div>

    <div id="guide-windows" style="display:none;">
      <div class="guide-step">
        <div class="step-num">1</div>
        <div class="step-text">Open <strong>Settings > Network & internet > Proxy</strong>.</div>
      </div>
      <div class="guide-step">
        <div class="step-num">2</div>
        <div class="step-text">Under <strong>Automatic proxy setup</strong>, turn on <strong>Use setup script</strong>.</div>
      </div>
      <div class="guide-step">
        <div class="step-num">3</div>
        <div class="step-text">Paste the PAC Script URL: <strong>http://%s:%d/proxy.pac</strong>, then click <strong>Save</strong>.</div>
      </div>
    </div>

    <button class="test-btn" onclick="testConnection()">Verify Connection Status</button>
    <div id="test-result" class="test-result"></div>
  </div>
</div>

<script>
function copyText(elementId, btn) {
  const text = document.getElementById(elementId).innerText.trim();
  navigator.clipboard.writeText(text).then(() => {
    const original = btn.innerText;
    btn.innerText = "Copied!";
    setTimeout(() => { btn.innerText = original; }, 1500);
  });
}

function selectTab(tab, btn) {
  document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  document.getElementById('guide-ios').style.display = tab === 'ios' ? 'block' : 'none';
  document.getElementById('guide-android').style.display = tab === 'android' ? 'block' : 'none';
  document.getElementById('guide-windows').style.display = tab === 'windows' ? 'block' : 'none';
}

function testConnection() {
  const resultDiv = document.getElementById('test-result');
  resultDiv.innerHTML = '<span style="color:var(--muted)">Checking connectivity...</span>';
  fetch('/api/ping')
    .then(res => res.json())
    .then(data => {
      if (data.status === 'ok') {
        resultDiv.innerHTML = '<span style="color:var(--success);font-weight:600;">Success: Your device is communicating with NetBridge Gateway!</span>';
      }
    })
    .catch(err => {
      resultDiv.innerHTML = '<span style="color:#ef4444;font-weight:600;">Failed: Ensure you are connected to the same Wi-Fi network.</span>';
    });
}

// Auto-select tab by user agent
(function() {
  const ua = navigator.userAgent || '';
  if (/android/i.test(ua)) {
    const androidTab = document.querySelectorAll('.tab-btn')[1];
    if (androidTab) selectTab('android', androidTab);
  }
})();
</script>
</body>
</html>`, host, s.config.HTTPPort, s.config.SOCKSPort, host, s.config.HTTPPort, host, s.config.HTTPPort, host, s.config.HTTPPort)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
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
