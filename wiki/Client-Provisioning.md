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
- **Provisioning Method:** Idempotent PowerShell script (`setup-<hostname>.ps1`, run as Administrator).
- **Workflow:**
  1. Imports the Root CA certificate into `Cert:\LocalMachine\Root` and the client `.p12` identity into `Cert:\LocalMachine\My`.
  2. Executes `Add-VpnConnection` (`IKEv2`, `MachineCertificate` authentication, split-tunneling) and configures IPsec proposal parameters (`Set-VpnConnectionIPsecConfiguration`).
  3. Adds split-tunnel routing table entries (`Add-VpnConnectionRoute`) for assigned resource group CIDRs.

---

## 3. Linux (`setup-<hostname>.sh` & `swanctl`)
- **Automated Shell Script (`setup-<hostname>.sh`):**
  - Installs strongSwan if needed, deploys Root CA and client certificate/key into `/etc/swanctl/`, writes `/etc/swanctl/conf.d/swanpanel.conf`, and loads connections via `swanctl --load-all`.
- **swanctl Config (`?format=conf`):**
  - Provides a standalone `swanctl-<hostname>.conf` connection block with client certificate and traffic selectors.

---

## 4. Android
- Android setup is performed entirely in **system Settings / KeyChain**, without
  third-party applications, ADB, helper APKs, or a downloaded ZIP.
- In the SwanPanel Resources or Dashboard page, choose **Set up Android VPN**.
  Download the direct `.p12` file and reveal/copy its matching import password only when needed.
- Import the certificate via Android **Settings > Security > Install certificate >
  VPN & app user certificate**. Create a built-in **IKEv2/IPsec RSA** VPN;
  use the server address, server identity and device/IPsec identifier displayed
  in the same setup dialog.
- The .p12 contains the CA chain. If the OEM requests a separate trusted root,
  download and import `swanpanel-ca.cer` from the dialog.
- Delete the `.p12` from Downloads and clear copied passwords after OS import. Not all Android/OEM versions
  expose this VPN type; mark unsupported cases clearly and do not require an app.
- SwanPanel retains `send_cert = always` in VICI and persistent gateway config
  to support native Apple clients that omit `CERTREQ`.
