# Security Policy

## Supported Versions

Security updates and patches are provided for the latest releases of NetBridge.

| Version | Supported          |
| :------ | :----------------- |
| `1.x`   | :white_check_mark: |
| `< 1.0` | :x:                |

---

## Security Model & Design Principles

NetBridge operates as a local network gateway. When deploying and running NetBridge, keep the following security considerations in mind:

### 1. End-to-End Encryption (Zero MITM)
- For HTTPS and TLS connections, NetBridge acts strictly as a transparent Layer-4 TCP tunnel (`HTTP CONNECT` method and SOCKS5).
- **NetBridge does not terminate TLS, does not decrypt payloads, and does not install root certificates on client devices.**
- All HTTPS traffic remains end-to-end encrypted between the client device and the upstream destination server.

### 2. Network Exposure & Scope
- By default, NetBridge binds to your Mac's local interface IP address (e.g., `192.168.x.x` or link-local `169.254.x.x`).
- It is designed for use on trusted local networks: home networks, personal hotspots, hotel networks, or direct peer-to-peer cable bridges.
- **Do not expose NetBridge gateway ports (8888 or 1080) directly to the public internet** without appropriate firewall rules or authentication.

### 3. User Space & System Integrity Protection (SIP)
- NetBridge runs entirely in user space. It does not require kernel extensions (`kext`), system-level driver modifications, or disabling macOS System Integrity Protection (SIP).
- Normal gateway operation does not require `root` or `sudo` privileges.

---

## Reporting a Vulnerability

If you discover a security vulnerability or security-related issue in NetBridge, please **do not open a public GitHub issue**.

Instead, report it responsibly via one of the following methods:

1. **GitHub Security Advisories (Preferred)**:
   - Go to the **Security** tab of this repository.
   - Click **Report a vulnerability** to submit a private report.
2. **Direct Contact**:
   - Email: **fandigit7@gmail.com** with the subject line `[NetBridge Security] Vulnerability Report`.

### What to Include in Your Report
- A clear description of the issue and its potential impact.
- Step-by-step reproduction instructions or a minimal Proof of Concept (PoC).
- Your environment details (macOS version, Go version, NetBridge version).

### Response Timeline
- **Initial Response**: Within 48 hours to acknowledge receipt of your report.
- **Status Update**: Within 7 business days with an assessment and remediation plan.
- **Disclosure**: We will coordinate with you to release a patch before any public announcement.
