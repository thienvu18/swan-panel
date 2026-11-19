# Quickstart & Installation

SwanPanel and strongSwan run together in the recommended Docker Compose deployment. Your users connect with the VPN built into their supported devices.

---

## 1. Docker Compose (Recommended)

On a Linux server with Docker and the Compose plugin:

```bash
git clone https://github.com/thienvu18/swan-panel.git
cd swan-panel
umask 077
printf 'JWT_SECRET=%s\n' "$(openssl rand -hex 32)" > .env
docker compose up -d
docker compose ps
```

The container includes strongSwan and persistent data; no host VICI socket mount is required. Open UDP 500/4500 for IKEv2 and TCP 8080 for the web UI, or put the UI behind HTTPS.

For logs:

```bash
docker compose logs -f
```

**Advanced host installation:** If you manage strongSwan with systemd on the server, see [deploy/README.md](https://github.com/thienvu18/swan-panel/blob/main/deploy/README.md) for the native installer and host-socket deployment.

---

## 2. Accessing the Web Control Plane

When the container is running, navigate to:

```text
http://<YOUR_SERVER_IP>:8080
```

SwanPanel has no default login. In the repository directory, run `docker compose exec swan-panel cat /var/lib/swan-panel/swanpanel.db.setup-token`. Enter the one-time token in the web setup form, create your first administrator, and set the public server address in **Server Settings**.

---

## 3. Service Management

From the repository directory, check, inspect and restart the Docker service:

```bash
docker compose ps
docker compose logs -f
docker compose restart swan-panel
```

For the optional host installation, use `sudo systemctl status swan-panel` and `sudo journalctl -u swan-panel -f`.
