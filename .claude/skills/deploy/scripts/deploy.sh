#!/usr/bin/env bash
set -euo pipefail

cd /Users/yansir/code/52/ccc

echo "Building ccc..."
go build -o ccc .

echo "Deploying to ~/.local/bin/ccc..."
cat ./ccc > ~/.local/bin/ccc
chmod +x ~/.local/bin/ccc

echo "Done. $(~/.local/bin/ccc --version 2>&1 || true)"
