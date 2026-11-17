#!/bin/sh
# Read-only Compose regression: never starts/stops containers or host services.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

command -v docker >/dev/null 2>&1 || { echo "BLOCKED: Docker CLI missing" >&2; exit 2; }
command -v node >/dev/null 2>&1 || { echo "BLOCKED: Node.js missing" >&2; exit 2; }
docker compose version >/dev/null 2>&1 || { echo "BLOCKED: Docker Compose missing" >&2; exit 2; }

docker compose -f docker-compose.yml config --format json |
  node -e '
    const fs = require("fs");
    const cfg = JSON.parse(fs.readFileSync(0, "utf8"));
    const s = cfg.services["swan-panel"];
    if (Object.keys(cfg.services).length !== 1 || !s) throw Error("default not single service");
    if (s.network_mode === "host") throw Error("default changed to host networking");
    const ports = s.ports || [];
    for (const p of ["500/udp", "4500/udp"]) {
      const [port, protocol] = p.split("/");
      if (!ports.some(x => String(x.target) === port && x.protocol === protocol)) {
        throw Error("default IKE port missing: " + p);
      }
    }
  '

docker compose -f docker-compose.yml -f deploy/docker/docker-compose.host-swan.yml config --format json |
  node -e '
    const fs = require("fs");
    const cfg = JSON.parse(fs.readFileSync(0, "utf8"));
    const s = cfg.services["swan-panel"];
    if (Object.keys(cfg.services).length !== 1 || !s) throw Error("host override adds a service");
    if (s.network_mode !== "host") throw Error("host override does not share kernel namespace");
    if ((s.ports || []).length) throw Error("host override retained published ports");
    if (s.environment.VICI_SOCKET !== "/host-run/charon.vici") throw Error("wrong external VICI socket");
    if (s.environment.SWANPANEL_EXTERNAL_STRONGSWAN !== "1") throw Error("external-daemon flag missing");
    const mounts = s.volumes || [];
    if (!mounts.some(v => v.source === "/etc/swanctl" && v.target === "/etc/swanctl")) {
      throw Error("host swanctl config is not persisted");
    }
    if (!mounts.some(v => v.source === "/run" && v.target === "/host-run" && v.read_only)) {
      throw Error("host VICI mount is not read-only");
    }
  '

echo "PASS: default one-command Docker and opt-in host strongSwan Compose isolation"
