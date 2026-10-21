// Package proxy implements HTTP, HTTPS CONNECT, and SOCKS5 proxy engines.
package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const tunnelBufferSize = 128 * 1024

var bufferPool = sync.Pool{
	New: func() any {
		b := make([]byte, tunnelBufferSize)
		return &b
	},
}

// optimizeTCPConn sets TCP_NODELAY and socket buffer sizes for high-throughput streaming.
func optimizeTCPConn(c net.Conn) {
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
		_ = tc.SetKeepAlive(true)
		_ = tc.SetReadBuffer(tunnelBufferSize)
		_ = tc.SetWriteBuffer(tunnelBufferSize)
	}
}

// ServerConfig defines initialization parameters for the HTTP proxy server.
type ServerConfig struct {
	Host      string
	HTTPPort  int
	SOCKSPort int
	Verbose   bool
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

	// Intercept active proxy verification probe
	if strings.Contains(r.Host, "probe.netbridge") || strings.Contains(r.URL.Host, "probe.netbridge") {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"verified":true,"client_ip":"` + clientIP + `"}`))
		log.Printf("[PROBE] %s validated active proxy routing", clientIP)
		return
	}

	if r.Method == http.MethodConnect {
		s.metrics.RecordConnection(clientIP)
		defer s.metrics.ReleaseConnection()
		s.handleConnect(w, r, clientIP)
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
	s.handleStandardHTTP(w, r, clientIP)
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

func (s *HTTPServer) handleConnect(w http.ResponseWriter, r *http.Request, clientIP string) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "webserver does not support hijacking", http.StatusInternalServerError)
		return
	}

	log.Printf("[CONNECT] %s -> %s", clientIP, r.Host)
	start := time.Now()

	targetConn, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
	if err != nil {
		log.Printf("[FAILED] %s -> %s: %v", clientIP, r.Host, err)
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

	optimizeTCPConn(targetConn)
	optimizeTCPConn(clientConn)

	_, err = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		return
	}

	up, down := s.tunnel(clientConn, targetConn)
	duration := time.Since(start).Round(time.Millisecond)
	log.Printf("[CLOSE] %s <- %s (Up: %s, Down: %s, Duration: %s)", clientIP, r.Host, FormatBytes(up), FormatBytes(down), duration)
}

func (s *HTTPServer) handleStandardHTTP(w http.ResponseWriter, r *http.Request, clientIP string) {
	if !r.URL.IsAbs() {
		http.Error(w, "this is an http/https proxy server. connect to target urls via proxy settings or visit /setup.sh", http.StatusBadRequest)
		return
	}

	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""
	removeHopByHopHeaders(outReq.Header)

	resp, err := s.transport.RoundTrip(outReq)
	if err != nil {
		log.Printf("[HTTP_ERROR] %s %s -> %s: %v", clientIP, r.Method, r.URL.String(), err)
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
	log.Printf("[HTTP] %s %s -> %s (%d, %s)", clientIP, r.Method, r.URL.Host, resp.StatusCode, FormatBytes(uint64(written)))
}

func (s *HTTPServer) tunnel(c1, c2 net.Conn) (uint64, uint64) {
	var wg sync.WaitGroup
	wg.Add(2)
	var upBytes, downBytes uint64

	pipe := func(dst, src net.Conn, isUpload bool) {
		defer wg.Done()
		bufPtr := bufferPool.Get().(*[]byte)
		defer bufferPool.Put(bufPtr)

		n, _ := io.CopyBuffer(dst, src, *bufPtr)
		if isUpload {
			atomic.AddUint64(&upBytes, uint64(n))
			s.metrics.AddBytes(uint64(n), 0)
		} else {
			atomic.AddUint64(&downBytes, uint64(n))
			s.metrics.AddBytes(0, uint64(n))
		}
		if tc, ok := dst.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}

	go pipe(c2, c1, true)
	go pipe(c1, c2, false)

	wg.Wait()
	return upBytes, downBytes
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
	speedIn, speedOut := s.metrics.GetSpeed()
	status := map[string]interface{}{
		"status":          "running",
		"http_port":       s.config.HTTPPort,
		"socks_port":      s.config.SOCKSPort,
		"total_requests":  s.metrics.TotalRequests,
		"active_conns":    s.metrics.ActiveConns,
		"bytes_in":        s.metrics.BytesIn,
		"bytes_out":       s.metrics.BytesOut,
		"speed_in_bps":    speedIn,
		"speed_out_bps":   speedOut,
		"speed_in_human":  FormatBytes(speedIn) + "/s",
		"speed_out_human": FormatBytes(speedOut) + "/s",
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
<title>NetBridge Gateway Setup</title>
<style>
:root {
  --bg: #090d16;
  --card: #131b2e;
  --card-border: #233354;
  --text: #f1f5f9;
  --text-muted: #94a3b8;
  --accent: #38bdf8;
  --accent-hover: #0284c7;
  --success: #22c55e;
  --danger: #ef4444;
}
* { box-sizing: border-box; margin: 0; padding: 0; }
body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  background-color: var(--bg);
  color: var(--text);
  line-height: 1.5;
  padding: 24px 16px;
  -webkit-font-smoothing: antialiased;
}
.container { max-width: 580px; margin: 0 auto; }
.header { text-align: center; margin-bottom: 24px; }
.brand { font-size: 20px; font-weight: 700; letter-spacing: -0.02em; color: var(--text); }
.status-pill { display: inline-flex; align-items: center; gap: 6px; background: rgba(34, 197, 94, 0.15); border: 1px solid rgba(34, 197, 94, 0.35); color: #86efac; padding: 4px 12px; border-radius: 999px; font-size: 13px; font-weight: 600; margin-top: 8px; }
.status-dot { width: 8px; height: 8px; border-radius: 50%%; background: var(--success); }
.callout { background: rgba(56, 189, 248, 0.08); border: 1px solid rgba(56, 189, 248, 0.25); border-radius: 8px; padding: 12px 14px; margin-bottom: 20px; font-size: 13px; color: #bae6fd; }
.card { background: var(--card); border: 1px solid var(--card-border); border-radius: 12px; padding: 20px; margin-bottom: 20px; }
.card-header { font-size: 15px; font-weight: 600; margin-bottom: 14px; color: var(--text); }
.param-list { display: flex; flex-direction: column; gap: 10px; }
.param-item { display: flex; justify-content: space-between; align-items: center; background: rgba(0, 0, 0, 0.25); border: 1px solid rgba(255, 255, 255, 0.05); padding: 10px 12px; border-radius: 8px; }
.param-label { font-size: 13px; color: var(--text-muted); }
.param-value { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 15px; font-weight: 600; color: #fff; }
.btn-copy { background: rgba(56, 189, 248, 0.15); color: var(--accent); border: 1px solid rgba(56, 189, 248, 0.4); border-radius: 6px; padding: 8px 14px; font-size: 13px; font-weight: 600; cursor: pointer; min-height: 44px; min-width: 68px; }
.btn-copy:active { background: var(--accent); color: #000; }
.tabs { display: grid; grid-template-columns: repeat(5, 1fr); gap: 6px; margin-bottom: 16px; }
.tab-btn { padding: 10px 4px; border: 1px solid var(--card-border); background: rgba(0, 0, 0, 0.2); color: var(--text-muted); border-radius: 8px; font-size: 13px; font-weight: 600; cursor: pointer; text-align: center; min-height: 44px; }
.tab-btn.active { background: var(--accent); color: #090d16; border-color: var(--accent); font-weight: 700; }
.guide-content { font-size: 14px; }
.guide-step { display: flex; gap: 12px; margin-bottom: 14px; align-items: flex-start; }
.step-index { background: rgba(255, 255, 255, 0.08); color: var(--text); width: 24px; height: 24px; border-radius: 50%%; display: flex; align-items: center; justify-content: center; font-size: 12px; font-weight: 700; flex-shrink: 0; }
.step-body { color: #cbd5e1; line-height: 1.45; }
.step-body strong { color: #fff; }
.code-block { background: #0b1120; border: 1px solid var(--card-border); padding: 10px 12px; border-radius: 6px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; color: #38bdf8; word-break: break-all; margin-top: 6px; }
.btn-verify { width: 100%%; background: var(--accent); color: #090d16; border: none; border-radius: 8px; padding: 12px; font-size: 15px; font-weight: 700; cursor: pointer; min-height: 48px; margin-top: 8px; }
.btn-verify:active { background: var(--accent-hover); }
.verify-result { margin-top: 12px; font-size: 13px; text-align: center; line-height: 1.4; }
</style>
</head>
<body>
<div class="container">
  <div class="header">
    <div class="brand">NetBridge Gateway</div>
    <div class="status-pill"><span class="status-dot"></span> Active and Forwarding</div>
  </div>

  <div class="callout">
    <strong>Connection Requirement:</strong> Your device must be connected to the same Wi-Fi network as the Mac gateway.
  </div>

  <div class="card">
    <div class="card-header">Proxy Parameters</div>
    <div class="param-list">
      <div class="param-item">
        <div>
          <div class="param-label">Server / Host IP</div>
          <div class="param-value" id="val-host">%s</div>
        </div>
        <button class="btn-copy" onclick="copyValue('val-host', this)">Copy</button>
      </div>
      <div class="param-item">
        <div>
          <div class="param-label">HTTP / HTTPS Port</div>
          <div class="param-value" id="val-port">%d</div>
        </div>
        <button class="btn-copy" onclick="copyValue('val-port', this)">Copy</button>
      </div>
      <div class="param-item">
        <div>
          <div class="param-label">SOCKS5 Port</div>
          <div class="param-value" id="val-socks">%d</div>
        </div>
        <button class="btn-copy" onclick="copyValue('val-socks', this)">Copy</button>
      </div>
    </div>
  </div>

  <div class="card">
    <div class="card-header">Setup Instructions for Your Device</div>
    <div class="tabs">
      <button class="tab-btn" id="tab-ios" onclick="selectTab('ios', this)">iOS</button>
      <button class="tab-btn" id="tab-android" onclick="selectTab('android', this)">Android</button>
      <button class="tab-btn" id="tab-macos" onclick="selectTab('macos', this)">macOS</button>
      <button class="tab-btn" id="tab-linux" onclick="selectTab('linux', this)">Linux</button>
      <button class="tab-btn" id="tab-windows" onclick="selectTab('windows', this)">Windows</button>
    </div>

    <div class="guide-content">
      <!-- iOS Guide -->
      <div id="guide-ios" style="display:none;">
        <div class="guide-step">
          <div class="step-index">1</div>
          <div class="step-body">Open <strong>Settings > Wi-Fi</strong> and tap the <strong>(i)</strong> icon next to your network.</div>
        </div>
        <div class="guide-step">
          <div class="step-index">2</div>
          <div class="step-body">Scroll to bottom and select <strong>Configure Proxy > Manual</strong>.</div>
        </div>
        <div class="guide-step">
          <div class="step-index">3</div>
          <div class="step-body">Enter Server (<strong>%s</strong>) and Port (<strong>%d</strong>), then tap <strong>Save</strong>.</div>
        </div>
      </div>

      <!-- Android Guide -->
      <div id="guide-android" style="display:none;">
        <div class="guide-step">
          <div class="step-index">1</div>
          <div class="step-body">Open <strong>Settings > Wi-Fi</strong> and tap the gear or edit icon on your network.</div>
        </div>
        <div class="guide-step">
          <div class="step-index">2</div>
          <div class="step-body">Expand <strong>Advanced options</strong> and set <strong>Proxy</strong> to <strong>Manual</strong>.</div>
        </div>
        <div class="guide-step">
          <div class="step-index">3</div>
          <div class="step-body">Enter Host (<strong>%s</strong>) and Port (<strong>%d</strong>), then tap <strong>Save</strong>.</div>
        </div>
      </div>

      <!-- macOS Guide -->
      <div id="guide-macos" style="display:none;">
        <div class="guide-step">
          <div class="step-index">1</div>
          <div class="step-body">Open Terminal on the client Mac and run the one-line auto setup script:
            <div class="code-block" id="cmd-macos">curl -fsSL http://%s:%d/setup.sh | bash</div>
          </div>
        </div>
        <div class="guide-step">
          <div class="step-index">2</div>
          <div class="step-body">To revert when finished:
            <div class="code-block">curl -fsSL http://%s:%d/restore.sh | bash</div>
          </div>
        </div>
      </div>

      <!-- Linux Guide -->
      <div id="guide-linux" style="display:none;">
        <div class="guide-step">
          <div class="step-index">1</div>
          <div class="step-body">Export environment variables in terminal or profile:
            <div class="code-block">export http_proxy="http://%s:%d"<br>export https_proxy="http://%s:%d"</div>
          </div>
        </div>
      </div>

      <!-- Windows Guide -->
      <div id="guide-windows" style="display:none;">
        <div class="guide-step">
          <div class="step-index">1</div>
          <div class="step-body">Open <strong>Settings > Network & internet > Proxy</strong>.</div>
        </div>
        <div class="guide-step">
          <div class="step-index">2</div>
          <div class="step-body">Under <strong>Automatic proxy setup</strong>, enable <strong>Use setup script</strong>.</div>
        </div>
        <div class="guide-step">
          <div class="step-index">3</div>
          <div class="step-body">Paste Script address: <strong>http://%s:%d/proxy.pac</strong> and click <strong>Save</strong>.</div>
        </div>
      </div>
    </div>

    <button class="btn-verify" onclick="verifyProxyConnection()">Verify Active Proxy Connection</button>
    <div id="verify-output" class="verify-result"></div>
  </div>
</div>

<script>
function copyValue(elementId, btn) {
  const text = document.getElementById(elementId).innerText.trim();
  const fallback = () => {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.left = "-9999px";
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    try {
      document.execCommand('copy');
      showCopied(btn);
    } catch (e) {
      prompt("Copy to clipboard:", text);
    }
    document.body.removeChild(ta);
  };

  if (navigator.clipboard && window.isSecureContext) {
    navigator.clipboard.writeText(text).then(() => showCopied(btn)).catch(fallback);
  } else {
    fallback();
  }
}

function showCopied(btn) {
  const original = btn.innerText;
  btn.innerText = "Copied!";
  btn.style.color = "var(--success)";
  btn.style.borderColor = "var(--success)";
  setTimeout(() => {
    btn.innerText = original;
    btn.style.color = "";
    btn.style.borderColor = "";
  }, 1400);
}

function selectTab(osKey, btn) {
  document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
  if (btn) btn.classList.add('active');
  ['ios', 'android', 'macos', 'linux', 'windows'].forEach(k => {
    const el = document.getElementById('guide-' + k);
    if (el) el.style.display = (k === osKey) ? 'block' : 'none';
  });
}

function verifyProxyConnection() {
  const out = document.getElementById('verify-output');
  out.innerHTML = '<span style="color:var(--text-muted)">Testing active proxy routing...</span>';

  const ctrl = new AbortController();
  const timeoutId = setTimeout(() => ctrl.abort(), 3500);

  // probe.netbridge is a virtual domain intercepted ONLY when routing through NetBridge proxy.
  // If proxy is not configured, the device attempts DNS resolution against public DNS and fails.
  fetch('http://probe.netbridge/test', { signal: ctrl.signal, cache: 'no-store' })
    .then(r => r.json())
    .then(data => {
      clearTimeout(timeoutId);
      if (data.verified) {
        out.innerHTML = '<div style="padding:10px;border-radius:8px;background:rgba(34,197,94,0.15);border:1px solid #22c55e;color:#86efac;font-weight:600;">Success: Your device traffic is actively routing through NetBridge!</div>';
      }
    })
    .catch(() => {
      clearTimeout(timeoutId);
      out.innerHTML = '<div style="padding:10px;border-radius:8px;background:rgba(239,68,68,0.15);border:1px solid #ef4444;color:#fca5a5;font-weight:600;">Not Active: Device is not routing through proxy yet. Please check Wi-Fi proxy settings above.</div>';
    });
}

// Auto-detect client operating system on load
(function() {
  const ua = (navigator.userAgent || '').toLowerCase();
  let defaultTab = 'ios';
  if (/iphone|ipad|ipod/.test(ua)) {
    defaultTab = 'ios';
  } else if (/android/.test(ua)) {
    defaultTab = 'android';
  } else if (/macintosh|mac os x/.test(ua)) {
    defaultTab = 'macos';
  } else if (/linux/.test(ua)) {
    defaultTab = 'linux';
  } else if (/windows/.test(ua)) {
    defaultTab = 'windows';
  }
  const btn = document.getElementById('tab-' + defaultTab);
  selectTab(defaultTab, btn);
})();
</script>
</body>
</html>`, host, s.config.HTTPPort, s.config.SOCKSPort, host, s.config.HTTPPort, host, s.config.HTTPPort, host, s.config.HTTPPort, host, s.config.HTTPPort, host, s.config.HTTPPort, host, s.config.HTTPPort, host, s.config.HTTPPort)

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
