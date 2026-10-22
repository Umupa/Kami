#!/usr/bin/env bash
# Builds build/darwin-arm64/Kami.app with the stable "Kami Dev" identity,
# creating it first when this Mac has none. Explicit --ad-hoc builds are for
# temporary artifacts; their grants do not survive rebuilds. Other arguments
# go to `mygo build`.
set -euo pipefail

cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
name="Kami Dev"
app="build/darwin-arm64/Kami.app"

if [[ "${1:-}" == "--ad-hoc" ]]; then
	shift
	echo "临时签名构建：重新构建后系统权限会失效。" >&2
	exec go tool mygo build -skip-dmg "$@"
fi

if ! security find-identity -v -p codesigning | grep -q "\"$name\""; then
	echo "还没有签名身份「${name}」，先创建（每台 Mac 一次）。" >&2
	scripts/dev-cert.sh
fi
go tool mygo build -skip-dmg -sign "$name" "$@"

# A designated requirement that names only the cdhash changes with every
# build, and so would every grant.
if codesign -dr - "$app" 2>&1 | grep -q 'designated => cdhash'; then
	echo "「${app}」没有用「${name}」签名，重新构建后系统权限会失效。" >&2
	exit 1
fi
