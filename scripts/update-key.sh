#!/usr/bin/env bash
# Creates the key pair that signs updates, once: the secret goes into the
# login keychain as "Kami update key", and the public key, printed, goes into
# updates.publicKey of mygo.json. Installed copies of Kami accept only
# updates signed with this key, so losing it strands them: back it up.
set -euo pipefail

cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
service="Kami update key"

if security find-generic-password -s "$service" >/dev/null 2>&1; then
	echo "钥匙串里已经有「${service}」，不再重复创建：换掉它，已安装的 Kami 就收不到更新了。" >&2
	exit 1
fi
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
go tool mygo keygen -o "$dir" >/dev/null
security add-generic-password -a "$USER" -s "$service" -w "$(cat "$dir/mygo-update.key")"
echo "私钥已存入登录钥匙串「${service}」。请另外备份（例如密码管理器）：丢失后，已安装的 Kami 再也收不到更新。" >&2
echo "公钥（写进 mygo.json 的 updates.publicKey）：" >&2
cat "$dir/mygo-update.pub"
