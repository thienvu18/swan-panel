# Native Client Provisioning

SwanPanel delivers native operating system configuration profiles without requiring users to install third-party VPN apps or background daemons.

---

## 1. Apple Ecosystem (macOS & iOS)
- **Profile Format:** XML Mobile Configuration (`.mobileconfig`).
- **Cryptographic Payload:** Embedded base64-encoded PKCS#12 payload containing the user's private key, X.509 client certificate, and SwanPanel Root CA certificate.
- **VPN Payload Type:** `com.apple.vpn.managed` configured with:
  - Authentication Method: `Certificate` (IKEv2 RSA)
  - Remote Address: Configured server hostname / public IP
  - Remote Identifier: strongSwan server identity
  - Local Identifier: Assigned device hostname
  - Dead Peer Detection: Active
  - IPv4 Addressing: Dynamic DHCP/IKEv2 Configuration Payload
- **Installation:**
  1. Download `.mobileconfig` from the SwanPanel portal.
  2. Double-click the file to open **System Settings -> Profiles**.
  3. Click **Install**, enter the PKCS#12 bundle password, and click **Trust**.
  4. Connect instantly from the macOS / iOS native network menu!

---

## 2. Windows 10 & 11
- **Provisioning Method:** Idempotent PowerShell script.
- **Workflow:**
  1. Exports the client `.p12` bundle to `cert:\CurrentUser\My` and installs the Root CA certificate into `cert:\CurrentUser\Root`.
  2. Executes `Add-VpnConnection`:
     ```powershell
     Add-VpnConnection -Name "SwanPanel-VPN" `
       -ServerAddress "<SERVER_ADDRESS>" `
       -TunnelType IKEv2 `
       -AuthenticationMethod MachineCertificate `
       -SplitTunneling
     ```
  3. Configures IPsec cryptographic proposal parameters (`Set-VpnConnectionIPsecConfiguration`) with modern AES-256-GCM / SHA-384 / ECP-384 cipher suites.
  4. Adds routing table entries (`Add-VpnConnectionRoute`) for assigned resource group CIDRs.

---

## 3. Linux (NetworkManager & swanctl)
- **NetworkManager (`.nmconnection`):**
  - Download the ready `.nmconnection` file to `/etc/NetworkManager/system-connections/`.
  - Set permissions `chmod 600`.
  - Reload connections: `nmcli connection reload && nmcli connection up SwanPanel`.
- **swanctl CLI:**
  - Provides a complete `swanctl.conf` connection block with client certificate and private key paths.

---

## 4. Android
- Supports native Android IKEv2 RSA certificate configurations.
- Download the `.p12` file to device storage, import into **Settings -> Security & Privacy -> More Security Settings -> Encryption & Credentials -> Install a Certificate -> VPN & App User Certificate**.
- Create native VPN connection with Type `IKEv2/IPSec RSA`.
