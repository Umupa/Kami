# 技术栈

总览：**Go + MyGo 原生 UI + purego 直连 macOS 系统框架**。不用 cgo，不用 Swift，不用 WebView，不依赖任何大模型 SDK。一个 Go 二进制同时是应用和命令行。

```
┌───────────────────────────────────────────────────────────────────┐
│ 界面层      MyGo ui（GPU 自绘，Core Text 排字） + ui/markdown           │
├───────────────────────────────────────────────────────────────────┤
│ 桌面能力    MyGo：面板窗口、全局快捷键、菜单栏、剪贴板、                 │
│            透明穿透窗口、开机启动、单实例、打包签名、更新                  │
├───────────────────────────────────────────────────────────────────┤
│ 系统集成    purego / purego/objc → ApplicationServices（辅助功能）、       │
│            CoreGraphics（CGEvent）、AppKit（NSPasteboard）、Vision、     │
│            ScreenCaptureKit、Carbon（键盘布局）                          │
├───────────────────────────────────────────────────────────────────┤
│ 业务核心    纯 Go：模式、会话、提示词合约、配置与热加载、分段、错误分类      │
├───────────────────────────────────────────────────────────────────┤
│ 模型接入    net/http + 自写 SSE：Chat Completions / Responses / Messages │
└───────────────────────────────────────────────────────────────────┘
```

## 一、语言与工具链

| 项 | 选择 | 说明 |
| --- | --- | --- |
| 语言 | Go 1.27.x | MyGo 要求 1.27+；`go.mod` 写 `go 1.27.2`，`GOTOOLCHAIN=auto` 自动下载 |
| 构建 | `go build` / `go tool mygo` | MyGo 的 CLI 作为 module tool（`go.mod` 的 `tool` 指令），不全局安装 |
| 开发 | `go tool mygo dev` | 带热重载的开发运行 |
| 打包 | `go tool mygo build` | 生成 `.app`、DMG，按 `mygo.json` 签名 |
| 静态检查 | `go vet`、`gofmt`、`go tool mygo vet`（检查 UI 代码） | CI 必过 |
| 不需要 | Xcode、cgo、Node/Bun | 原生 UI 模板只要 Go；只装 Command Line Tools 即可 |

## 二、界面：MyGo 原生 UI

| 项 | 选择 | 说明 |
| --- | --- | --- |
| 框架 | [MyGo](https://github.com/egoist/mygo) **v0.5.5，锁定精确版本** | MIT；2026-09-26 创建，迭代很快，升级要单独评估（见 §八） |
| UI 模式 | 原生 UI（`github.com/egoist/mygo/ui`），**不用 WebView** | 窗口打开即有内容、内存小；面板的「按下即出现」要求它 |
| 编程模型 | 即时模式：视图是状态的函数 `func(c *ui.Context)`，在主线程上按需重建 | 后台 goroutine 通过 `Window.Update` 回到主线程改状态 |
| 布局 | `ui.Column / Row`，flexbox 与 grid | |
| 文本 | `ui.Text`、`ui.RichText`；macOS 上由 Core Text 排字，仅对缺失字形使用系统补齐 | 中日韩文字、emoji 不需要自带字体 |
| 输入 | `ui.TextInput` / `TextInputBase`：多行、输入法组字、编辑菜单 | 原文栏和追问框 |
| Markdown | `github.com/egoist/mygo/ui/markdown` | 见下表 |
| 列表 | `ui.List`：虚拟化，`FollowEnd` 跟随末尾 | 对话流 |
| 其他组件 | `Segmented`（模式选择）、`Popover`、`MenuButton`（复制菜单）、`Toast`、`Tooltip`、动画与过渡 | |
| 自定义字体 | `ui.RegisterFont` | 注册通过 `go:embed` 内置的 CaskaydiaCove Nerd Font 常规、粗体、斜体和粗斜体；不依赖本机安装（需求 F1.8、Q1） |
| 可访问性 | MyGo 为自绘 UI 提供读屏支持 | |

`ui/markdown` 正好覆盖结果区的需求：

| 能力 | API |
| --- | --- |
| GFM：表格、任务列表、删除线、自动链接、脚注、公式 | `markdown.View(c, src, &opts)` |
| 流式：只增量解析新追加的部分，已完成的块不重排 | `markdown.Stream(c, src, &opts)`，完成后换成 `View` |
| 半截语法按最终样式显示；表头等分隔行到了再出现 | 内置 |
| 匀速出字与淡入：按词 / 字 / 句 / 行 / 段，追平时间、淡入时长可调；系统「减弱动态效果」时关闭 | `Options.Reveal{Unit, CatchUp, Fade}` |
| 代码块高亮、复制按钮、自定义代码块 | `Options.CodeBlock`、`Theme.CodeColors` |
| 选中复制时还原 Markdown 语法 | 内置（`copyMarkdown`） |
| 主题：字体、字号、行高、颜色、表格内边距 | `markdown.Theme` |

## 三、桌面能力：MyGo 提供

| 需求 | MyGo API |
| --- | --- |
| 不激活应用的浮动面板 | `WindowOptions{Panel: true, Frameless: true, Hidden: true}`：拿键盘但不激活应用，浮在全屏应用之上，跨桌面空间 |
| 无 Dock 图标 | `App.SetActivationPolicy(ActivationPolicyAccessory)`，打包时 `infoPlist` 写 `LSUIElement` |
| 全局快捷键 | `GlobalShortcut.Register`（macOS 上是 Carbon 热键）；自定义双击修饰键不是热键，走 §四 的 CGEventTap |
| 菜单栏图标与菜单 | `NewTray`、`NewMenu`，模板图标 |
| 鼠标所在屏幕、可见区域 | `Screen.CursorScreenPoint`、`Screen.DisplayNearestPoint(...).WorkArea` |
| 截图的框选窗口 | `Transparent`、`Frameless`、`AlwaysOnTop`、`SetVisibleOnAllWorkspaces` |
| 剪贴板读写 | `mygo.Clipboard`（多格式）；需要逐项备份与临时标记时走 purego（§四） |
| 配置文件位置 | `App.Path(PathUserData)` → `~/Library/Application Support/<Name>` |
| 开机启动、单实例 | `App.SetOpenAtLogin`、`App.RequestSingleInstanceLock` |
| 系统外观、强调色、减弱动态效果 | `Theme.IsDark`、可访问性设置 |
| 打包、签名 | `mygo.json` 的 `macos.signingIdentity`、`entitlements`、`infoPlist` |
| 配置文件热加载 | MyGo 的文件监听 |
| 附带辅助程序 | 「helper executables」：必要时随包附带并签名一个小工具（§八 退路） |
| 取得原生窗口 | `Window.NativeHandle()` → `NSWindow*`，用于 MyGo 没覆盖的窗口属性 |

## 四、系统集成：purego 直连系统框架

MyGo 内部有自己的 Objective-C 封装，但在 `internal/` 里，不能引用。我们直接用 MyGo 已依赖的 [purego](https://github.com/ebitengine/purego) **v0.11.1**：`purego.Dlopen` + `RegisterLibFunc` 调 C 函数，`purego/objc` 发 Objective-C 消息，`objc.NewBlock` 传回调 block。全部封装在 `internal/macos`，业务代码不直接碰 purego。

| 框架 | 用到的 API | 用途 |
| --- | --- | --- |
| ApplicationServices（HIServices） | `AXIsProcessTrusted(WithOptions)`、`AXUIElementCreateSystemWide`、`AXUIElementCopyAttributeValue`、`AXUIElementSetMessagingTimeout` | 读选区（`kAXSelectedText`、`kAXValue`、`kAXRole`） |
| CoreGraphics | `CGEventCreateKeyboardEvent`、`CGEventSetFlags`、`CGEventPost`、`CGEventTapCreate`（listen-only）、`CGEventTapEnable`、`CGPreflightListenEventAccess`、`CGRequestListenEventAccess`、`CGPreflightScreenCaptureAccess`、`CGRequestScreenCaptureAccess` | 模拟 ⌘C，监听自定义双击修饰键，输入监控与屏幕录制权限 |
| Carbon / HIToolbox | `TISCopyCurrentKeyboardLayoutInputSource`、`UCKeyTranslate`、`IsSecureEventInputEnabled` | 按当前键盘布局找 C 键，安全输入时不复制 |
| AppKit | `NSPasteboard`（`changeCount`、`pasteboardItems`、`types`、`dataForType:`、`writeObjects:`）、`NSWorkspace.frontmostApplication`、`NSRunningApplication` | 剪贴板逐项备份与恢复（`org.nspasteboard.TransientType`），前台应用 |
| Vision | `VNRecognizeTextRequest`（accurate，`zh-Hans` + `en-US`，自动判断语言）、`VNImageRequestHandler` | 截图识别文字 |
| ScreenCaptureKit | `SCScreenshotManager captureImageInRect:completionHandler:`（macOS 15.2+） | 冻结鼠标所在屏幕的截图 |
| CoreImage / CoreGraphics 图像 | `CGImage` ↔ Go `image.Image` | 截图裁剪、送入 Vision |

线程约定：辅助功能、Vision、截图都在后台 goroutine 上调用（`runtime.LockOSThread` + autorelease pool），带超时；只有窗口操作回到主线程。

## 五、模型接入

| 项 | 选择 | 说明 |
| --- | --- | --- |
| HTTP | 标准库 `net/http`，`context` 取消 | ⌘. 停止就是取消 context |
| 流式 | 自写 SSE 解析器（约 100 行） | 三种格式共用一个解析器，各自把事件映射成「文本 / 完成 / 失败」 |
| 格式 | Chat Completions、Responses、Anthropic Messages | 配置字段包括（`endpoint`、`format`、`model`、`auth`、`headers`、`body`），`body` 逐层合并进请求体 |
| SDK | **不用** | 三家 SDK 的依赖、体积、行为差异都不值得；只用到流式补全这一个接口 |
| 多轮 | 消息数组 | 第一轮是「模式提示词 + 合约 + 参数」的 system 和原文；追问依次追加（见 architecture.md §四） |
| 测试 | `net/http/httptest` 起本地 SSE 服务 | 模拟不均匀到达、断流、各类错误码 |

## 六、数据与配置

| 数据 | 存放 | 格式 |
| --- | --- | --- |
| 模型配置（不含 Key）、模式、快捷键、我的语言、外语、开机启动 | `~/Library/Application Support/Gloss/config.json` | JSON，带版本号与 `$schema`，原子写入（临时文件 + rename） |
| 配置说明 | 同目录 `config.schema.json` | JSON Schema，应用启动时写出 |
| API Key | 同 `config.json`，每个模型的 `apiKey` | 文件权限 0600 |
| 会话、原文、结果 | **只在内存** | 不落盘 |
| 日志 | `~/Library/Logs/Gloss/`，`log/slog` 文本格式 | 只记事件与耗时，不记文字 |

agent 直接编辑 `config.json`，应用用 MyGo 的文件监听发现变化后立即生效；命令行只负责检查（architecture.md §七）。

## 七、测试与质量

| 层 | 工具 | 覆盖 |
| --- | --- | --- |
| 纯逻辑 | `go test` | 提示词组装、SSE 解析、分段、错误分类、配置校验与热加载、自定义双击修饰键状态机 |
| 视图 | `ui.NewTester`：无窗口点击、输入、推进时钟 | 面板状态机、模式切换、对话流、键位 |
| 视觉快照 | `ui.Render` 输出 PNG，与基准图比较 | Markdown 表格、代码块、中英混排、深浅色 |
| 性能 | Go benchmark | 长回复每 token 的渲染开销、100 万字粘贴 |
| 系统集成 | `//go:build integration` 的测试，只做只读调用 | 辅助功能读取、私有剪贴板的备份恢复、Vision 识别生成的图片 |
| 端到端 | 手工清单；之后视需要引入 Tart 虚拟机跑 UI 流程 | ⌥空格唤起、自定义双击修饰键、带入选区、截图 |

测试纪律：测试不得在开发者的屏幕上发送合成的键盘鼠标事件、不得截取实时屏幕、不得写系统剪贴板、不得激活其他应用的窗口（详见 [AGENTS.md](../AGENTS.md)）。

## 八、签名、权限与发布

| 阶段 | 做法 |
| --- | --- |
| 自用（现在） | 本机创建一个自签名的代码签名证书（如「Gloss Dev」），`mygo.json` 的 `signingIdentity` 指向它。签名身份稳定，重新构建后辅助功能和屏幕录制授权不丢失；临时签名（`-`）每次构建都会丢 |
| CI | GitHub Actions：`go vet`、`go test`、`go tool mygo vet`、构建 |

## 九、风险与退路

| 风险 | 影响 | 应对 |
| --- | --- | --- |
| MyGo 很新（v0.5.x，单一维护者，UI API 已有过一次破坏性迁移） | 升级困难、遇到框架 bug 无人及时修 | 锁定精确版本；UI 代码与业务核心分离；升级单独开分支评估；必要时 vendor 并打补丁 |
| 自绘输入框的中文输入法、文本选择手感 | 原文栏是最高频的交互 | M1 第一件事就实测；不达标时用 `NativeHandle` 在面板上叠一个原生 `NSTextView`，或换 WebView 输入框 |
| purego 调 Objective-C 的 block、结构体返回值、`NSError**` 等细节 | 截图、Vision 实现成本高 | 先做 C 接口（辅助功能、CGEvent）；Vision / ScreenCaptureKit 不顺时，用 helper executable 附带一个几十行的 Swift 小工具，通过标准输入输出通信 |
| `ui/markdown` 在中文、宽表格、长代码块下的实际效果 | 结果区是核心体验 | M1 用视觉快照验证；可以通过 `Options.Block`、`CodeBlock` 自定义渲染 |
| 没有 Developer ID | 权限反复丢失 | 自签名证书（§八） |
| 自定义双击修饰键依赖 CGEventTap | 需要输入监控权限；回调慢会被系统停用；误触发 | listen-only 不拦截事件；被停用时立即重启；300ms 窗口内夹杂其他键即作废；没有权限时启动后请求一次，面板仍可从菜单栏打开 |
