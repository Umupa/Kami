#!/usr/bin/env bash
# Runs `mygo dev` signed with the stable "Kami Dev" identity, so the
# development app keeps its grants across hot reloads. It is a separate app
# (Kami Dev, dev.umupa.kami.dev) with its own grants and data directory.
# Other arguments go to `mygo dev`.
set -euo pipefail

cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
scripts/dev-cert.sh
exec go tool mygo dev -sign "Kami Dev" "$@"
