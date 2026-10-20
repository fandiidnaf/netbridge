package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/fandiidnaf/netbridge/internal/network"
	"github.com/fandiidnaf/netbridge/internal/proxy"
	"github.com/fandiidnaf/netbridge/internal/ui"
)

var (
	version = "1.0.0"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "serve":
		runServe(args)
	case "join":
		runJoin(args)
	case "leave":
		runLeave(args)
	case "status":
		runStatus(args)
	case "version", "-v", "--version":
		fmt.Printf("netbridge v%s (commit: %s, built: %s)\n", version, commit, date)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`NetBridge: User-Space Network Sharing Gateway for macOS

Usage:
  netbridge <command> [arguments]

Commands:
  serve      Start the network sharing gateway (HTTP/HTTPS + SOCKS5)
  join       Configure local machine to use an existing NetBridge gateway
  leave      Disable all local proxy settings on this machine
  status     Inspect the current system proxy status
  version    Show version and build information

Run 'netbridge <command> --help' for details on a specific command.`)
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	hostFlag := fs.String("host", "", "Host address to announce (defaults to auto-detected local IP)")
	httpPortFlag := fs.Int("http-port", 8888, "Port for HTTP/HTTPS proxy and setup scripts")
	socksPortFlag := fs.Int("socks-port", 1080, "Port for SOCKS5 proxy (0 to disable)")
	quietFlag := fs.Bool("quiet", false, "Suppress banner and QR code output")
	preventSleepFlag := fs.Bool("prevent-sleep", true, "Prevent macOS from sleeping while the gateway is active (via caffeinate)")

	_ = fs.Parse(args)

	// Keep Mac awake while serve is running
	if runtime.GOOS == "darwin" && *preventSleepFlag {
		caffeinateCmd := exec.Command("caffeinate", "-i", "-s", "-w", strconv.Itoa(os.Getpid()))
		if err := caffeinateCmd.Start(); err == nil {
			go func() {
				_ = caffeinateCmd.Wait()
			}()
		}
	}

	announcedHost := *hostFlag
	if announcedHost == "" {
		detected, err := network.GetPreferredOutboundIP()
		if err != nil {
			announcedHost = "127.0.0.1"
		} else {
			announcedHost = detected
		}
	}

	httpCfg := proxy.ServerConfig{
		Host:      "0.0.0.0",
		HTTPPort:  *httpPortFlag,
		SOCKSPort: *socksPortFlag,
	}
	httpServer := proxy.NewHTTPServer(httpCfg)

	var socksServer *proxy.SOCKSServer
	if *socksPortFlag > 0 {
		socksServer = proxy.NewSOCKSServer("0.0.0.0", *socksPortFlag, httpServer.Metrics())
	}

	// Start HTTP Proxy
	go func() {
		if err := httpServer.Start(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "HTTP proxy error: %v\n", err)
			os.Exit(1)
		}
	}()

	// Start SOCKS5 Proxy
	if socksServer != nil {
		go func() {
			if err := socksServer.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "SOCKS5 proxy error: %v\n", err)
			}
		}()
	}

	if !*quietFlag {
		info := ui.ServerInfo{
			Host:      announcedHost,
			HTTPPort:  *httpPortFlag,
			SOCKSPort: *socksPortFlag,
			PACURL:    fmt.Sprintf("http://%s:%d/proxy.pac", announcedHost, *httpPortFlag),
			PortalURL: fmt.Sprintf("http://%s:%d/connect", announcedHost, *httpPortFlag),
			SetupCmd:  fmt.Sprintf("curl -fsSL http://%s:%d/setup.sh | bash", announcedHost, *httpPortFlag),
		}
		ui.RenderDashboard(os.Stdout, info)
	} else {
		fmt.Printf("NetBridge gateway running on %s (HTTP: %d, SOCKS: %d)\n", announcedHost, *httpPortFlag, *socksPortFlag)
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down NetBridge gateway...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = httpServer.Shutdown(ctx)
	if socksServer != nil {
		_ = socksServer.Close()
	}
	fmt.Println("Gateway stopped cleanly.")
}

func runJoin(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: netbridge join <HOST_IP> [HTTP_PORT] [SOCKS_PORT]")
		os.Exit(1)
	}

	host := args[0]
	httpPort := 8888
	socksPort := 1080

	if len(args) >= 2 {
		if p, err := strconv.Atoi(args[1]); err == nil {
			httpPort = p
		}
	}
	if len(args) >= 3 {
		if p, err := strconv.Atoi(args[2]); err == nil {
			socksPort = p
		}
	}

	mgr, err := network.NewMacOSProxyManager("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Configuring proxy on service '%s' to %s:%d...\n", mgr.Service, host, httpPort)
	if err := mgr.Enable(host, httpPort, socksPort); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to enable proxy: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Connected to NetBridge gateway successfully!")
	fmt.Println("Traffic on this machine is now routed through the gateway.")
	fmt.Println("Press Ctrl+C to disconnect and restore default settings.")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nRestoring network settings...")
	if err := mgr.Disable(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to restore settings automatically: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Network settings restored to default.")
}

func runLeave(_ []string) {
	mgr, err := network.NewMacOSProxyManager("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Disabling proxy settings on service '%s'...\n", mgr.Service)
	if err := mgr.Disable(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to disable proxy: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Proxy disabled successfully.")
}

func runStatus(_ []string) {
	mgr, err := network.NewMacOSProxyManager("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	status, err := mgr.Status()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to query status: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Service:      %s\n", status.Service)
	if status.HTTPEnabled {
		fmt.Printf("HTTP Proxy:   Enabled (%s:%d)\n", status.HTTPHost, status.HTTPPort)
	} else {
		fmt.Println("HTTP Proxy:   Disabled")
	}

	if status.SOCKSEnabled {
		fmt.Printf("SOCKS5 Proxy: Enabled (%s:%d)\n", status.SOCKSHost, status.SOCKSPort)
	} else {
		fmt.Println("SOCKS5 Proxy: Disabled")
	}
}
