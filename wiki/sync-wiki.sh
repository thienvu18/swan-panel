#!/usr/bin/env bash
# Script to synchronize markdown documents in this directory to the GitHub Wiki
set -euo pipefail

REPO_WIKI_URL="git@github.com:thienvu18/swan-panel.wiki.git"
TEMP_DIR=$(mktemp -d)

echo "==> Cloning SwanPanel GitHub Wiki..."
if git clone "$REPO_WIKI_URL" "$TEMP_DIR"; then
    echo "==> Copying wiki files..."
    cp wiki/*.md "$TEMP_DIR/"
    cd "$TEMP_DIR"
    git add .
    if git diff-index --quiet HEAD --; then
        echo "==> No changes to commit in wiki."
    else
        git commit -m "docs(wiki): update wiki documentation pages"
        git push origin master || git push origin main
        echo "==> GitHub Wiki updated successfully!"
    fi
    rm -rf "$TEMP_DIR"
else
    echo "Notice: If the wiki repository is not yet initialized on GitHub,"
    echo "please navigate to https://github.com/thienvu18/swan-panel/wiki"
    echo "and click 'Create the first page' once in the web browser."
    echo "Then re-run this script to synchronize all pages."
    rm -rf "$TEMP_DIR"
    exit 1
fi
