#!/usr/bin/env bash
# Builds build/darwin-arm64/Gloss.app with a stable "Gloss Dev" identity.
# Explicit --ad-hoc builds are for temporary artifacts; their grants do not
# survive rebuilds. Other arguments go to `mygo build`.
set -euo pipefail

cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
name="Gloss Dev"

if [[ "${1:-}" == "--ad-hoc" ]]; then
	shift
	echo "临时签名构建：重新构建后系统权限会失效。" >&2
elif security find-identity -v -p codesigning | grep -q "\"$name\""; then
	set -- -sign "$name" "$@"
else
	echo "没有签名身份「${name}」。请先运行 scripts/dev-cert.sh，再重新构建。" >&2
	exit 1
fi
exec go tool mygo build -skip-dmg "$@"
