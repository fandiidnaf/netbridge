package network

import (
	"errors"
	"net"
	"strings"
)

// GetPreferredOutboundIP determines the primary local IPv4 address used
// to communicate with external networks. It uses a non-connected UDP socket
// probe so no actual traffic is dispatched.
func GetPreferredOutboundIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		return localAddr.IP.String(), nil
	}

	ips, err := ListLocalIPv4Addresses()
	if err != nil {
		return "", err
	}
	if len(ips) == 0 {
		return "", errors.New("no active non-loopback ipv4 interfaces found")
	}

	return ips[0], nil
}

// ListLocalIPv4Addresses returns all non-loopback IPv4 addresses.
func ListLocalIPv4Addresses() ([]string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var results []string
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP.To4()
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}

			results = append(results, ip.String())
		}
	}

	return results, nil
}

// IsValidHostAddress checks if the host string is a valid IP or resolvable hostname.
func IsValidHostAddress(host string) bool {
	if strings.TrimSpace(host) == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return true
	}
	_, err := net.LookupHost(host)
	return err == nil
}
