package config

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the site's copy of the install prompt")

// The site's "交给 Agent 安装" button copies the installation prompt.
const (
	sitePrompt   = "../../site/prompt.js"
	installSteps = `你好！请帮我在这台 Mac 上安装 Kami，再把它连上一个 AI 模型。Kami 需要 macOS 15.2 或更新版本。

## 先装好 Kami（不用问我）

1. 用 ` + "`curl -s https://api.github.com/repos/Umupa/Kami/releases/latest`" + ` 找到最新版本，按 ` + "`uname -m`" + ` 选 DMG：arm64 选名字以 ` + "`.arm64.dmg`" + ` 结尾的，x86_64 选以 ` + "`.amd64.dmg`" + ` 结尾的。没有对应的 DMG 就停下，告诉我这台 Mac 暂时还不能用。
2. 下载到临时目录，用 ` + "`hdiutil attach -nobrowse`" + ` 挂载，把 Kami.app 复制到 /Applications（已经装过的话，先退出 Kami 再替换），然后卸载 DMG、删掉临时文件。
3. 用 ` + "`open -a Kami`" + ` 打开一次。Kami 只在菜单栏显示一只小狐，第一次打开时会写出下面说到的配置文件和字段说明。

装好后，照下面这段说明帮我接上模型。路径里的 ~ 是我的用户目录。

---

`
)

// installPrompt is the steps that install Kami, then the configuration
// prompt the app itself gives, for a copy in /Applications with the default
// settings.
func installPrompt(t *testing.T) string {
	t.Helper()
	c, err := Decode(fmt.Appendf(nil, `{"version":%d}`, Version))
	if err != nil {
		t.Fatal(err)
	}
	return installSteps + AgentPrompt(PromptInput{
		Exe:    "/Applications/Kami.app/Contents/MacOS/Kami",
		Dir:    "~/Library/Application Support/Kami",
		Config: c,
	})
}

// TestSiteInstallPrompt keeps the site in sync with the app configuration prompt.
// Run go test ./internal/config -update to regenerate it.
func TestSiteInstallPrompt(t *testing.T) {
	prompt := installPrompt(t)

	text, err := json.Marshal(prompt)
	if err != nil {
		t.Fatal(err)
	}
	var site bytes.Buffer
	site.WriteString("// The prompt that installs Kami and connects a model, which the site's\n")
	site.WriteString("// \"交给 Agent 安装\" button copies. Written by `go test ./internal/config\n")
	site.WriteString("// -update`; do not edit.\n")
	fmt.Fprintf(&site, "window.KAMI_AGENT_PROMPT = %s;\n", text)

	if *update {
		if err := os.WriteFile(sitePrompt, site.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if got, err := os.ReadFile(sitePrompt); err != nil || !bytes.Equal(got, site.Bytes()) {
		t.Errorf("%s is out of date; run go test ./internal/config -update", sitePrompt)
	}
}
