#!/usr/bin/env bash
# Creates the self-signed code signing identity "Kami Dev" in the login
# keychain, once per Mac; does nothing when it exists. Builds signed with it
# keep their Accessibility, Input Monitoring and Screen Recording grants
# across rebuilds (decisions D8).
# macOS asks for your password to trust the certificate for code signing.
set -euo pipefail

name="Kami Dev"
keychain="$HOME/Library/Keychains/login.keychain-db"

if security find-identity -v -p codesigning | grep -q "\"$name\""; then
	exit 0
fi

echo "创建签名身份「${name}」（每台 Mac 一次），macOS 会要求输入密码。" >&2
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

cat >"$tmp/cert.conf" <<CONF
[req]
distinguished_name = dn
x509_extensions = ext
prompt = no
[dn]
CN = $name
[ext]
basicConstraints = critical, CA:false
keyUsage = critical, digitalSignature
extendedKeyUsage = critical, codeSigning
CONF

/usr/bin/openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
	-config "$tmp/cert.conf" -keyout "$tmp/key.pem" -out "$tmp/cert.pem" 2>/dev/null
/usr/bin/openssl pkcs12 -export -inkey "$tmp/key.pem" -in "$tmp/cert.pem" \
	-out "$tmp/id.p12" -passout pass:kami
security import "$tmp/id.p12" -k "$keychain" -P kami -T /usr/bin/codesign
security add-trusted-cert -p codeSign -k "$keychain" "$tmp/cert.pem"

echo "已创建签名身份「${name}」。" >&2
