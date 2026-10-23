# Kami

完整产品名为 **Kami · 墨**。Kami 意为纸，与「墨」共同表达纸与文字的意象。

我自己的桌面 AI 工具箱：一个常驻菜单栏的 macOS 原生应用。⌥空格打开面板，双击 ⌥ 带着选中的文字打开，⌥S 截图识别文字；用自己配置的大模型完成翻译、润色（杂乱时顺带整理）、提示词梳理和快速问答，结果以 Markdown 流式呈现，可以继续追问。面板里还有随手记录的笔记，不接模型。

用 Go 和 [MyGo](https://github.com/egoist/mygo) 编写；设置全在一个带 schema 的配置文件里，交给 Coding Agent 配置，在面板里输入 `/operator` 也能查看和修改。

## 安装与更新

从 [Releases](https://github.com/Umupa/Kami/releases) 下载 DMG，把 Kami 拖进「应用程序」。需要 macOS 15.2 及以上、Apple Silicon。Kami 用自签名证书签名，第一次打开要在 Finder 里右键 Kami 选「打开」，或在「系统设置 → 隐私与安全性」里点「仍要打开」。之后的版本由 Kami 自己检查和安装：打开面板时会提示新版本。

## 构建与开发

| 命令 | 作用 |
| --- | --- |
| `scripts/check.sh` | 提交前的检查：gofmt、`go vet`、`go tool mygo vet`、`go test`；`--integration` 加跑只读的 macOS 集成测试 |
| `scripts/build.sh` | 构建 `build/darwin-arm64/Kami.app`，用本机签名身份 `Kami Dev` 签名；第一次运行时自动创建该身份（需要输入一次系统密码） |
| `scripts/dev.sh` | 带热重载的开发运行，同样签名。它是另一个应用 `Kami Dev`，权限与数据目录和正式构建分开；没有模型时显示首次使用引导，`KAMI_DEMO=1 scripts/dev.sh` 改用演示回复 |
| `scripts/reset-permissions.sh [--dev]` | 清除 Kami 的辅助功能、输入监控、屏幕录制记录，用于授权错乱时 |
| `scripts/release.sh <版本>` | 发布新版本：检查、改版本号、打 tag、构建签名并上传到 GitHub Releases；更新内容取自 `CHANGELOG.md` |
| `scripts/update-key.sh` | 一次性生成更新签名密钥，私钥存进钥匙串（要另外备份） |
| `swift scripts/icon.swift` | 换了 `assets/icon-fox.png` 之后重新生成应用图标 |

签名固定后，重新构建不会丢失系统权限。`scripts/build.sh --ad-hoc` 只用于临时产物，它的权限在重新构建后失效。

**权限总是要重新授予？** 系统设置里的开关可能属于之前的某次构建，所以显示开启却不生效。运行 `scripts/reset-permissions.sh`，或在系统设置里用「−」移除 Kami 后重新添加。输入监控和屏幕录制授权后，从菜单栏选「重新启动 Kami」才会生效。

**API Key 在哪里？** 在 `~/Library/Application Support/Kami/config.json` 的 `model.apiKey`，文件权限 0600，不进钥匙串（见 [D12](docs/decisions.md)）。

## 文档

| 文档 | 内容 |
| --- | --- |
| [需求](docs/requirements.md) | 定位与原则、场景、功能需求、非功能指标、不做的事 |
| [架构](docs/architecture.md) | 技术栈、模块划分、数据流、提示词合约、入口流程、配置接口、测试 |
| [决策记录](docs/decisions.md) | 已经做出的关键决定及其理由 |

参与开发前先读 [AGENTS.md](AGENTS.md)。

## 许可

[Apache-2.0](LICENSE)。内置的 CaskaydiaCove Nerd Font 使用 SIL Open Font License，见 `internal/design/fonts/LICENSE`。
