package network

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// ProxyStatus describes the active state of macOS network proxies.
type ProxyStatus struct {
	Service     string
	HTTPEnabled bool
	HTTPHost    string
	HTTPPort    int
	SOCKSEnabled bool
	SOCKSHost   string
	SOCKSPort   int
}

// MacOSProxyManager manages system-level proxy configurations on macOS
// by wrapping the official networksetup utility.
type MacOSProxyManager struct {
	Service string
}

// NewMacOSProxyManager creates a manager for the primary active network service.
func NewMacOSProxyManager(serviceOverride string) (*MacOSProxyManager, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("macos proxy management is only supported on darwin (current: %s)", runtime.GOOS)
	}

	service := strings.TrimSpace(serviceOverride)
	if service == "" {
		detected, err := DetectPrimaryNetworkService()
		if err != nil {
			return nil, fmt.Errorf("failed to detect primary network service: %w", err)
		}
		service = detected
	}

	return &MacOSProxyManager{Service: service}, nil
}

// DetectPrimaryNetworkService identifies the active default network service.
func DetectPrimaryNetworkService() (string, error) {
	out, err := exec.Command("networksetup", "-listnetworkserviceorder").Output()
	if err != nil {
		return "Wi-Fi", nil
	}

	lines := strings.Split(string(out), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "(1) ") || strings.HasPrefix(line, "(2) ") {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) == 2 {
				serviceName := strings.TrimSpace(parts[1])
				if i+1 < len(lines) && strings.Contains(lines[i+1], "Device: en") {
					return serviceName, nil
				}
			}
		}
	}

	return "Wi-Fi", nil
}

// Enable configures system web and SOCKS proxies on the target service.
func (m *MacOSProxyManager) Enable(host string, httpPort, socksPort int) error {
	if err := runNetworkSetup("-setwebproxy", m.Service, host, strconv.Itoa(httpPort)); err != nil {
		return fmt.Errorf("failed to set web proxy: %w", err)
	}
	if err := runNetworkSetup("-setsecurewebproxy", m.Service, host, strconv.Itoa(httpPort)); err != nil {
		return fmt.Errorf("failed to set secure web proxy: %w", err)
	}

	if socksPort > 0 {
		if err := runNetworkSetup("-setsocksfirewallproxy", m.Service, host, strconv.Itoa(socksPort)); err != nil {
			return fmt.Errorf("failed to set socks proxy: %w", err)
		}
	}

	return nil
}

// Disable turns off system web, secure web, and SOCKS proxies on the service.
func (m *MacOSProxyManager) Disable() error {
	var errs []string

	if err := runNetworkSetup("-setwebproxystate", m.Service, "off"); err != nil {
		errs = append(errs, fmt.Sprintf("web proxy: %v", err))
	}
	if err := runNetworkSetup("-setsecurewebproxystate", m.Service, "off"); err != nil {
		errs = append(errs, fmt.Sprintf("secure web proxy: %v", err))
	}
	if err := runNetworkSetup("-setsocksfirewallproxystate", m.Service, "off"); err != nil {
		errs = append(errs, fmt.Sprintf("socks proxy: %v", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to disable proxies: %s", strings.Join(errs, "; "))
	}

	return nil
}

// Status inspects the current proxy settings on the service.
func (m *MacOSProxyManager) Status() (*ProxyStatus, error) {
	status := &ProxyStatus{Service: m.Service}

	httpOut, err := exec.Command("networksetup", "-getwebproxy", m.Service).Output()
	if err == nil {
		status.HTTPEnabled, status.HTTPHost, status.HTTPPort = parseProxyOutput(string(httpOut))
	}

	socksOut, err := exec.Command("networksetup", "-getsocksfirewallproxy", m.Service).Output()
	if err == nil {
		status.SOCKSEnabled, status.SOCKSHost, status.SOCKSPort = parseProxyOutput(string(socksOut))
	}

	return status, nil
}

func runNetworkSetup(args ...string) error {
	cmd := exec.Command("networksetup", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errOutput := strings.TrimSpace(stderr.String())
		if errOutput != "" {
			return fmt.Errorf("%w: %s", err, errOutput)
		}
		return err
	}
	return nil
}

func parseProxyOutput(output string) (enabled bool, host string, port int) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch strings.ToLower(key) {
		case "enabled":
			enabled = strings.EqualFold(val, "yes")
		case "server":
			host = val
		case "port":
			p, _ := strconv.Atoi(val)
			port = p
		}
	}
	return
}
