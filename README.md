# NetBridge

NetBridge is a lightweight, user-space network sharing gateway designed for macOS. It allows you to share your Mac's active Wi-Fi, Ethernet, or VPN connection with other devices (smartphones, other laptops, and tablets) without requiring kernel extensions, without disabling System Integrity Protection (SIP), and without modifying internal system settings.

---

## Why NetBridge?

On macOS, the native *Internet Sharing* feature prevents you from sharing an incoming Wi-Fi connection over Wi-Fi. While Windows and Linux support virtual STA/AP concurrency on select chips, Apple's proprietary Wi-Fi driver architecture (`DriverKit-AppleBCMWLAN`) strictly locks the internal wireless interface from running as an infrastructure Access Point while simultaneously connected to an upstream network.

Bypassing this via low-level kernel hacks requires disabling SIP and downgrading Apple Silicon security policies, which introduces severe security and stability risks.

**NetBridge solves this problem legally in User Space:**
Instead of attempting to broadcast a secondary Wi-Fi radio signal from a locked driver, NetBridge turns your Mac into a local gateway. Any client device on the same local network or connected via a direct cable can route all its traffic through your Mac's Wi-Fi and VPN connection.

---

## How It Works: Traffic Flow & Router Perspective

A common question is: *If client devices connect to the same Wi-Fi or router, doesn't the router still see them?*

Here is how NetBridge handles traffic across network layers:

```
[ Client Device (Laptop B / Smartphone) ]
                  |
                  | 1. Sends internal LAN tunnel request to Mac (Port 8888)
                  v
       +--------------------+
       |  Local Wi-Fi / LAN |  <-- Router acts ONLY as a Layer-2 local switch.
       +--------------------+      It does NOT see what websites the client opens.
                  |
                  v
       [ Host Mac (NetBridge) ]
                  |
                  | 2. Mac opens outbound connections on behalf of the client
                  v
       +--------------------+
       |  Router NAT / VPN  |  <-- Router sees ONLY the Mac creating WAN sessions.
       +--------------------+
                  |
                  v
        [ Upstream Internet ]  <-- Websites see the Public IP (Router NAT or Mac's VPN).
```

### 1. In the Eyes of the Local Router
- **Normal browsing (without NetBridge)**: The router inspects every packet, monitors domains via DNS, and enforces per-device captive portal logins or bandwidth limits against each client's private IP and MAC address.
- **With NetBridge**: The client sends its packets **only to the Mac's local IP address**. The router functions merely as a Layer-2 switch forwarding packets within the local subnet. When traffic exits to the internet, the router's NAT table only records connections initiated by the host Mac.

### 2. In the Eyes of Upstream Websites
Upstream web servers (e.g., Google, GitHub) communicate via **Public IP addresses**:
- **On standard Wi-Fi**: External servers see the **Router's Public WAN IP**, with all client requests multiplexed under the Mac's authenticated NAT connection.
- **When Mac is connected to a VPN**: If your Mac runs a VPN or secure tunnel (Tailscale, WireGuard, WARP, or corporate VPN), client traffic routes through that tunnel, and external websites see the **Mac's VPN Exit Public IP** instead of the local router's IP.

---

## Connection Modes

NetBridge supports two primary connection architectures:

### Mode 1: Wireless LAN Sharing (Default)
Both your Mac and client devices join the same local Wi-Fi network.
- **Best for**: Smartphones, tablets, and mobile devices scanning the terminal QR code.
- **Requirement**: Client devices must be connected to the same Wi-Fi network (or router) to communicate with the Mac's local IP.

### Mode 2: Direct Cable Bridge (Zero-Router / 100% Bypass)
Connect your Mac directly to a secondary laptop using a **USB-C, Thunderbolt, or Ethernet cable**.
- **Best for**: 
  - Overcoming weak, faulty, or congested Wi-Fi on a secondary laptop (Laptop B).
  - Venues with strict Wi-Fi *Client Isolation* (where devices cannot communicate with each other over Wi-Fi).
  - Maximizing throughput with link speeds up to 40 Gbps, 0 ms added latency, and 0% packet loss.
- **How to use**:
  1. Plug a USB-C / Thunderbolt cable between your Mac and Laptop B.
  2. Turn off Wi-Fi on Laptop B (optional, ensures all traffic uses the cable).
  3. Run `netbridge serve` on your Mac.
  4. On Laptop B, run the setup command using the Mac's Thunderbolt bridge IP (e.g., `169.254.x.x`):
     ```bash
     curl -fsSL http://<MAC_BRIDGE_IP>:8888/setup.sh | bash
     ```
  5. Laptop B is now 100% disconnected from the router, routing entirely through the Mac's fast Wi-Fi connection.

---

## Primary Use Cases

### 1. Hotel, Airport, or In-Flight Single-Device Paywalls (Captive Portals)
Many hotels, public venues, and flights restrict Wi-Fi access to a single authenticated device per voucher.
- **Solution**: Authenticate on your Mac. Run `netbridge serve`. Connect your smartphone or secondary laptop to the same venue Wi-Fi, then route through NetBridge. All your secondary devices gain internet access under the Mac's authenticated session.

### 2. Whole-Device VPN Sharing
If your Mac is connected to a corporate VPN, Tailscale, WireGuard, or custom proxy that cannot easily be installed on a mobile device or managed secondary laptop:
- **Solution**: Clients routing through NetBridge automatically inherit the Mac's VPN route without installing VPN profiles on each device.

### 3. Boosting Speed for a Slow Secondary Laptop (Laptop B)
*Problem*: Laptop B has a weak Wi-Fi chip, bad drivers, or poor antenna placement, resulting in slow speeds and high packet loss, while your Mac gets full speed on the same Wi-Fi.
- **Over Wi-Fi**: Routing over the same Wi-Fi will not fix the issue because packets still have to traverse Laptop B's weak radio link to reach the Mac.
- **The Cable Solution**: Connect Laptop B directly to the Mac via a **USB-C or Thunderbolt cable**. Laptop B routes across the wire directly into the Mac, completely bypassing its weak Wi-Fi card and matching your Mac's internet speed.

### 4. Local Development & Traffic Inspection
Quickly test web applications across mobile browsers without complex DNS reconfigurations or self-signed certificate installations.

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

## Installation

### Via Homebrew (Recommended)

Install in a single command (Homebrew will automatically tap the repository):

```bash
brew install fandiidnaf/tap/netbridge
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

## Updating NetBridge

### Built-in Smart Updater (Recommended)

NetBridge includes a built-in update command that checks GitHub for newer releases and provides instructions based on your installation method:

```bash
netbridge update
```

### Via Homebrew

If installed through the Homebrew tap:

```bash
brew upgrade fandiidnaf/tap/netbridge
```

### Via Go

```bash
go install github.com/fandiidnaf/netbridge/cmd/netbridge@latest
```

---

## Uninstallation & Clean Removal

If you decide to uninstall NetBridge, follow these steps to ensure your system network configurations are cleanly restored:

### Step 1: Ensure Proxy Settings Are Disabled

Before removing the binary, verify that your Mac's network interfaces are restored to default (no lingering proxy configurations):

```bash
netbridge leave
```

You can confirm that HTTP and SOCKS5 proxies are disabled with:

```bash
netbridge status
```

### Step 2: Remove the Application

#### If installed via Homebrew:

```bash
# 1. Uninstall the formula
brew uninstall fandiidnaf/tap/netbridge

# 2. (Optional) Remove the tap repository
brew untap fandiidnaf/tap
```

#### If installed manually or via Go:

```bash
# Locate and delete the binary
rm -f $(which netbridge)

# (Optional) Clear the local update cache
rm -rf ~/Library/Caches/netbridge
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

## Understanding Terminal Logs & Activity

While `netbridge serve` is running, real-time connection events appear in your terminal. Here is how to interpret them:

```text
22:30:10 [CONNECT] 192.168.1.105 -> api.github.com:443
22:30:12 [CLOSE]   192.168.1.105 <- api.github.com:443 (Up: 1.4 KiB, Down: 32.1 KiB, Duration: 2.1s)
22:30:15 [FAILED]  192.168.1.105 -> analytics.tracker.com:443: dial tcp: i/o timeout
22:30:16 [PROBE]   192.168.1.105 validated active proxy routing
```

### Log Symbols & Tags Reference

| Tag / Symbol | Meaning | Explanation |
| :--- | :--- | :--- |
| **`->`** | **Outbound Request** | The client device is requesting a new connection **toward** the destination server. |
| **`<-`** | **Inbound Closure** | The connection session has ended and returned data **back to** the client device. |
| **`[CONNECT]`** | **HTTPS Tunnel Open** | The client initiated a secure HTTPS tunnel via HTTP CONNECT. |
| **`[CLOSE]`** | **Tunnel Closed** | The connection completed normally. Displays upload bytes (`Up`), download bytes (`Down`), and total duration. |
| **`[FAILED]`** | **Connection Failed** | Mac could not reach the target host (e.g., target server down, DNS resolution failure, or timeout). |
| **`[HTTP]`** | **Standard HTTP** | Unencrypted HTTP forwarding request, including response HTTP status code and payload size. |
| **`[SOCKS5]`** | **SOCKS5 Session** | Client connected via the SOCKS5 proxy port (default: 1080). |
| **`[PROBE]`** | **Route Verified** | Client successfully verified active proxy routing via the virtual probe domain (`probe.netbridge`). |

> **Note on high connection frequency**: Modern websites and mobile operating systems routinely open 20–50 parallel TCP connections for fonts, scripts, API calls, and telemetry. Seeing frequent `[CONNECT]` and `[CLOSE]` events within seconds is completely normal.

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
  update     Check for updates and show upgrade instructions
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
