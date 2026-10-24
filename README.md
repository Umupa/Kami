<p align="center">
  <img src="resources/icon.png" width="128" alt="Kami 的图标：浅色底上的黑色小狐">
</p>

<h1 align="center">Kami</h1>

<p align="center">
  常驻菜单栏的 Mac 桌面 AI 工具箱：一个快捷键，把手头的文字交给你选择的大模型。<br>
  <a href="https://github.com/Umupa/Kami/releases/latest">下载</a>
</p>

## 怎么用

- **选中文字，双击 <kbd>⌥</kbd>。** 面板带着选中的文字出现，立即执行默认模式，即翻译。翻译时，其他语言都译成你的语言（默认简体中文），你的语言译成外语（默认英文）。终端里的框线和硬换行会被清理掉，代码、路径和命令保持原样，适合读 Claude Code 的英文输出。

- **按 <kbd>Tab</kbd> 换个模式，按 <kbd>Return</kbd> 执行。** 内置三个模式：
  - **翻译**：在你的语言和外语之间互译，译文读起来自然；内容密集时整理成段落、列表和表格。
  - **润色**：保持原来的语言，改用词、语法和语气，不改意思。聊天记录、网页或 PDF 里复制出来的杂乱文字，会顺带整理成标题、列表和表格，不增删事实。
  - **问答**：先给结论，回答简短；不确定时会说明。

  每个模式各留一份结果，切过去就能看到。

- **对结果不满意，直接追问。** 「再短一点」「换个语气」「表格加一列」：任何模式的结果之后都能接着聊，<kbd>⌘</kbd> <kbd>J</kbd> 在原文和追问之间切换。

- **结果拿来就能用。** 回复以 Markdown 流式显示，支持表格、代码高亮和公式。<kbd>⌘</kbd> <kbd>C</kbd> 复制的同时带上富文本，粘到飞书、Notion 里表格还是表格；代码块有单独的复制按钮。

- **屏幕上的文字，按 <kbd>⌥</kbd> <kbd>S</kbd>。** 框出图片、视频字幕或远程桌面里的一块区域，Kami 在本机识别文字并接回段落，然后翻译。

- **按 <kbd>⌥</kbd> <kbd>Space</kbd> 打开面板，什么都能输入。** 输入或粘贴一段文字，选好模式按 <kbd>Return</kbd>。面板不抢原应用的焦点，按 <kbd>Esc</kbd> 隐藏；隐藏后请求继续完成，下次打开时结果还在。<kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>P</kbd> 固定面板，切到别的应用它也不消失。

- **随手记一笔。** 模式条的最后一项是笔记，不接模型，打开就写，停下就自动保存成 Markdown 文件。<kbd>⌘</kbd> <kbd>N</kbd> 新建，<kbd>⌘</kbd> <kbd>P</kbd> 打开笔记列表，输入即可搜索，找不到就按 <kbd>Return</kbd> 用这个标题新建；<kbd>⌘</kbd> <kbd>[</kbd> <kbd>⌘</kbd> <kbd>]</kbd> 在最近的笔记之间切换。

- **用一句话改设置。** 在面板里输入 `/operator` 加一句话，比如「双击 ⌥ 改成双击 ⌘」「加一个改写成邮件的模式」「把润色的提示词改得更口语」，改动立即生效，回复里列出改前和改后，<kbd>⌘</kbd> <kbd>Z</kbd> 撤销。输入 `/theme` 从 18 套内置主题里挑一个，边选边预览，也可以让 Operator 调出一套自己的配色。

## 安装

需要 macOS 15.2 或更新版本，以及一个大模型。

1. 从 [Releases](https://github.com/Umupa/Kami/releases/latest) 下载 DMG：Apple Silicon 选 `arm64.dmg`，Intel 选 `amd64.dmg`。
2. 打开 DMG，把 Kami 拖进「应用程序」。Kami 用自签名证书签名，第一次打开要在 Finder 里右键 Kami 选「打开」，或在「系统设置 → 隐私与安全性」里点「仍要打开」。Kami 只在菜单栏显示一只小狐，不占用 Dock。
3. 第一次打开时，面板里是一段配置提示词。按 <kbd>Return</kbd> 复制它，交给 Claude Code、Codex 等 Coding Agent。Agent 会一次问你一个问题，帮你接好模型，再运行 `check` 确认能用。

手上是 API Key、Claude / ChatGPT 等订阅，还是本机模型，都能接：支持 OpenAI Chat Completions、Responses 与 Anthropic Messages 三种接口，本机模型不需要 Key，订阅通过 [magpie](https://github.com/yetone/magpie) 网关使用。API Key 不经过对话：Agent 会请你自己复制 Key，再由它写进配置文件。Kami 本身免费，模型调用按服务商的价格计费。

连上模型后，Kami 会带你逐项授权。权限都是可选的，缺了哪项只影响对应的入口：

| 权限 | 开启后 | 不开启时 |
| --- | --- | --- |
| 辅助功能 | 双击 <kbd>⌥</kbd> 读取选中的文字 | 先 <kbd>⌘</kbd> <kbd>C</kbd>，再在面板里 <kbd>⌘</kbd> <kbd>V</kbd> |
| 输入监控 | 监听双击 <kbd>⌥</kbd> | 双击 <kbd>⌥</kbd> 不可用，<kbd>⌥</kbd> <kbd>Space</kbd> 照常 |
| 屏幕录制 | 用 <kbd>⌥</kbd> <kbd>S</kbd> 截图识别文字 | 截图不可用 |

输入监控和屏幕录制授权后，要从菜单栏选「重新启动 Kami」才生效。系统设置里开关显示已打开却不起作用时，用「−」把 Kami 从列表里移除，再重新添加。

## 隐私

- 不保存会话、原文和结果，退出即消失；日志只记事件和耗时，不记任何文字。笔记是唯一会保存的内容，因为它是你主动写下的。
- 请求只发往你配置的服务商，不收集任何数据。
- 截图在本机用 Apple 的 Vision 识别，图片不离开你的 Mac，也不保存。
- API Key 只存在本机的 `~/Library/Application Support/Kami/config.json` 里，文件权限 0600；日志、命令输出和配置提示词里都不会出现。

## 快捷键

| 按键 | 作用 |
| --- | --- |
| <kbd>⌥</kbd> <kbd>Space</kbd> | 显示或隐藏面板 |
| 双击 <kbd>⌥</kbd> | 带着选中的文字打开面板，并执行默认模式 |
| <kbd>⌥</kbd> <kbd>S</kbd> | 截图识别文字并翻译 |
| <kbd>Tab</kbd> / <kbd>⇧</kbd> <kbd>Tab</kbd> | 切换模式 |
| <kbd>Return</kbd> | 执行或追问 |
| <kbd>⌘</kbd> <kbd>J</kbd> | 在原文和追问之间切换 |
| <kbd>⌘</kbd> <kbd>N</kbd> | 新会话；在笔记里是新建笔记 |
| <kbd>⌘</kbd> <kbd>P</kbd> | 笔记列表 |
| <kbd>⌘</kbd> <kbd>.</kbd> | 停止生成 |
| <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>P</kbd> | 固定面板 |
| <kbd>⌘</kbd> <kbd>=</kbd> / <kbd>⌘</kbd> <kbd>-</kbd> / <kbd>⌘</kbd> <kbd>0</kbd> | 放大、缩小、恢复字号 |
| <kbd>/</kbd> | 命令列表：Operator、主题和各个模式 |
| <kbd>Esc</kbd> | 隐藏面板 |

三个全局快捷键都可以改成别的组合键或双击修饰键，用 `/operator` 说一句就行。

## 设置

Kami 没有设置窗口。全部设置在 `~/Library/Application Support/Kami/config.json` 里，包括模型、模式与提示词、快捷键、你的语言和外语、主题、笔记目录、开机启动和 Dock 图标。旁边的 `config.schema.json` 写清了每个字段，改完立即生效，写错了也不会崩溃，菜单栏会提示有几处错误。

日常的修改用 `/operator` 说一句话；换模型、改 Key 交给 Coding Agent，菜单栏的「复制提示词」随时给你那份配置提示词。Kami 用着有问题时，也可以把它交给 Agent 检查。

应用里的可执行文件也是命令行，Agent 用它确认模型能用：

```sh
/Applications/Kami.app/Contents/MacOS/Kami check --verbose   # 真的请求一次，列出请求与响应，Key 打码
```

## 更新

Kami 定期检查 GitHub Releases，有新版本时，在你下次打开面板时显示更新内容，可以「安装并重启」「稍后」或「跳过这个版本」。更新只接受用发布私钥签名的版本，安装后系统权限保留。

## 卸载

退出 Kami，把「应用程序」里的 Kami 移到废纸篓。想清掉全部痕迹，再删除 `~/Library/Application Support/Kami`（配置与笔记，笔记要先备份）和 `~/Library/Logs/Kami`，并在「系统设置 → 隐私与安全性」的辅助功能、输入监控、屏幕录制里移除 Kami。

## 参与开发

```sh
git clone https://github.com/Umupa/Kami.git
cd Kami
scripts/dev.sh      # 带热重载的开发运行
scripts/check.sh    # 提交前的检查
```

用 Go 和 [MyGo](https://github.com/egoist/mygo) 编写。需求、架构和决策记录在 [`docs/`](docs/)，提交改动前请阅读 [`AGENTS.md`](AGENTS.md)。

## 许可证

[Apache-2.0](LICENSE)。内置的 CaskaydiaCove Nerd Font 使用 SIL Open Font License，见 `internal/design/fonts/LICENSE`。

Kami 是日语里的「纸」。
