package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestGenerateTerminalQRCode(t *testing.T) {
	qr, err := GenerateTerminalQRCode("http://192.168.1.100:8888/proxy.pac")
	if err != nil {
		t.Fatalf("GenerateTerminalQRCode failed: %v", err)
	}
	if len(qr) == 0 {
		t.Fatal("expected non-empty QR code string")
	}
}

func TestRenderDashboard(t *testing.T) {
	var buf bytes.Buffer
	info := ServerInfo{
		Host:        "192.168.1.50",
		BonjourHost: "my-mac.local",
		HTTPPort:    8888,
		SOCKSPort:   1080,
		PACURL:      "http://192.168.1.50:8888/proxy.pac",
		PortalURL:   "http://192.168.1.50:8888/connect",
		SetupCmd:    "curl -fsSL http://192.168.1.50:8888/setup.sh | bash",
	}

	RenderDashboard(&buf, info)
	output := buf.String()

	if !strings.Contains(output, "NetBridge") {
		t.Errorf("expected dashboard to contain 'NetBridge'")
	}
	if !strings.Contains(output, "192.168.1.50:8888") {
		t.Errorf("expected dashboard to contain host and port")
	}
	if !strings.Contains(output, "socks5://192.168.1.50:1080") {
		t.Errorf("expected dashboard to contain socks port")
	}
	if !strings.Contains(output, "http://192.168.1.50:8888/connect") {
		t.Errorf("expected dashboard to contain portal url")
	}
	if !strings.Contains(output, "my-mac.local") {
		t.Errorf("expected dashboard to contain Bonjour host")
	}
	if !strings.Contains(output, "USB-C / Cable: curl -fsSL http://my-mac.local:8888/setup.sh | bash") {
		t.Errorf("expected dashboard to contain USB-C cable setup command")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes uint64
		want  string
	}{
		{500, "500 B"},
		{1024, "1.0 KiB"},
		{1048576, "1.0 MiB"},
		{1073741824, "1.0 GiB"},
	}

	for _, tt := range tests {
		got := FormatBytes(tt.bytes)
		if got != tt.want {
			t.Errorf("FormatBytes(%d) = %q; want %q", tt.bytes, got, tt.want)
		}
	}
}
