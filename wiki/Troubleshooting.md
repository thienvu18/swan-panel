# Troubleshooting & FAQ

Common troubleshooting scenarios and diagnostic steps for SwanPanel.

---

## 1. strongSwan VICI Socket Issues

### Symptom:
Admin dashboard reports `strongSwan Daemon: Disconnected` or `VICI socket not found`.

### Resolution:
1. Ensure the strongSwan daemon is active:
   ```bash
   sudo systemctl status strongswan
   # Or on Ubuntu/Debian:
   sudo systemctl status strongswan-starter
   ```
2. Verify that the VICI socket file exists at `/var/run/charon.vici`:
   ```bash
   ls -la /var/run/charon.vici
   ```
3. Ensure the `charon.vici` plugin is enabled in `/etc/strongswan.d/charon/vici.conf`:
   ```text
   vici {
       load = yes
       socket = unix:///var/run/charon.vici
   }
   ```
4. Restart strongSwan:
   ```bash
   sudo systemctl restart strongswan
   ```

---

## 2. Client Cannot Connect (Timeout on Port 500/4500)

### Symptom:
Client shows "Connecting..." and fails with "The VPN server did not respond".

### Resolution:
1. Ensure cloud provider security groups (AWS, GCP, DigitalOcean, Hetzner) allow incoming UDP traffic on ports **500** and **4500**.
2. Verify UDP ports are listening:
   ```bash
   sudo ss -ulnp | grep -E '500|4500'
   ```
3. Verify that your server's public IP or domain in **Server Settings** exactly matches the server identifier in the client configuration.

---

## 3. Apple `.mobileconfig` Shows "Unsigned Profile"

### Symptom:
macOS / iOS displays a yellow "Not Signed" badge when importing the `.mobileconfig`.

### Resolution:
- This is standard behavior for internally generated VPN profiles using an internal Root CA.
- Click **Continue** / **Install** and approve the profile in **System Settings -> Profiles**. The internal Root CA embedded in the profile will be added to the local keychain for tunnel validation.

---

## 4. DNS Queries Failing Over VPN

### Symptom:
Client connects successfully, but domain names ending in `.strongnet` do not resolve.

### Resolution:
1. Verify SwanPanel's DNS engine is listening on port 53:
   ```bash
   sudo ss -ulpn | grep ':53'
   ```
2. Check if `systemd-resolved` is occupying port 53:
   ```bash
   # Temporarily disable stub listener if needed:
   sudo sed -i 's/#DNSStubListener=yes/DNSStubListener=no/' /etc/systemd/resolved.conf
   sudo systemctl restart systemd-resolved
   ```
