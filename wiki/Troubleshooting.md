# Troubleshooting & FAQ

Common troubleshooting scenarios and diagnostic steps for SwanPanel.

---

## 1. strongSwan VICI Socket Issues

### Symptom:
Admin dashboard reports `strongSwan Daemon: Disconnected` or `VICI socket not found`.

### Resolution:
For the recommended Docker Compose installation, run these commands from the repository directory:

```bash
docker compose ps
docker compose logs --tail=100 swan-panel
docker compose exec swan-panel ls -l /var/run/charon.vici
```

For the optional host strongSwan installation:

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
3. In **Server Settings**, set **Server Endpoint** to the reachable IP/DNS address and **Gateway Identity** to the identity covered by the server certificate. These values can differ (e.g. a LAN endpoint IP and a DNS-name certificate identity); the client must trust the CA and verify the configured identity.
4. For a client stuck at IKE_SA_INIT, run `swanctl --log` inside the strongSwan container and capture UDP 500/4500 on the macOS host with `sudo tcpdump -ni en0 'udp port 500 or udp port 4500'` (replace `en0` with the active interface). Repeated IKE_SA_INIT requests without IKE_AUTH indicate an incomplete exchange; verify that replies actually return to the client.
5. On **Docker Desktop for macOS**, UDP port publishing may not preserve the IKEv2 return path. If the host receives the phone's request but replies are addressed elsewhere, investigate Docker's UDP/NAT forwarding; use a bridged Linux VM or Linux host for reliable end-to-end IPsec tests.

---

## 3. Apple `.mobileconfig` Signature Cannot Be Verified

SwanPanel signs the profile using an attached SHA-256 CMS/PKCS#7 signature and a dedicated signing certificate. If macOS/iOS says the signer cannot be verified, the device does not yet trust the SwanPanel Root CA. Trust must be established **out-of-band** (for example, via a managed trust store); embedding a CA certificate inside a downloaded profile does not establish independent trust in that same profile.

For inspection, run `openssl cms -verify -inform DER -in profile.mobileconfig -CAfile ca.crt -out profile.plist` with a trusted local copy of `ca.crt`. A tampered profile must fail verification. Do not override a failed cryptographic signature.

---

## 4. DNS Queries Failing Over VPN

### Symptom:
Client connects successfully, but domain names ending in `.strongnet` do not resolve.

### Resolution:
1. Verify SwanPanel's DNS engine is listening on port 53:
   ```bash
   sudo ss -ulpn | grep ':53'
   ```
2. If port 53 is already occupied, identify the host process using `sudo ss -ulpn | grep ':53'`. Resolve the conflict according to your host DNS setup; do not change `systemd-resolved` blindly. For Docker Compose, inspect `docker compose logs --tail=100 swan-panel`.
