#!/bin/sh
set -eu
cd /workspace/swan-panel/ui

if [ ! -x node_modules/.bin/vite ]; then
    echo "Installing frontend dependencies into the devcontainer node_modules volume..."
    npm ci --no-audit --no-fund
fi

exec npm run dev -- --host 0.0.0.0 --port 3000 --strictPort
