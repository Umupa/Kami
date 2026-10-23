#!/usr/bin/env bash
# Publishes version <x.y.z> of Kami from main: runs the checks, sets the
# version in mygo.json, tags and pushes it, builds the disk image and the
# signed update with the "Kami Dev" identity and the key in the keychain,
# uploads them to the GitHub release v<x.y.z> and publishes it. Installed
# copies find it at their next check. CHANGELOG.md's "## <x.y.z>" section
# is the release notes the update screen shows.
set -euo pipefail

cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
version="${1:-}"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	echo "用法：scripts/release.sh <版本，如 0.2.0>" >&2
	exit 1
fi
tag="v$version"
name="Kami Dev"

if [[ "$(git branch --show-current)" != main ]]; then
	echo "只从 main 发布。" >&2
	exit 1
fi
if ! git diff --quiet || ! git diff --cached --quiet; then
	echo "还有没提交的改动，先提交。" >&2
	exit 1
fi
if ! grep -q "^## $version\$" CHANGELOG.md; then
	echo "CHANGELOG.md 里还没有「## $version」这一段：它是更新界面里的更新内容。" >&2
	exit 1
fi
if gh release view "$tag" --json isDraft --jq 'select(.isDraft == false)' 2>/dev/null | grep -q .; then
	echo "$tag 已经发布过。" >&2
	exit 1
fi
if ! key=$(security find-generic-password -s "Kami update key" -w 2>/dev/null); then
	echo "钥匙串里没有更新签名私钥，先运行 scripts/update-key.sh。" >&2
	exit 1
fi
if ! security find-identity -v -p codesigning | grep -q "\"$name\""; then
	echo "没有签名身份「${name}」：每个版本都要用同一个身份签名，系统权限才能在更新后保留。" >&2
	exit 1
fi

echo "检查…" >&2
scripts/check.sh

if ! grep -q "\"version\": \"$version\"" mygo.json; then
	sed -i '' -E "s/\"version\": \"[0-9.]+\"/\"version\": \"$version\"/" mygo.json
	git commit -q -m "build: release $version" mygo.json
fi
git rev-parse -q --verify "refs/tags/$tag" >/dev/null || git tag "$tag"
git push -q origin main "$tag"

MYGO_UPDATER_PRIVATE_KEY="$key" go tool mygo build -sign "$name" -upload
# A requirement that names only the cdhash would change with every version,
# and with it every grant of every installed copy.
if codesign -dr - build/darwin-arm64/Kami.app 2>&1 | grep -q 'designated => cdhash'; then
	echo "Kami.app 没有用「${name}」签名，不发布：草稿 $tag 留着，修好后重新运行。" >&2
	exit 1
fi
gh release edit "$tag" --draft=false --latest
echo "已发布 ${tag}。" >&2
