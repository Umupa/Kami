#!/usr/bin/env bash
# Builds build/darwin-arm64/cida-next.app, signed with "cida-next Dev" when
# scripts/dev-cert.sh has created it, so that permission grants survive the
# rebuild. Extra arguments go to `mygo build`.
set -euo pipefail

cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
name="cida-next Dev"

if security find-identity -v -p codesigning | grep -q "\"$name\""; then
	sign=(-sign "$name")
else
	echo "没有签名身份「$name」，使用临时签名：每次构建后都要重新授权。先运行 scripts/dev-cert.sh。" >&2
	sign=()
fi
exec go tool mygo build -skip-dmg "${sign[@]}" "$@"
