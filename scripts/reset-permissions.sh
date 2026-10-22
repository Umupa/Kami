#!/usr/bin/env bash
# Clears Kami's Accessibility, Input Monitoring and Screen Recording records,
# for when System Settings shows a grant that belongs to an earlier
# signature. Pass --dev for the app `scripts/dev.sh` runs. Kami asks again
# the next time it needs each permission.
set -euo pipefail

cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
id=$(sed -n 's/.*"identifier": *"\([^"]*\)".*/\1/p' mygo.json)
if [[ "${1:-}" == "--dev" ]]; then
	id="$id.dev"
fi

for service in Accessibility ListenEvent ScreenCapture; do
	tccutil reset "$service" "$id"
done
echo "已清除「${id}」的系统权限记录。退出并重新打开 Kami 后按提示重新授权。"
