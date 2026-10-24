#!/usr/bin/env bash
# Writes the Homebrew cask of released version <x.y.z> to standard output,
# its checksums taken from the disk images of the GitHub release. The
# release workflow commits it to the tap Umupa/homebrew-kami, as
# Casks/kami.rb, where `brew install --cask umupa/kami/kami` finds it.
set -euo pipefail

cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
version="${1:-}"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	echo "用法：scripts/cask.sh <版本，如 0.2.0>" >&2
	exit 1
fi
if ! gh release view "v$version" --json tagName >/dev/null 2>&1; then
	echo "没有 v$version 这个 Release。" >&2
	exit 1
fi

digest() {
	gh release view "v$version" --json assets \
		--jq ".assets[] | select(.name == \"Kami.$version.$1.dmg\") | .digest | ltrimstr(\"sha256:\")"
}
arm=$(digest arm64)
intel=$(digest amd64)
if [[ -z "$arm" ]]; then
	echo "v$version 里没有 Apple 芯片的 DMG。" >&2
	exit 1
fi

# Releases before 0.2.0 have an Apple silicon disk image only.
if [[ -n "$intel" ]]; then
	head="  arch arm: \"arm64\", intel: \"amd64\"

  version \"$version\"
  sha256 arm:   \"$arm\",
         intel: \"$intel\"

  url \"https://github.com/Umupa/Kami/releases/download/v#{version}/Kami.#{version}.#{arch}.dmg\""
	needs=""
else
	head="  version \"$version\"
  sha256 \"$arm\"

  url \"https://github.com/Umupa/Kami/releases/download/v#{version}/Kami.#{version}.arm64.dmg\""
	needs="
  depends_on arch: :arm64"
fi

# Kami updates itself, so brew leaves upgrades to it. Zap keeps the notes:
# they are what the user wrote.
cask="cask \"kami\" do
$head
  name \"Kami\"
  desc \"常驻菜单栏的桌面 AI 工具箱\"
  homepage \"https://github.com/Umupa/Kami\"

  livecheck do
    url :url
    strategy :github_latest
  end

  auto_updates true$needs
  depends_on macos: :sequoia

  app \"Kami.app\"

  uninstall quit: \"dev.umupa.kami\"

  zap trash: [
        \"~/Library/Application Support/Kami/config.json\",
        \"~/Library/Application Support/Kami/config.schema.json\",
        \"~/Library/Logs/Kami\",
        \"~/Library/Preferences/dev.umupa.kami.plist\",
      ],
      rmdir: \"~/Library/Application Support/Kami\"

  caveats <<~EOS
    Kami 用自签名证书签名。第一次打开时 macOS 会提示无法验证开发者：
    在「系统设置 → 隐私与安全性」里点「仍要打开」。
  EOS
end"

printf '%s\n' "$cask"
