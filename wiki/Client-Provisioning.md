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
- Android setup is performed entirely in **system Settings / KeyChain**, without
  third-party applications, ADB, helper APKs, or a downloaded ZIP.
- In the SwanPanel Resources or Dashboard page, choose **Set up Android VPN**.
  Download the direct `.p12` file and copy its matching import password.
- Import the certificate via Android **Settings > Security > Install certificate >
  VPN & app user certificate**. Create a built-in **IKEv2/IPsec RSA** VPN;
  use the server address, server identity and device/IPsec identifier displayed
  in the same setup dialog.
- The .p12 contains the CA chain. If the OEM requests a separate trusted root,
  download and import `swanpanel-ca.cer` from the dialog.
- Delete the .p12 from Downloads after OS import. Not all Android/OEM versions
  expose this VPN type; mark unsupported cases clearly and do not require an app.
- SwanPanel retains `send_cert = always` in VICI and persistent gateway config
  to support native Apple clients that omit `CERTREQ`.
