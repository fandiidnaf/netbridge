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
| Dynamic PAC & Mobile Setup Portal  |
| Caffeinate Sleep Prevention        |
+------------------------------------+
                 |
        +--------+--------+
        |                 |
        v                 v
+---------------+ +------------------+
| Smartphones   | | Other Laptops    |
| (Mobile Portal| | (1-Line Command) |
| & QR Code)    | | (or USB-C Bridge)|
+---------------+ +------------------+
```

---

## Key Features

- **100% User Space**: Runs without root permissions for normal operations. SIP remains fully enabled.
- **Sleep Prevention**: Built-in macOS `caffeinate` integration keeps your Mac awake and Wi-Fi powered while the gateway is running.
- **Interactive Mobile Portal**: Scanning the terminal QR code opens an interactive web portal that automatically detects your client OS (iOS, Android, macOS, Linux, Windows), provides one-tap copy buttons, and verifies active proxy connectivity.
- **Zero False-Positive Verification**: Features a virtual probe domain (`probe.netbridge`) to verify that client device traffic is genuinely routing through the gateway.
- **Live Activity Logs**: Real-time terminal logging with client IP, target host, connection duration, transfer speed, and throughput metrics.
- **One-Line Laptop Connect**: Generates an auto-configuration shell script for client Macs and Linux laptops.
- **Dual Engine**: High-performance HTTP/HTTPS CONNECT tunneling and standard RFC 1928 SOCKS5 proxy support.
- **Automatic Cleanup**: Automatically unsets and restores client system proxies upon disconnection.

---

## Primary Use Cases

### 1. Hotel, Airport, or In-Flight Single-Device Paywalls (Captive Portals)
Many hotels, public venues, and flights restrict Wi-Fi access to a single authenticated device per voucher.
- **Solution**: Authenticate on your Mac. Run `netbridge serve`. Connect your smartphone or secondary laptop to the same venue Wi-Fi, then route through NetBridge. All your secondary devices gain internet access under the Mac's authenticated session.

### 2. Whole-Device VPN Sharing
If your Mac is connected to a corporate VPN, Tailscale, WireGuard, or custom proxy that cannot easily be installed on a mobile device or managed secondary laptop:
- **Solution**: Clients routing through NetBridge automatically inherit the Mac's VPN route without installing VPN profiles on each device.

### 3. Boosting Speed for a Slow Secondary Laptop (Laptop B)
*Question*: If Laptop B is on the same Wi-Fi network as the Mac, but Laptop B gets terrible speeds (e.g., poor Wi-Fi antenna, bad driver, interference) while the Mac gets high speeds, can NetBridge make Laptop B faster?
- **Analysis**: If Laptop B communicates with the Mac over its weak Wi-Fi, the weak radio link remains a bottleneck.
- **High-Speed Solution**: Connect Laptop B directly to the Mac via a **USB-C / Thunderbolt cable**. macOS automatically negotiates a direct connection (up to 40 Gbps, 0 ms latency, 0% packet loss). Run `netbridge serve` on the Mac, and Laptop B will route directly over the cable through the Mac's fast Wi-Fi connection, achieving full speed!

### 4. Local Development & Traffic Inspection
Quickly test web applications across mobile browsers without complex DNS reconfigurations or self-signed certificate installations.

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
Mobile Portal:    http://192.168.1.50:8888/connect
----------------------------------------------------------------
Client Quick-Connect:
  • Mac / Linux:  curl -fsSL http://192.168.1.50:8888/setup.sh | bash
  • Disconnect:   curl -fsSL http://192.168.1.50:8888/restore.sh | bash
  • Windows:      Enable 'Use setup script' and paste PAC URL above
  • Smartphone:   Scan QR code below or set Wi-Fi proxy manually:
                  Host: 192.168.1.50 | Port: 8888
----------------------------------------------------------------

[ ANSI QR CODE PRINTED HERE ]

Proxy gateway is active. Press Ctrl+C to terminate.
================================================================
```

### 2. Connect from Other Devices

#### Option A: Smartphone (iOS / Android)

1. Make sure your smartphone is connected to the same Wi-Fi network as your Mac.
2. Open your camera and scan the QR code displayed in the terminal.
3. Your mobile browser opens the **NetBridge Mobile Portal**:
   - The portal automatically detects whether you are on iOS or Android.
   - Use the **Copy** buttons to copy Host IP and Port.
   - Follow the 3-step setup guide for your OS.
   - Tap **Verify Active Proxy Connection** to confirm traffic is successfully routing through your Mac!

#### Option B: Another Mac or Linux Laptop

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

#### Option C: Windows Laptop

1. Open **Settings** > **Network & internet** > **Proxy**.
2. Under **Automatic proxy setup**, turn on **Use setup script**.
3. In **Script address**, paste the PAC URL: `http://<HOST_IP>:8888/proxy.pac`.
4. Click **Save**.

---

## Live Activity Logging

While `netbridge serve` is active, real-time connection events and throughput are logged to the terminal:

```text
22:05:10 [CONNECT] 192.168.1.105 -> api.github.com:443
22:05:12 [CLOSE]   192.168.1.105 <- api.github.com:443 (Up: 1.4 KiB, Down: 32.1 KiB, Duration: 2.1s)
22:05:15 [SOCKS5]  192.168.1.105 -> 1.1.1.1:53
22:05:16 [PROBE]   192.168.1.105 validated active proxy routing
```

Query live gateway throughput via the JSON status endpoint:

```bash
curl -s http://127.0.0.1:8888/status | jq
```

---

## Sleep Prevention

By default, `netbridge serve` activates macOS `caffeinate` to prevent the host Mac from sleeping while the gateway is active:
- Prevents system idle sleep (`-i`) and AC sleep (`-s`).
- Automatically terminates when `netbridge` exits on `Ctrl+C`.
- Can be disabled if desired using `--prevent-sleep=false`.

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
  --host            Host address to announce (defaults to auto-detected local IP)
  --http-port       Port for HTTP/HTTPS proxy and setup scripts (default: 8888)
  --socks-port      Port for SOCKS5 proxy (default: 1080, set 0 to disable)
  --prevent-sleep   Keep macOS awake while gateway is active (default: true)
  --quiet           Suppress banner and QR code output
```

---

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
