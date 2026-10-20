package network

import (
	"testing"
)

func TestGetPreferredOutboundIP(t *testing.T) {
	ip, err := GetPreferredOutboundIP()
	if err != nil {
		t.Skipf("Skipping outbound IP test in isolated environment: %v", err)
	}
	if ip == "" {
		t.Fatal("Expected non-empty IP address")
	}
}

func TestListLocalIPv4Addresses(t *testing.T) {
	ips, err := ListLocalIPv4Addresses()
	if err != nil {
		t.Fatalf("ListLocalIPv4Addresses returned error: %v", err)
	}
	for _, ip := range ips {
		if !IsValidHostAddress(ip) {
			t.Errorf("Expected %s to be a valid host address", ip)
		}
	}
}

func TestIsValidHostAddress(t *testing.T) {
	tests := []struct {
		host  string
		valid bool
	}{
		{"127.0.0.1", true},
		{"192.168.1.1", true},
		{"localhost", true},
		{"", false},
	}

	for _, tt := range tests {
		got := IsValidHostAddress(tt.host)
		if got != tt.valid {
			t.Errorf("IsValidHostAddress(%q) = %v; want %v", tt.host, got, tt.valid)
		}
	}
}

func TestParseProxyOutput(t *testing.T) {
	sampleOutput := `Enabled: Yes
Server: 192.168.1.100
Port: 8888
Authenticated Proxy Enabled: 0
`
	enabled, host, port := parseProxyOutput(sampleOutput)
	if !enabled {
		t.Error("Expected enabled to be true")
	}
	if host != "192.168.1.100" {
		t.Errorf("Expected host '192.168.1.100', got %q", host)
	}
	if port != 8888 {
		t.Errorf("Expected port 8888, got %d", port)
	}
}
