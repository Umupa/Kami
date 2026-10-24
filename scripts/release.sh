#!/usr/bin/env bash
# Releases version <x.y.z> from main: runs the checks, sets the version in
# mygo.json, commits, tags and pushes. The tag starts the release workflow
# (.github/workflows/release.yml), which builds Kami for Apple silicon and
# Intel, signs it with the "Kami Dev" identity, uploads the disk images and
# the signed updates, publishes the release and points the Homebrew cask at
# it; this script follows it to the end. Installed copies find the release
# at their next check. CHANGELOG.md's "## <x.y.z>" section is the release
# notes the update screen shows.
set -euo pipefail

cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
version="${1:-}"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	echo "用法：scripts/release.sh <版本，如 0.2.0>" >&2
	exit 1
fi
tag="v$version"

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
secrets=$(gh secret list --json name --jq '.[].name')
for secret in KAMI_SIGNING_IDENTITY KAMI_SIGNING_PASSWORD MYGO_UPDATER_PRIVATE_KEY HOMEBREW_TAP_KEY; do
	if ! grep -qx "$secret" <<<"$secrets"; then
		echo "仓库的 Actions secrets 里没有 ${secret}，先运行 scripts/ci-secrets.sh。" >&2
		exit 1
	fi
done

echo "检查…" >&2
scripts/check.sh

if ! grep -q "\"version\": \"$version\"" mygo.json; then
	sed -i '' -E "s/\"version\": \"[0-9.]+\"/\"version\": \"$version\"/" mygo.json
	git commit -q -m "build: release $version" mygo.json
fi
git rev-parse -q --verify "refs/tags/$tag" >/dev/null || git tag "$tag"
git push -q origin main "$tag"

# The run shows up a few seconds after the push.
run=""
for _ in $(seq 30); do
	run=$(gh run list --workflow release --branch "$tag" --limit 1 --json databaseId --jq '.[0].databaseId // empty')
	[[ -n "$run" ]] && break
	sleep 2
done
if [[ -z "$run" ]]; then
	echo "已推送 ${tag}，发布在 GitHub Actions 里进行：gh run list --workflow release" >&2
	exit 0
fi
echo "发布在 GitHub Actions 里进行（构建、签名、上传、更新 Homebrew），约十分钟…" >&2
if ! gh run watch "$run" --exit-status --interval 15 >/dev/null; then
	echo "发布没有完成：gh run view $run --log-failed。修好后在 Actions 里重新运行失败的任务，草稿 $tag 会被沿用。" >&2
	exit 1
fi
echo "已发布 ${tag}：$(gh release view "$tag" --json url --jq .url)" >&2
