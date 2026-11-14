# SwanPanel VS Code Dev Container

The workspace uses **one Linux development container with NET_ADMIN/NET_RAW capabilities**, not the production Docker Compose stack. It includes Go/CGO, Node 26, strongSwan, nftables, Air and Vite.

## Start with VS Code on macOS

1. Start **Docker Desktop**; install VS Code's **Dev Containers** extension.
2. Open the SwanPanel repository in VS Code.
3. Run **Dev Containers: Reopen in Container**. VS Code builds `.devcontainer/Dockerfile` and starts `.devcontainer/compose.yml`.
4. In the integrated **container** terminal run:
   ```sh
   supervisorctl -c .devcontainer/supervisord.conf status
   sh .devcontainer/diagnostics.sh
   cat /var/lib/swan-panel/swanpanel.db.setup-token
   ```
5. Enter the one-time setup token in the login/setup screen and create your administrator. No default admin credentials are installed.

Auto-started services:

| Component | Inside container | Published on MacBook |
|---|---|---|
| Vite + React HMR, including the same-origin `/api` proxy | TCP 3000 | TCP 3000 |
| Air-run Go API | TCP 8080 | TCP 8080 |
| strongSwan IKE | UDP 500 | UDP 500 |
| strongSwan NAT-T | UDP 4500 | UDP 4500 |
| Embedded DNS | TCP/UDP 53 | TCP/UDP 1053 by default |

All published ports use `0.0.0.0` to allow devices on the same Wi-Fi/LAN. **Use an isolated, trusted LAN only**. API/UI are not HTTPS by default.

On the MacBook, find its active Wi-Fi address:

```sh
ipconfig getifaddr en0
```

If Wi-Fi is not `en0`, see `networksetup -listallhardwareports`. On a phone/tablet connected to the same network:

- UI / HMR / API proxy: `http://<MAC_LAN_IP>:3000`
- Direct backend health: `http://<MAC_LAN_IP>:8080/api/health`
- IKEv2 remote gateway **candidate**: `<MAC_LAN_IP>`, UDP 500/4500 (see limitations below)

The React app uses relative `/api` URLs; the Vite proxy preserves the original HTTP Host so requests work from phones and other physical machines. **Always use port 3000 for development UI**. Port 8080 is the backend, whose embedded UI may be an old build/placeholder.

## Live reload and logs

```sh
# Inside the VS Code container terminal:
supervisorctl -c .devcontainer/supervisord.conf status
supervisorctl -c .devcontainer/supervisord.conf restart backend
supervisorctl -c .devcontainer/supervisord.conf restart frontend
supervisorctl -c .devcontainer/supervisord.conf restart strongswan
sh .devcontainer/diagnostics.sh

# Go and frontend tests (Linux with CGO):
go test -count=1 ./...
go test -race -count=1 ./...
cd ui && npm test && npm run build

# On the macOS host:
docker compose -f .devcontainer/compose.yml logs -f --tail=150 workspace
```

Air watches Go files (using polling for Docker Desktop bind mounts), recompiles with `CGO_ENABLED=1` and restarts only the Go API. Vite runs independently with React Fast Refresh/HMR. Both can restart without recreating the strongSwan daemon. The frontend dependency tree is stored in a Docker named volume (not mixed with macOS native dependencies). SQLite, one-time setup token and PKI keys are persisted in `swanpanel_dev_data` across rebuilds.

Go requires `ui/dist/index.html` for `//go:embed`; `.devcontainer/start.sh` creates a **local placeholder** if the directory does not yet exist. The backend now explicitly detects and rejects that placeholder as a frontend bundle. This is expected in development: Air still starts the API and Vite serves the live React UI on port 3000. A production server refuses to start with the placeholder or missing built JavaScript. Never commit it or expose port 8080 as the development UI.

## Real IKEv2 client testing: Docker Desktop limitation

This dev container **runs real strongSwan and VICI**. It does **not** mock the daemon, but Docker Desktop on macOS runs a Linux VM behind NAT. Publishing UDP 500 and 4500 can expose the IKE listener; full IKE/IPsec/XFRM negotiation, routing of decrypted traffic and the Linux kernel SA are **not guaranteed to work**. Docker Desktop host networking is L4 TCP/UDP only, not raw ESP. Do not equate a reachable API or VICI socket with a working VPN.

For a physical device test, open SwanPanel Server Settings and configure the **Server Endpoint** to the MacBook LAN IP (or a LAN-resolvable hostname). Set **Gateway Identity** separately to the name/IP in the server certificate SAN. The identity does not need a DNS record when clients connect using the endpoint IP. Download a new device profile after changing the endpoint; restart the Go backend after changing certificate identity so server credentials can be reconciled.

For failed phone handshakes, run `swanctl --log` inside the container while capturing UDP on the macOS host with `sudo tcpdump -ni en0 'udp port 500 or udp port 4500'` (adjust the interface). Repeated `IKE_SA_INIT` requests with no `IKE_AUTH` mean the exchange did not advance; compare the phone's source address with the destination of the server's reply. A reply sent to an unexpected IP/port points to a possible Docker Desktop UDP/NAT return-path issue, not necessarily a certificate problem.

If the native client cannot establish a tunnel even though the IKE packets reach the container, run the same stack on a dedicated **Linux host or bridged Linux VM** with NET_ADMIN, XFRM and IP forwarding. This is the dependable environment for end-to-end IPsec and firewall tests. A NAT-mode VM behind Docker Desktop is not an equivalent substitute.

**Certificate safety:** the generated VPN profile may embed PKCS#12 private-key material. Do not download/install it over plain HTTP on an untrusted LAN. For mobile provisioning, terminate TLS using a trusted HTTPS reverse proxy with a hostname your device can validate. Browser testing over local HTTP does not validate profile trust. The OIDC implementation additionally requires HTTPS for non-loopback callback URLs, so LAN-IP HTTP cannot be used to validate external SSO.

The default external DNS mapping is **1053 → 53** to avoid clashing with host services. Real VPN clients generally require destination port 53; for a trial, set `SWAN_DEV_DNS_PORT=53` when launching Compose if macOS permits it.

## From a host terminal, without VS Code

```sh
docker compose -f .devcontainer/compose.yml config
docker compose -f .devcontainer/compose.yml up --build -d
docker compose -f .devcontainer/compose.yml logs -f workspace
docker compose -f .devcontainer/compose.yml exec workspace sh .devcontainer/diagnostics.sh
docker compose -f .devcontainer/compose.yml down
```

`down` retains named SQLite/PKI/Go/npm volumes. **Never use `down -v` unless you explicitly want to destroy the dev CA, private keys and database.**

## Troubleshooting

- **UI unavailable from phone:** confirm the Mac and device are on the same LAN, Mac firewall allows Docker port 3000, Wi-Fi AP client isolation is disabled, and `curl http://<MAC_LAN_IP>:3000/` succeeds from Mac.
- **Vite/HMR fails:** use port 3000 from the phone; Docker must publish port 3000 unchanged for WebSocket HMR.
- **Air compilation fails:** run `go test ./...` in the container, check `go.mod`/`go.sum`, `ui/dist/index.html`, and log output; native Go/CGO compilation runs entirely inside Linux.
- **VICI socket missing:** inspect `supervisorctl ... status`, `ipsec statusall`, and `swanctl --stats`. Docker Desktop kernel/network restrictions may prevent charon from operating fully.
- **Lost setup token:** inspect the persisted volume at `/var/lib/swan-panel`; don't delete the token file while the initial administrator has not been created.
- **OIDC on phone:** use valid HTTPS and a resolvable hostname instead of the Mac's plain-HTTP LAN address.
