# AGENTS.md

所有参与开发 Kami 的人与 agent 的工作规则。产品介绍见 [README.md](README.md)，开发用的脚本见下面的目录。

## 目录

| 路径 | 内容 |
| --- | --- |
| `docs/requirements.md` | 需求：Kami 做什么、做成什么样 |
| `docs/architecture.md` | 架构：技术栈、模块、数据流、核心机制、测试 |
| `docs/decisions.md` | 决策记录：关键决定、理由与放弃的方案 |
| `docs/media/` | README 的动图与截图，从官网的演示录制；演示改了就重新录制 |
| `internal/` | 应用代码，模块划分见 architecture.md §二 |
| `main.go` | 入口：带子命令（`check`）时是命令行，否则启动应用 |
| `assets/` | 图稿原图；`swift scripts/icon.swift` 由 `icon-fox.png` 生成 `resources/icon.png`，只在换图稿时运行 |
| `scripts/` | `check.sh` 本地检查；`build.sh` 构建签名的 `.app`；`dev.sh` 签名的热重载开发运行；`dev-cert.sh` 创建本机签名身份（前两者自动调用）；`reset-permissions.sh` 清除授权记录；`release.sh <版本>` 打 tag 触发发布工作流并跟到结束，`cask.sh <版本>` 输出 Homebrew cask（发布工作流写进 tap `Umupa/homebrew-kami`），`ci-secrets.sh` 把签名身份与更新私钥写进 Actions secrets，`update-key.sh` 一次性生成更新签名密钥 |
| `CHANGELOG.md` | 每个版本的更新内容，写给用户看，也是更新界面里显示的文字 |
| `site/` | 官网：静态页面，不需要构建；演示面板在网页里重绘（D28），`themes.js` 由 `go test ./internal/design -update` 写出，`prompt.js` 与 README 里的安装提示词由 `go test ./internal/config -update` 写出 |

## 文档先行

- `docs/requirements.md` 是需求的唯一来源，`docs/architecture.md` 是结构的唯一来源。改行为或结构的变更，在同一个提交里更新对应文档。
- 新的关键决定写进 `docs/decisions.md`；推翻旧决定时改原条目，不保留过时版本。
- 文档只写当前状态：不写开发过程、验收记录和待办，这些留在提交信息里。
- 引入第三方代码时保留其版权与许可声明。

## 语言

- 代码、注释、标识符、提交信息用英文。
- 文档用简体中文；界面文字用简体中文。

## 代码

- 业务核心（`session`、`mode`、`llm`、`config`）不依赖 UI 和系统框架，必须能无窗口测试。
- purego 调用只出现在 `internal/macos`，其他包通过它暴露的 Go 接口使用系统能力。
- 每个系统调用都有超时；任何失败都不能导致崩溃。
- 注释解释代码表达不了的约束，不写历史。

## 验证

- 每次改动都要通过 `scripts/check.sh`（gofmt、arm64 与 amd64 的 `go vet`、`go tool mygo vet`、`go test`）；改了 `internal/macos` 时运行 `scripts/check.sh --integration`。
- CI 只在手动触发时运行（`gh workflow run ci`），提交前在本机跑完检查。发布由 `scripts/release.sh` 推送的 tag 触发 `release` 工作流；`gh workflow run release` 只构建、签名，不上传，用来试 secrets。
- 改了面板、Markdown 渲染或主题时，更新视觉快照并检查 PNG。
- 改了 `site/`，或面板的结构、文字有变化时，在浏览器里检查官网的演示面板，以及桌面端与手机端的布局。
- 系统集成测试用 `//go:build integration`，只做只读调用。

## 不要碰开发者的屏幕

开发者在这台 Mac 上工作，测试和调试时：

- 不发送合成的键盘、鼠标事件；模拟 ⌘C 只能由用户按下快捷键触发；双击 ⌥ 的事件监听只读，不拦截、不改写事件。
- 不截取实时屏幕，不录屏。
- 不写系统剪贴板；剪贴板相关测试用 `NSPasteboard pasteboardWithUniqueName` 创建的私有剪贴板。
- 不激活、不前置其他应用的窗口。
- 全局快捷键用默认值：⌥空格面板、双击 ⌥ 带入选区、⌥S 截图（D7）。
- 不读、不改开发者真实的 `config.json`（里面有 API Key）；测试用临时目录。

## 提交

- 提交信息以类型开头：`feat:`、`fix:`、`docs:`、`test:`、`refactor:`、`build:`、`ci:`，正文说明为什么要改。
- 不提交任何密钥。API Key 只在开发者本机的 `config.json` 里，不进仓库、不进日志；签名证书与更新私钥在本机钥匙串里，另一份只在仓库的 Actions secrets 里（`scripts/ci-secrets.sh` 写入），供发布工作流使用。
