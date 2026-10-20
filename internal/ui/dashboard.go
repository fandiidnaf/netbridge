package ui

import (
	"fmt"
	"io"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// ServerInfo holds display data for the dashboard.
type ServerInfo struct {
	Host      string
	HTTPPort  int
	SOCKSPort int
	PACURL    string
	SetupCmd  string
}

// GenerateTerminalQRCode creates a compact ANSI QR code suitable for terminal screens.
func GenerateTerminalQRCode(content string) (string, error) {
	qr, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return "", err
	}
	// ToSmallString(false) uses unicode half-blocks for compact display.
	return qr.ToSmallString(false), nil
}

// RenderDashboard prints a structured status board to the output writer.
func RenderDashboard(w io.Writer, info ServerInfo) {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString("NetBridge — User-Space Network Sharing Gateway\n")
	b.WriteString("================================================================\n")
	b.WriteString(fmt.Sprintf("Host Address:     %s\n", info.Host))
	b.WriteString(fmt.Sprintf("HTTP/HTTPS Proxy: http://%s:%d\n", info.Host, info.HTTPPort))
	if info.SOCKSPort > 0 {
		b.WriteString(fmt.Sprintf("SOCKS5 Proxy:     socks5://%s:%d\n", info.Host, info.SOCKSPort))
	}
	b.WriteString(fmt.Sprintf("PAC Auto-Config:  %s\n", info.PACURL))
	b.WriteString("----------------------------------------------------------------\n")
	b.WriteString("Client Quick-Connect:\n")
	b.WriteString(fmt.Sprintf("  • Mac / Linux:  %s\n", info.SetupCmd))
	b.WriteString(fmt.Sprintf("  • Disconnect:   curl -fsSL http://%s:%d/restore.sh | bash\n", info.Host, info.HTTPPort))
	b.WriteString("  • Windows:      Enable 'Use setup script' and paste PAC URL above\n")
	b.WriteString("  • Smartphone:   Connect to the same Wi-Fi, then scan QR code below\n")
	b.WriteString("----------------------------------------------------------------\n\n")

	// QR code encodes the PAC URL for easy proxy auto-discovery
	qrCode, err := GenerateTerminalQRCode(info.PACURL)
	if err == nil {
		b.WriteString(qrCode)
		b.WriteString("\n")
	}

	b.WriteString("Proxy gateway is active. Press Ctrl+C to terminate.\n")
	b.WriteString("================================================================\n\n")

	_, _ = io.WriteString(w, b.String())
}

// FormatBytes formats byte counts into human-readable binary sizes.
func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
