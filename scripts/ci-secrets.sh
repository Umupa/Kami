#!/usr/bin/env bash
# Puts what the release workflow signs with into the repository's Actions
# secrets, from this Mac's login keychain: the "Kami Dev" identity, as a
# .p12 with a password of its own, and the key that signs updates. Run it
# once, and again only when they change. macOS asks to allow exporting the
# keychain's private keys; only the one of "Kami Dev" leaves this Mac, and
# nothing is printed or left on disk.
set -euo pipefail

cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
name="Kami Dev"
keychain="$HOME/Library/Keychains/login.keychain-db"
ssl=/usr/bin/openssl

if ! security find-identity -v -p codesigning | grep -q "\"$name\""; then
	echo "这台 Mac 上没有签名身份「${name}」：每个版本都要用同一个身份签名，请在签过以往版本的 Mac 上运行。" >&2
	exit 1
fi
if ! key=$(security find-generic-password -s "Kami update key" -w 2>/dev/null); then
	echo "钥匙串里没有更新签名私钥「Kami update key」。" >&2
	exit 1
fi

tmp=$(mktemp -d)
chmod 700 "$tmp"
trap 'rm -rf "$tmp"' EXIT
pass=$("$ssl" rand -hex 24)

# security exports every identity of the keychain; keep the one named.
echo "导出「${name}」，macOS 会请你允许导出钥匙串里的私钥…" >&2
if ! security export -k "$keychain" -t identities -f pkcs12 -P "$pass" -o "$tmp/all.p12" >/dev/null 2>&1; then
	echo "没能从钥匙串导出身份：导出私钥的请求被拒绝了。" >&2
	exit 1
fi
"$ssl" pkcs12 -in "$tmp/all.p12" -passin "pass:$pass" -nodes -out "$tmp/all.pem" 2>/dev/null
awk -v dir="$tmp" '
	/-----BEGIN / { n++; file = sprintf("%s/block%03d.pem", dir, n) }
	file { print > file }
	/-----END / { close(file); file = "" }
' "$tmp/all.pem"

cert=""
for block in "$tmp"/block*.pem; do
	grep -q "BEGIN CERTIFICATE" "$block" || continue
	if "$ssl" x509 -in "$block" -noout -subject | grep -qE "CN ?= ?$name(\$|[,/])"; then
		if [[ -n "$cert" ]]; then
			echo "钥匙串里有不止一个「${name}」证书，请先删掉多余的。" >&2
			exit 1
		fi
		cert="$block"
	fi
done
if [[ -z "$cert" ]]; then
	echo "导出的身份里没有「${name}」。" >&2
	exit 1
fi
public=$("$ssl" x509 -in "$cert" -noout -pubkey)
private=""
for block in "$tmp"/block*.pem; do
	grep -q "PRIVATE KEY" "$block" || continue
	if [[ "$("$ssl" pkey -in "$block" -pubout 2>/dev/null)" == "$public" ]]; then
		private="$block"
		break
	fi
done
if [[ -z "$private" ]]; then
	echo "没有找到「${name}」的私钥。" >&2
	exit 1
fi
"$ssl" pkcs12 -export -inkey "$private" -in "$cert" -name "$name" -out "$tmp/identity.p12" -passout "pass:$pass"

base64 -i "$tmp/identity.p12" | gh secret set KAMI_SIGNING_IDENTITY
printf '%s' "$pass" | gh secret set KAMI_SIGNING_PASSWORD
printf '%s' "$key" | gh secret set MYGO_UPDATER_PRIVATE_KEY
echo "已写入 Actions secrets：KAMI_SIGNING_IDENTITY、KAMI_SIGNING_PASSWORD、MYGO_UPDATER_PRIVATE_KEY。" >&2
echo "试一次签名（不上传）：gh workflow run release" >&2
