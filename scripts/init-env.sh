#!/usr/bin/env bash
# Creates .env from .env.example with a random TIAMAT_HUB_TOKEN (local dev only).
# Use the printed token in Tiamat's .env or process environment — same value on both.

set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

if [[ -f .env ]]; then
	echo "init-env: .env already exists — not overwriting."
	echo "To regenerate the hub token, set TIAMAT_HUB_TOKEN manually in both Sauron and Tiamat, or delete .env and run: make init-env"
	exit 0
fi

if [[ ! -f .env.example ]]; then
	echo "init-env: missing .env.example" >&2
	exit 1
fi

token="$(openssl rand -hex 32)"
cp .env.example .env
sed -i "s|^TIAMAT_HUB_TOKEN=.*|TIAMAT_HUB_TOKEN=${token}|" .env

echo "init-env: wrote .env with a new TIAMAT_HUB_TOKEN (not committed to git)."
echo ""
echo "Set the SAME value on Tiamat (e.g. in its .env after godotenv, or systemd Environment=):"
echo "  TIAMAT_HUB_TOKEN=${token}"
echo ""
