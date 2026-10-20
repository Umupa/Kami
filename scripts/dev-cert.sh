#!/usr/bin/env bash
# Creates the self-signed code signing identity "Gloss Dev" in the login
# keychain, once per Mac. Builds signed with it keep their Accessibility,
# Input Monitoring and Screen Recording grants across rebuilds (decisions D8).
# macOS asks for your password to trust the certificate for code signing.
set -euo pipefail

name="Gloss Dev"
keychain="$HOME/Library/Keychains/login.keychain-db"

if security find-identity -v -p codesigning | grep -q "\"$name\""; then
	echo "已经有签名身份「$name」。"
	exit 0
fi

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
	-out "$tmp/id.p12" -passout pass:Gloss
security import "$tmp/id.p12" -k "$keychain" -P Gloss -T /usr/bin/codesign
security add-trusted-cert -p codeSign -k "$keychain" "$tmp/cert.pem"

security find-identity -v -p codesigning | grep "\"$name\""
echo "完成：之后用 scripts/build.sh 构建。"
