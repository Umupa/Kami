#!/usr/bin/env bash
# Runs `mygo dev` signed with the stable "Kami Dev" identity, so the
# development app keeps its grants across hot reloads. It is a separate app
# (Kami Dev, dev.umupa.kami.dev) with its own grants and data directory.
# Other arguments go to `mygo dev`.
set -euo pipefail

cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
name="Kami Dev"

if ! security find-identity -v -p codesigning | grep -q "\"$name\""; then
	echo "还没有签名身份「${name}」，先创建（每台 Mac 一次）。" >&2
	scripts/dev-cert.sh
fi
exec go tool mygo dev -sign "$name" "$@"
