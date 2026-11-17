#!/usr/bin/env bash
# Backwards-compatible entrypoint: the old script used outdated API payloads,
# assumed a non-existent client container, and could report false PASS results.
# The replacement uses disposable isolated Docker Compose resources only.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec "$ROOT/scripts/test_docker_integration.sh" "$@"
