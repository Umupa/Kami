# AGENTS.md

所有参与开发 Kami 的人与 agent 的工作规则。项目目标见 [docs/vision.md](docs/vision.md)，需求见 [docs/requirements.md](docs/requirements.md)。

## 目录

| 路径 | 内容 |
| --- | --- |
| `docs/` | 目标、需求、技术栈、架构、路线图、决策记录 |
| `internal/` | 应用代码，模块划分见 [docs/architecture.md](docs/architecture.md) §一（M0 建立） |
| `main.go` | 入口：带子命令（`check`）时是命令行，否则启动应用（M0 建立） |
| `scripts/` | `build.sh` 构建签名的 `.app`；`dev.sh` 签名的热重载开发运行；`dev-cert.sh` 创建本机签名身份（每台 Mac 一次，前两者缺少时自动调用）；`reset-permissions.sh` 清除授权记录 |
| `scripts/dst/` | 把任务交给 dst 的脚本与配置（见「协作开发」） |
| `.dst/` | 任务书与运行日志，不提交 |

## 文档先行

- `docs/requirements.md` 是需求的唯一来源，`docs/architecture.md` 是结构的唯一来源。改行为或结构的变更，在同一个提交里更新对应文档。
- 新的关键决定写进 `docs/decisions.md`；推翻旧决定时改原条目，不保留过时版本。
- 行为约定写在本项目的需求与架构文档里，引用时写明文件与章节。引入第三方代码时保留其版权与许可声明。

## 语言

- 代码、注释、标识符、提交信息用英文。
- 文档用简体中文；界面文字用简体中文。

## 代码

- 业务核心（`session`、`mode`、`llm`、`config`）不依赖 UI 和系统框架，必须能无窗口测试。
- purego 调用只出现在 `internal/macos`，其他包通过它暴露的 Go 接口使用系统能力。
- 每个系统调用都有超时；任何失败都不能导致崩溃。
- 注释解释代码表达不了的约束，不写历史。

## 验证

- 每次改动都要通过：`gofmt -l .` 无输出、`go vet ./...`、`go tool mygo vet .`、`go test ./...`。
- CI 只在手动触发时运行（`gh workflow run ci`），提交前在本机跑完上面的检查；改了 `internal/macos` 时再跑 `go test -tags integration ./internal/macos`。
- 改了面板、Markdown 渲染或主题时，更新视觉快照并检查 PNG。
- 系统集成测试用 `//go:build integration`，只做只读调用。

## 不要碰开发者的屏幕

开发者在这台 Mac 上工作，测试和调试时：

- 不发送合成的键盘、鼠标事件；模拟 ⌘C 只能由用户按下快捷键触发；双击 ⌥ 的事件监听只读，不拦截、不改写事件。
- 不截取实时屏幕，不录屏。
- 不写系统剪贴板；剪贴板相关测试用 `NSPasteboard pasteboardWithUniqueName` 创建的私有剪贴板。
- 不激活、不前置其他应用的窗口。
- 全局快捷键用默认值：⌥空格面板、双击 ⌥ 带入选区、⌥S 截图（见 docs/decisions.md D7）。
- 不读、不改开发者真实的 `config.json`（里面有 API Key）；测试用临时目录。

## 协作开发

主开发者是 Claude Code，负责架构、核心代码、UI / UX、review 与提交。次要的编码工作交给 **dst**：用户本机的 DeepSeek Harness（`dsh-tui`），由 Claude 以无界面方式调用，思考强度固定为最高档。dst 的产出都要经过 Claude review 才能进入提交。

### 分工

| Claude | dst |
| --- | --- |
| 模块边界、接口、类型与 doc comment；`docs/` | 按已定接口实现的叶子函数 |
| UI：面板、Markdown 呈现、设计 token、交互与动效 | 测试补全：表格驱动用例、`httptest` 的 SSE 夹具、配置校验用例 |
| `session` 状态机、提示词合约、并发与 `context` 取消 | 有样板之后的同类实现（例如写好一种接口格式后的另外两种） |
| `internal/macos` 的 purego 绑定 | schema 字段说明、错误码到类别的映射、命令行参数解析 |
| API Key 的处理与打码等安全相关代码 | 机械重构、重命名、按代码同步文档 |
| review、最终验证、提交 | — |

判断标准：需要设计取舍、跨模块、或者错了很难看出来的，Claude 做；接口已定、能用测试验证对错的，交给 dst。

### 流程

1. **先定接口。** Claude 写好类型、函数签名、doc comment，最好再加一个样板实现或测试，让 dst 照着写。
2. **写任务书**：`.dst/tasks/<名字>.md`，包含目标、允许改动的文件清单、要实现的接口、边界情况、验收命令、不要做的事。任务要小到一次 review 能看完。
3. **工作区干净时派发**：`scripts/dst/run.sh .dst/tasks/<名字>.md`。脚本会在任务前加上固定的约束（只改清单内文件、不跑 git 写操作、不启动应用、不碰屏幕和剪贴板、结束前跑验证），完整事件流存进 `.dst/runs/`。同一时间只跑一个任务，因为它和 Claude 共用工作区。
4. **Review**：用 `git status` 和 `git diff` 看全部改动。清单外的改动直接撤回；按本文的代码规则和周围代码的写法检查；Claude 自己再跑一遍「验证」里的命令。
5. **反馈**：把问题写进新的任务书，带上 `session:` 一行打出的 id 接着跑：`scripts/dst/run.sh <反馈.md> <session-id>`。最多两轮，还不合格就由 Claude 自己改完。
6. **提交**：只由 Claude 提交，规则同下。

### 约束

- dst 能直接执行 shell 命令，没有逐条确认。任务书里不放任何密钥，也不让它操作仓库以外的东西。
- 模型与思考强度在 `scripts/dst/worker.patch.yml` 里，提供方和模型名要与用户的 `dsh-tui` 配置一致。
- dst 跑失败、超时或明显跑偏时，Claude 自己做，不反复重试。

## 提交

- 提交信息以类型开头：`feat:`、`fix:`、`docs:`、`test:`、`refactor:`、`build:`、`ci:`，正文说明为什么要改。
- 不提交任何密钥。API Key 只在开发者本机的 `config.json` 里，不进仓库、不进日志；签名证书只在本机钥匙串里。
