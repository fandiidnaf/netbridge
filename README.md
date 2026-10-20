# NetBridge

NetBridge is a lightweight, user-space network sharing gateway designed for macOS. It allows you to share your Mac's active Wi-Fi or VPN connection with other devices (smartphones, other laptops, and tablets) without requiring kernel extensions, without disabling System Integrity Protection (SIP), and without modifying internal system settings.

---

## Why NetBridge?

On macOS, the built-in *Internet Sharing* feature prevents you from sharing an incoming Wi-Fi connection over Wi-Fi. While Windows and Linux support virtual STA/AP concurrency on select chips, Apple's proprietary Wi-Fi driver architecture (`DriverKit-AppleBCMWLAN`) strictly locks the internal wireless interface from running as an infrastructure Access Point while simultaneously connected to an upstream network.

Bypassing this via low-level kernel hacks requires disabling SIP and downgrading Apple Silicon security policies, which introduces severe security and stability risks.

**NetBridge solves this problem legally in User Space:**
Instead of attempting to broadcast a secondary Wi-Fi radio signal from a locked driver, NetBridge turns your Mac into a local gateway. Any client device on the same local network or paired cable can route all its traffic through your Mac's Wi-Fi and VPN connection.

```
+------------------------------------+
| Upstream Internet (Wi-Fi / VPN)    |
+------------------------------------+
                 |
                 v
+------------------------------------+
| Host Mac (running NetBridge)       |
| HTTP/HTTPS Tunnel : Port 8888      |
| SOCKS5 Proxy      : Port 1080      |
| Dynamic PAC & Auto-Setup Endpoint  |
+------------------------------------+
                 |
        +--------+--------+
        |                 |
        v                 v
+---------------+ +------------------+
| Smartphones   | | Other Laptops    |
| (Scan QR Code)| | (1-Line Command) |
+---------------+ +------------------+
```

---

## Key Features

- **100% User Space**: Runs without root permissions for normal operations. SIP remains fully enabled.
- **Terminal QR Code**: Prints a compact ANSI QR code directly in your terminal for instant smartphone camera scanning.
- **One-Line Laptop Connect**: Generates an auto-configuration shell script for client Macs and Linux laptops.
- **Proxy Auto-Configuration (PAC)**: Provides a dynamic PAC script endpoint for Windows and macOS automatic proxy configuration.
- **Dual Engine**: High-performance HTTP/HTTPS CONNECT tunneling and standard RFC 1928 SOCKS5 proxy support.
- **Automatic Cleanup**: Automatically unsets and restores client system proxies upon disconnection.

---

## Installation

### Via Homebrew (Recommended)

```bash
brew tap fandiidnaf/tap
brew install netbridge
```

### Via Go Install

```bash
go install github.com/fandiidnaf/netbridge/cmd/netbridge@latest
```

### From Source

```bash
git clone https://github.com/fandiidnaf/netbridge.git
cd netbridge
go build -o netbridge ./cmd/netbridge
sudo mv netbridge /usr/local/bin/
```

---

## Usage

### 1. Start the Gateway on Host Mac

Start the sharing gateway on your primary Mac:

```bash
netbridge serve
```

NetBridge automatically detects your local IP address and displays a status dashboard:

```text
NetBridge: User-Space Network Sharing Gateway
================================================================
Host Address:     192.168.1.50
HTTP/HTTPS Proxy: http://192.168.1.50:8888
SOCKS5 Proxy:     socks5://192.168.1.50:1080
PAC Auto-Config:  http://192.168.1.50:8888/proxy.pac
----------------------------------------------------------------
Client Quick-Connect:
  • Mac / Linux:  curl -fsSL http://192.168.1.50:8888/setup.sh | bash
  • Disconnect:   curl -fsSL http://192.168.1.50:8888/restore.sh | bash
  • Windows:      Enable 'Use setup script' and paste PAC URL above
  • Smartphone:   Connect to the same Wi-Fi, then scan QR code below
----------------------------------------------------------------

[ ANSI QR CODE PRINTED HERE ]

Proxy gateway is active. Press Ctrl+C to terminate.
================================================================
```

### 2. Connect from Other Devices

#### Option A: Another Mac or Linux Laptop

Run the one-line setup command in Terminal:

```bash
curl -fsSL http://<HOST_IP>:8888/setup.sh | bash
```

To disconnect and restore normal network settings:

```bash
curl -fsSL http://<HOST_IP>:8888/restore.sh | bash
```

Alternatively, if `netbridge` is installed on the client Mac:

```bash
netbridge join <HOST_IP>
```

#### Option B: Windows Laptop

1. Open **Settings** > **Network & internet** > **Proxy**.
2. Under **Automatic proxy setup**, turn on **Use setup script**.
3. In **Script address**, paste the PAC URL: `http://<HOST_IP>:8888/proxy.pac`.
4. Click **Save**.

#### Option C: Smartphone (iOS / Android)

1. Make sure your smartphone is connected to the same Wi-Fi network (or personal hotspot).
2. Open your camera and scan the QR code displayed in the terminal.
3. Alternatively, open your Wi-Fi settings, select **Configure Proxy > Manual**, and set:
   - **Server**: `<HOST_IP>`
   - **Port**: `8888`

---

## Handling Hotel & Captive Portal Networks

In hotel or public Wi-Fi networks that enable **Client Isolation** (preventing devices from communicating directly over Wi-Fi), you can connect two Macs directly using a USB-C or Thunderbolt cable:

1. Connect the Host Mac and Client Mac with a USB-C cable.
2. macOS automatically negotiates a high-speed **Thunderbolt Bridge** (up to 40 Gbps).
3. On the Host Mac, start `netbridge serve`.
4. The client can now route all traffic through the cable connection directly.

---

## CLI Reference

```text
Usage:
  netbridge <command> [arguments]

Commands:
  serve      Start the network sharing gateway (HTTP/HTTPS + SOCKS5)
  join       Configure local machine to use an existing NetBridge gateway
  leave      Disable all local proxy settings on this machine
  status     Inspect the current system proxy status
  version    Show version and build information

Flags for 'serve':
  --host         Host address to announce (defaults to auto-detected local IP)
  --http-port    Port for HTTP/HTTPS proxy and setup scripts (default: 8888)
  --socks-port   Port for SOCKS5 proxy (default: 1080, set 0 to disable)
  --quiet        Suppress banner and QR code output
```

---

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
