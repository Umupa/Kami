// The site's motion. Kami's panel is redrawn in HTML and played by the
// page's scroll; the screenshot capture is acted out on a drawn screen;
// sections rise into view. The panel's parts, text and behavior follow
// internal/panel, the capture's internal/capture. The replies are written
// ahead of time, as the page calls no model.
(() => {
  "use strict";

  // On the page, Kami's default theme takes the page's white: Kami's panel
  // is frosted, and over white that is what it shows.
  const PAGE_WHITE = { background: "#FFFFFF", accentText: "#FFFFFF", surface: "#F6F5F1", raised: "#E6E4DD", border: "#E7E5DF" };
  const THEMES = (window.KAMI_THEMES || []).map((t) =>
    t.name === "flexoki-light" ? { ...t, colors: { ...t.colors, ...PAGE_WHITE } } : t,
  );
  const byName = new Map(THEMES.map((t) => [t.name, t]));
  const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches;
  const systemDark = matchMedia("(prefers-color-scheme: dark)");
  const narrow = matchMedia("(max-width: 860px)");

  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  const md = (...lines) => lines.join("\n");

  function el(tag, className, text) {
    const e = document.createElement(tag);
    if (className) e.className = className;
    if (text !== undefined) e.textContent = text;
    return e;
  }

  // The built-in modes, internal/mode.
  const BUILTIN = [
    { id: "translate", name: "翻译", desc: "外文译成我的语言，我的语言译成外语" },
    { id: "improve", name: "润色", desc: "改得通顺准确；内容杂乱时整理成标题、列表和表格" },
    { id: "ask", name: "问答", desc: "直接回答问题，先给结论" },
  ];

  // What /operator offers before a request, internal/panel/slash.go.
  const TOPICS = [
    ["theme", "切换配色主题"],
    ["自定义主题", "改颜色，或基于内置主题新建一套"],
    ["模型", "当前使用的模型"],
    ["快捷键", "查看或修改全局快捷键"],
    ["模式", "查看模式和完整提示词，增删模式"],
    ["权限", "哪些系统权限还没给"],
    ["语言", "我的语言和外语"],
    ["开机启动", "登录时自动打开"],
    ["配置模型", "交给 Coding Agent 配置模型服务"],
  ];

  const svg = (body, width = 1.4) =>
    `<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="${width}" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${body}</svg>`;
  const ICON = {
    pin: svg('<path d="M9.8 2.2l4 4-2.3.9-2.6 2.6.3 2.8-1.2 1.2-2.4-3.5-3.2 3.2M5.6 7.9L2.1 5.5l1.2-1.2 2.8.3 2.6-2.6z"/>'),
    operator: svg('<path d="M6.2 6.2l7.3 2.6-3.1 1.1-1.1 3.1z"/><path d="M3 1.5v2M1.5 3h2M6.5 2.2l-.8.8M2.2 6.5l.8-.8"/>', 1.3),
    plus: svg('<path d="M8 3.5v9M3.5 8h9"/>'),
    list: svg('<path d="M3 4.5h10M3 8h10M3 11.5h6"/>'),
    copy: svg('<rect x="5.5" y="5.5" width="8" height="8" rx="1.6"/><path d="M10.5 3.6V3.2A1.7 1.7 0 0 0 8.8 1.5H4.2A1.7 1.7 0 0 0 2.5 3.2v4.6a1.7 1.7 0 0 0 1.7 1.7h.4"/>'),
    check: svg('<path d="M3.5 8.5l3 3 6-7"/>', 1.6),
    more: svg('<path d="M4 6.5l4 4 4-4"/>'),
    retry: svg('<path d="M2.5 8a5.5 5.5 0 1 0 1.6-3.9"/><path d="M2.5 2.5v3h3"/>'),
    undo: svg('<path d="M5.5 3.5L2.5 6.5l3 3"/><path d="M2.5 6.5h7a4 4 0 0 1 0 8H7"/>'),
  };

  // inline appends text with **strong** and `code` to parent.
  function inline(text, parent) {
    const re = /(\*\*[^*]+\*\*|`[^`]+`)/g;
    let at = 0;
    for (const m of text.matchAll(re)) {
      if (m.index > at) parent.append(text.slice(at, m.index));
      const tok = m[0];
      parent.append(tok[0] === "*" ? el("strong", "", tok.slice(2, -2)) : el("code", "", tok.slice(1, -1)));
      at = m.index + tok.length;
    }
    if (at < text.length) parent.append(text.slice(at));
  }

  const GO_KEYWORDS = new Set(
    "break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var".split(" "),
  );

  // highlight colors code as the panel's syntax theme does: keywords,
  // strings, numbers, comments, called functions; in a shell, commands.
  function highlight(lang, code, parent) {
    const shell = lang === "bash" || lang === "sh";
    const comment = shell ? "#" : "//";
    code.split("\n").forEach((line, n) => {
      if (n > 0) parent.append("\n");
      if (line.trimStart().startsWith(comment)) {
        parent.append(el("span", "tc", line));
        return;
      }
      let rest = line;
      let first = true;
      while (rest) {
        let m;
        let cls = "";
        if (!shell && rest.startsWith("//")) {
          parent.append(el("span", "tc", rest));
          break;
        } else if ((m = /^\s+/.exec(rest))) {
        } else if ((m = /^"(?:[^"\\]|\\.)*"/.exec(rest))) {
          cls = "ts";
        } else if ((m = /^\d+(\.\d+)?\b/.exec(rest))) {
          cls = "tn";
        } else if ((m = /^[A-Za-z_]\w*/.exec(rest))) {
          const after = rest.slice(m[0].length);
          if (!shell && GO_KEYWORDS.has(m[0])) cls = "tk";
          else if ((shell && first) || (!shell && after.startsWith("("))) cls = "tf";
          first = false;
        } else {
          m = [rest[0]];
          first = false;
        }
        parent.append(cls ? el("span", cls, m[0]) : m[0]);
        rest = rest.slice(m[0].length);
      }
    });
  }

  function codeBlock(lang, code) {
    const box = el("div", "k-code");
    const head = el("div", "k-code-head");
    const copy = el("span");
    copy.innerHTML = `${ICON.copy}<span>复制</span>`;
    head.append(el("span", "", lang || "text"), copy);
    const pre = el("pre");
    const c = el("code");
    highlight(lang, code, c);
    pre.append(c);
    box.append(head, pre);
    return box;
  }

  function table(rows) {
    const t = el("table");
    const cells = (row) =>
      row
        .replace(/^\||\|$/g, "")
        .split("|")
        .map((s) => s.trim());
    const head = el("tr");
    for (const cell of cells(rows[0])) inline(cell, head.appendChild(el("th")));
    t.append(head);
    for (const row of rows.slice(2)) {
      const tr = el("tr");
      for (const cell of cells(row)) inline(cell, tr.appendChild(el("td")));
      t.append(tr);
    }
    return t;
  }

  // markdown renders the subset of GFM the replies use.
  function markdown(src) {
    const root = el("div", "md");
    const lines = src.replace(/\n+$/, "").split("\n");
    let i = 0;
    while (i < lines.length) {
      const line = lines[i];
      let m;
      if (!line.trim()) {
        i++;
      } else if ((m = /^```(\w*)/.exec(line))) {
        const body = [];
        i++;
        while (i < lines.length && !lines[i].startsWith("```")) body.push(lines[i++]);
        i++;
        root.append(codeBlock(m[1], body.join("\n").replace(/\t/g, "    ")));
      } else if ((m = /^(#{1,4})\s+(.*)$/.exec(line))) {
        const h = el("h" + m[1].length);
        inline(m[2], h);
        root.append(h);
        i++;
      } else if (/^-{3,}\s*$/.test(line)) {
        root.append(el("div", "hr", "​"));
        i++;
      } else if (line.startsWith("|")) {
        const rows = [];
        while (i < lines.length && lines[i].startsWith("|")) rows.push(lines[i++]);
        root.append(table(rows));
      } else if (/^([-*]|\d+\.)\s+/.test(line)) {
        const ordered = /^\d/.test(line);
        const list = el(ordered ? "ol" : "ul");
        while (i < lines.length && /^([-*]|\d+\.)\s+/.test(lines[i])) {
          inline(lines[i].replace(/^([-*]|\d+\.)\s+/, ""), list.appendChild(el("li")));
          i++;
        }
        root.append(list);
      } else if (line.startsWith(">")) {
        const quote = el("blockquote");
        const text = [];
        while (i < lines.length && lines[i].startsWith(">")) text.push(lines[i++].replace(/^>\s?/, ""));
        inline(text.join(" "), quote);
        root.append(quote);
      } else {
        const text = [];
        while (i < lines.length && lines[i].trim() && !/^(```|#{1,4}\s|\||>|([-*]|\d+\.)\s|-{3,}\s*$)/.test(lines[i])) {
          text.push(lines[i++]);
        }
        const p = el("p");
        inline(text.join(" "), p);
        root.append(p);
      }
    }
    return root;
  }

  // unitize wraps the text of root into the units it streams by: a word of
  // Latin text, a character of Chinese, as ui/markdown reveals them.
  function unitize(root) {
    const units = [];
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    const nodes = [];
    while (walker.nextNode()) nodes.push(walker.currentNode);
    const split = /[⺀-鿿豈-﫿＀-￯　-〿]|[^\s⺀-鿿豈-﫿＀-￯　-〿]+\s*|\s+/g;
    for (const node of nodes) {
      if (node.parentElement.closest(".k-code-head")) continue;
      const frag = document.createDocumentFragment();
      for (const part of node.data.match(split) || []) {
        const u = el("span", "u", part);
        frag.append(u);
        units.push(u);
      }
      node.replaceWith(frag);
    }
    return units;
  }

  const BLOCKS = "p, h1, h2, h3, h4, li, tr, table, ul, ol, blockquote, .k-code, .hr";

  // ---------- The examples ----------

  // Each holds a reply for every built-in mode, so that switching modes and
  // running shows what Kami would.
  const TERMINAL = {
    source: md(
      "⏺ The follow-up bar now stays inside the window. I changed",
      "  internal/panel/view.go so the result gives way first, then",
      "  ran scripts/check.sh: gofmt, go vet for arm64 and amd64, and",
      "  go test all pass.",
    ),
    replies: {
      translate:
        "追问栏现在始终留在窗口内。我修改了 internal/panel/view.go，让结果区先让位，然后运行了 scripts/check.sh：gofmt、arm64 与 amd64 的 go vet，以及 go test 全部通过。",
      improve:
        "The follow-up bar now stays inside the window. I changed internal/panel/view.go so that the result gives way first, then ran scripts/check.sh: gofmt, go vet for arm64 and amd64, and go test all pass.",
      ask: "Sounds done: the change is in and every check passes. To be sure, fill the panel with a long reply and confirm the follow-up bar stays in view.",
    },
    follow: {
      ask: "再短一点",
      reply: "追问栏已固定在窗口内；改了 internal/panel/view.go，scripts/check.sh 全部通过。",
    },
  };

  const CHAT = {
    source: md(
      "小王 10:02 官网周五上线来得及吗",
      "小李 10:03 文案周三给你 首屏那句还在改",
      "阿杰 10:05 演示面板我来 手机上要能看 减少动态效果也要支持",
      "小王 10:06 好 下载就用 releases/latest 周四一起过一遍",
    ),
    replies: {
      improve: md(
        "## 官网上线安排",
        "",
        "目标周五上线，周四一起过一遍。",
        "",
        "| 负责人 | 事项 | 时间 |",
        "| --- | --- | --- |",
        "| 小李 | 文案（首屏那句还在改） | 周三 |",
        "| 阿杰 | 演示面板：手机上能看，支持减少动态效果 | — |",
        "| 小王 | 下载链接用 `releases/latest` | — |",
      ),
      translate: md(
        "Xiao Wang 10:02 Can we launch the website by Friday?",
        "",
        "Xiao Li 10:03 I'll send you the copy on Wednesday; the hero line is still being reworked.",
        "",
        "A Jie 10:05 I'll take the demo panel. It has to work on phones and support reduced motion.",
        "",
        "Xiao Wang 10:06 OK. Use releases/latest for the download, and let's go over it together on Thursday.",
      ),
      ask: "**来得及，前提是周三拿到文案。** 演示面板还没有约定时间，最好在周四一起过之前完成。",
    },
  };

  const QUESTION = {
    source: "Go 里多个 defer 的执行顺序是什么？",
    replies: {
      ask: md(
        "**后进先出。** 同一个函数里的多个 `defer` 按注册的相反顺序执行，在函数返回前依次调用。",
        "",
        "```go",
        "func main() {",
        '\tdefer fmt.Println("1")',
        '\tdefer fmt.Println("2")',
        '\tdefer fmt.Println("3")',
        "}",
        "// 输出：3 2 1",
        "```",
        "",
        "注意：`defer` 的参数在注册时就已求值。",
      ),
      translate: "In Go, what order do multiple defers run in?",
      improve: "Go 中多个 defer 的执行顺序是怎样的？",
    },
  };

  const COMMIT = {
    id: "commit",
    name: "提交信息",
    desc: "把改动写成一条 commit message：第一行是简短的摘要，空一行后说明为什么要改。",
  };

  const ORANGE = { id: "flexoki-light-custom", base: "flexoki-light", colors: { accent: "#BC5215" } };

  // What the screenshot scenes show, recognize and translate.
  const SCENES = [
    {
      id: "dialog",
      target: [".alert-title", ".alert-body"],
      source: md("Your startup disk is almost full.", "To make more space available, delete files you no longer need."),
      reply: md("启动磁盘快满了。", "", "要腾出更多空间，请删除不再需要的文件。"),
    },
    {
      id: "video",
      target: [".subtitle"],
      source: "The best interface is the one you never have to look for.",
      reply: "最好的界面，是你从来不用去找的那一个。",
    },
    {
      id: "image",
      target: [".slide .caption"],
      source: "Snapshots never pause writers. Recovery replays the log from the last snapshot.",
      reply: "快照从不暂停写入。恢复时，从最近一次快照开始重放日志。",
    },
  ];

  // ---------- Runs ----------

  // A run is one play of a scene. Cancelled, its script stops at its next
  // step; a reply already streaming goes on, as a request in Kami does.
  const CANCELLED = Symbol("cancelled");

  function newRun() {
    let reject;
    const stopped = new Promise((_, r) => (reject = r));
    stopped.catch(() => {});
    const run = {
      cancelled: false,
      stopped,
      cancel() {
        if (run.cancelled) return;
        run.cancelled = true;
        reject(CANCELLED);
      },
    };
    return run;
  }

  const quiet = (e) => {
    if (e !== CANCELLED) console.error(e);
  };

  // instantRun is set while a scene is shown at its end at once.
  let instantRun = false;
  const still = () => reduced || instantRun;

  // ---------- The panel ----------

  // createPanel draws a Kami panel in root. opts.keys shows the keys a
  // script presses, opts.shortcut the global shortcut it uses; opts.visible
  // tells whether the panel is on screen, for scripts to wait while it is
  // not; opts.takeOver hears the visitor take the panel.
  function createPanel(root, opts = {}) {
    const interactive = opts.interactive !== false;
    const S = {
      theme: "flexoki-light",
      preview: "",
      custom: new Map(),
      modes: BUILTIN.slice(),
      idx: 0,
      source: "",
      follow: "",
      followFocus: false,
      results: new Map(), // mode id → session
      op: null, // the operator session
      opSince: 0,
      menuSel: 0,
      menuMoved: false,
      themeReturn: "",
      note: "",
      pinned: false,
      least: 0,
      followEnd: true,
      fresh: "",
      freshAt: 0,
      example: null,
      typing: false,
      // hold keeps the height of the scene before through a change of
      // scene, until the new one has its reply.
      hold: 0,
    };

    root.innerHTML = `
      <div class="k-inner">
      <div class="k-source"></div>
      <div class="k-note" hidden></div>
      <div class="k-menu" hidden></div>
      <div class="k-fill"></div>
      <div class="k-rule"></div>
      <div class="k-bar"></div>
      <div class="k-rule k-rule-result" hidden></div>
      <div class="k-result-wrap" hidden><div class="k-result"></div></div>
      <div class="k-fill k-fill-result" hidden></div>
      <div class="k-rule k-rule-follow" hidden></div>
      <div class="k-follow" hidden><span class="k-prompt">❯</span><span class="k-follow-text"></span><span class="k-kbd"><span>⌘J</span><span>切换</span></span></div>
      </div>
      <div class="k-sparkles"></div>`;

    const $ = (sel) => root.querySelector(sel);
    const inner = $(".k-inner");

    // The frame follows the content's height; once drawn, it glides there.
    root.classList.add("fluid");
    const frame = () => {
      root.style.height = inner.offsetHeight + (root.offsetHeight - root.clientHeight) + "px";
    };
    new ResizeObserver(frame).observe(inner);
    requestAnimationFrame(() => requestAnimationFrame(() => root.classList.add("gliding")));

    // fadeOut and fadeIn cross a change of scene; unfade puts the content
    // back at once.
    let fade = null;
    function fadeOut() {
      fade = inner.animate([{ opacity: 0, transform: "translateY(-4px)" }], { duration: 170, easing: "ease-in", fill: "forwards" });
      return fade.finished.catch(() => {});
    }
    function fadeIn() {
      const prev = fade;
      fade = inner.animate([{ opacity: 0, transform: "translateY(6px)" }, { opacity: 1, transform: "none" }], {
        duration: 340,
        easing: "cubic-bezier(0.22, 1, 0.36, 1)",
      });
      if (prev) prev.cancel();
    }
    function unfade() {
      if (fade) fade.cancel();
      fade = null;
    }

    const E = {
      source: $(".k-source"),
      note: $(".k-note"),
      menu: $(".k-menu"),
      fill: $(".k-fill"),
      bar: $(".k-bar"),
      ruleResult: $(".k-rule-result"),
      resultWrap: $(".k-result-wrap"),
      result: $(".k-result"),
      fillResult: $(".k-fill-result"),
      ruleFollow: $(".k-rule-follow"),
      follow: $(".k-follow"),
      followText: $(".k-follow-text"),
      sparkles: $(".k-sparkles"),
      run: null,
    };

    const isNotes = () => S.idx === S.modes.length;
    const currentId = () => (isNotes() ? "note" : S.modes[S.idx].id);
    const indexFor = (id) => {
      if (id === "note") return S.modes.length;
      return Math.max(0, S.modes.findIndex((m) => m.id === id));
    };

    // leading returns the request after /operator, or null when the source
    // does not start with it (operator.Leading).
    function leading(text) {
      const m = /^\s*\/operator(\s|$)/.exec(text);
      return m ? text.slice(m[0].length).trim() : null;
    }

    const operatorActive = () => !isNotes() && leading(S.source) !== null;
    const themeTyped = (src) => src.startsWith("/theme") && (src.length === 6 || src[6] === " ");

    // shown is the session the result shows: the operator's while the
    // source asks for it, else the current mode's own.
    function shown() {
      if (isNotes()) return null;
      if (S.op && operatorActive()) return S.op;
      return S.results.get(S.modes[S.idx].id) || null;
    }

    // translateTarget names the language the source goes to, as the panel
    // labels the translate tab: Chinese text goes to the foreign language.
    function translateTarget() {
      if (!S.source.trim()) return "";
      let han = 0;
      let letters = 0;
      for (const ch of S.source) {
        if (/\p{Script=Han}/u.test(ch)) han++;
        else if (/\p{L}/u.test(ch)) letters++;
      }
      return han * 2 >= letters ? "English" : "简体中文";
    }

    // ---------- Theme ----------

    const VARS = {
      bg: "background",
      surface: "surface",
      raised: "raised",
      border: "border",
      edge: "edge",
      text: "text",
      bright: "bright",
      muted: "muted",
      faint: "faint",
      accent: "accent",
      "accent-text": "accentText",
      link: "link",
      code: "code",
      quote: "quote",
      operator: "operator",
      "operator-fill": "operatorFill",
      sparkle: "sparkle",
      danger: "danger",
      kw: "keyword",
      str: "string",
      num: "number",
      com: "comment",
      fn: "function",
      ty: "type",
    };
    let applied = null;

    function paletteOf(name) {
      if (S.custom.has(name)) return S.custom.get(name);
      if (name === "auto" || !byName.has(name)) return byName.get(systemDark.matches ? "flexoki-night" : "flexoki-light");
      return byName.get(name);
    }

    function applyTheme() {
      const p = paletteOf(S.preview || S.theme);
      if (p === applied) return;
      applied = p;
      for (const [v, key] of Object.entries(VARS)) root.style.setProperty("--" + v, p.colors[key]);
      root.style.setProperty("--glass", p.colors.background);
    }

    function setTheme(name) {
      S.theme = name;
      S.preview = "";
    }

    // defineTheme adds a custom theme: a built-in base and the colors it
    // changes; the frame follows a new accent, as design.Derive does.
    function defineTheme(t) {
      const base = byName.get(t.base);
      const colors = { ...base.colors, ...t.colors };
      if (t.colors.accent && !t.colors.edge) colors.edge = t.colors.accent + "59";
      S.custom.set(t.id, { name: t.id, dark: base.dark, base: t.base, colors });
    }

    // ---------- Rendering ----------

    function sheen(node) {
      const since = performance.now() - S.opSince;
      if (still() || !S.opSince || since >= 900) return;
      node.classList.add("sheen");
      node.style.animationDelay = `-${since}ms`;
    }

    const caret = () => el("span", "k-caret");

    function renderSource() {
      const box = E.source;
      box.replaceChildren();
      const focused = !S.followFocus && (S.typing || document.activeElement === root);
      if (!S.source) {
        if (focused) box.append(caret());
        box.append(el("span", "k-placeholder", "输入或粘贴，/ 查看命令"));
        return;
      }
      const m = /^(\s*)(\/operator)(?=\s|$)/.exec(S.source);
      if (m) {
        box.append(m[1]);
        const cmd = el("span", "k-cmd", m[2]);
        sheen(cmd);
        box.append(cmd, S.source.slice(m[0].length));
      } else {
        box.append(S.source);
      }
      if (focused) box.append(caret());
    }

    // renderNote colors the note's Markdown without changing it, as
    // note.Highlight does: headings, list marks, code, links.
    function renderNote() {
      const box = E.note;
      box.replaceChildren();
      if (!S.note) {
        box.append(caret(), el("span", "k-placeholder", "随手记点什么…  ⌘N 新建  ⌘P 全部笔记"));
        return;
      }
      const lines = S.note.split("\n");
      lines.forEach((line, i) => {
        let m;
        if ((m = /^(#{1,6})(\s.*)$/.exec(line))) {
          const h = el("span", "nh");
          h.append(el("span", "nm", m[1]), m[2]);
          box.append(h);
        } else if ((m = /^(\s*)([-*+]|\d+[.)])(\s)(.*)$/.exec(line))) {
          box.append(m[1], el("span", "nm", m[2]), m[3]);
          noteInline(m[4], box);
        } else {
          noteInline(line, box);
        }
        if (i < lines.length - 1) box.append("\n");
      });
      box.append(caret());
    }

    function noteInline(text, box) {
      let at = 0;
      for (const m of text.matchAll(/https?:\/\/\S+|`[^`]+`/g)) {
        box.append(text.slice(at, m.index));
        box.append(el("span", m[0][0] === "`" ? "nc" : "nl", m[0]));
        at = m.index + m[0].length;
      }
      box.append(text.slice(at));
    }

    // filter keeps the items whose key or title has query, those starting
    // with it first (slash.go).
    function filter(all, query) {
      const q = query.toLowerCase();
      const first = [];
      const rest = [];
      for (const it of all) {
        const key = it.key.toLowerCase();
        if (key.startsWith(q) || it.title.startsWith(query)) first.push(it);
        else if (key.includes(q) || it.title.includes(query)) rest.push(it);
      }
      return first.concat(rest);
    }

    function menuFor() {
      const src = S.source;
      if (isNotes() || src.includes("\n")) return null;
      const request = leading(src);
      let kind;
      let items;
      if (themeTyped(src)) {
        kind = "themes";
        const all = [{ key: "auto", title: "跟随系统" }];
        for (const t of S.custom.values()) all.push({ key: t.name, title: "基于 " + t.base, swatch: t.colors });
        for (const t of THEMES) all.push({ key: t.name, title: t.dark ? "深色" : "浅色", swatch: t.colors });
        items = filter(all, src.slice(6).trim());
      } else if (request !== null) {
        if (/[ \t]/.test(request)) return null;
        kind = "topics";
        items = filter(
          TOPICS.map(([key, title]) => ({ key, title })),
          request,
        );
      } else if (src.startsWith("/") && !/[ \t]/.test(src)) {
        kind = "commands";
        const all = [
          { key: "/operator", title: "Operator", desc: "用一句话查看或修改 Kami 的设置", op: true },
          { key: "/theme", title: "主题", desc: "" },
          ...S.modes.map((m) => ({ key: "/" + m.id, title: m.name, desc: m.desc })),
          { key: "/note", title: "笔记", desc: "" },
        ];
        items = filter(all, src);
      } else {
        return null;
      }
      return items.length ? { kind, items } : null;
    }

    let menu = null;

    function renderMenu() {
      menu = menuFor();
      E.menu.hidden = !menu;
      if (!menu) {
        S.preview = "";
        return;
      }
      S.menuSel = Math.min(S.menuSel, menu.items.length - 1);
      E.menu.className = "k-menu " + menu.kind;
      E.menu.replaceChildren();
      menu.items.forEach((it, i) => {
        const row = el("div", "k-row" + (i === S.menuSel ? " on" : ""));
        row.append(el("b", it.op ? "op" : "", it.key));
        if (menu.kind === "themes") {
          const sw = el("span", "k-swatch");
          if (it.swatch) {
            for (const k of ["background", "accent", "code", "quote", "sparkle"]) sw.appendChild(el("i")).style.background = it.swatch[k];
          }
          row.append(sw, el("span", "", it.title));
          if (interactive) {
            row.addEventListener("pointerenter", () => {
              takeOver();
              choosePreview(i);
            });
            row.addEventListener("click", () => {
              takeOver();
              takeTheme(it.key);
            });
          }
        } else {
          row.append(el("span", "", it.title));
          if (menu.kind === "commands") row.append(el("em", "", it.desc));
        }
        E.menu.append(row);
      });
      if (menu.kind === "themes" && S.menuMoved) S.preview = menu.items[S.menuSel].key;
    }

    // choosePreview marks a theme row and shows the panel in its colors,
    // in place: rebuilding the rows would take the one under the pointer.
    function choosePreview(i) {
      S.menuSel = i;
      S.menuMoved = true;
      [...E.menu.children].forEach((row, j) => row.classList.toggle("on", j === i));
      S.preview = menu.items[i].key;
      applyTheme();
      const row = E.menu.children[i];
      if (row) {
        const top = row.offsetTop - E.menu.offsetTop;
        if (top < E.menu.scrollTop) E.menu.scrollTop = top - 6;
        else if (top + row.offsetHeight > E.menu.scrollTop + E.menu.clientHeight) {
          E.menu.scrollTop = top + row.offsetHeight - E.menu.clientHeight + 6;
        }
      }
    }

    function openThemes() {
      if (!themeTyped(S.source)) S.themeReturn = S.source;
      S.source = "/theme ";
      S.menuSel = 0;
      S.menuMoved = false;
    }

    // closeThemes leaves the theme list. Kami clears the source here; the
    // demo puts the example's text back, as a visitor cannot type it again.
    function closeThemes() {
      S.source = S.themeReturn;
      S.themeReturn = "";
      S.preview = "";
      S.menuSel = 0;
      S.menuMoved = false;
    }

    function takeTheme(name) {
      setTheme(name);
      closeThemes();
      render();
    }

    function button(className, html, label, onClick) {
      const b = el(interactive ? "button" : "span", className);
      if (interactive) b.type = "button";
      b.innerHTML = html;
      if (label) {
        b.title = label;
        b.setAttribute("aria-label", label);
      }
      if (interactive) {
        b.addEventListener("click", (e) => {
          e.stopPropagation();
          takeOver();
          onClick();
          root.focus({ preventScroll: true });
        });
      }
      return b;
    }

    function renderBar() {
      const bar = E.bar;
      bar.replaceChildren();
      const pin = button("k-icon k-pin" + (S.pinned ? " on" : ""), ICON.pin, S.pinned ? "取消固定 ⌘⇧P" : "固定面板 ⌘⇧P", () => {
        S.pinned = !S.pinned;
        render();
      });
      if (operatorActive()) {
        const pill = el("span", "k-pill");
        pill.innerHTML = ICON.operator;
        const name = el("span", "k-pill-name", "Operator");
        sheen(name);
        const x = button("", "×", "退出 Operator", () => {
          S.source = leading(S.source) || "";
          S.opSince = 0;
          render();
        });
        pill.append(name, x);
        bar.append(pill, el("span", "k-spacer"), pin, runButton(true));
        return;
      }
      const modes = el("div", "k-modes");
      S.modes.forEach((m, i) => {
        let name = m.name;
        if (m.id === "translate" && i === S.idx) {
          const to = translateTarget();
          if (to) name += " → " + to;
        }
        const chip = button("k-chip" + (i === S.idx ? " on" : ""), "", "", () => actions.setMode(i));
        chip.textContent = name;
        const since = performance.now() - S.freshAt;
        if (m.id === S.fresh && S.freshAt && since < 1600 && !still()) {
          chip.classList.add("fresh");
          chip.style.animationDelay = `-${since}ms`;
        }
        modes.append(chip);
      });
      const notes = button("k-chip notes" + (isNotes() ? " on" : ""), "", "", () => actions.setMode(S.modes.length));
      notes.textContent = "笔记";
      modes.append(notes);
      bar.append(modes, el("span", "k-spacer"));
      if (isNotes()) {
        const all = el("span", "k-icon");
        all.innerHTML = ICON.list;
        all.title = "全部笔记 ⌘P";
        bar.append(
          button("k-icon", ICON.plus, "新建 ⌘N", () => {
            S.note = "";
            render();
          }),
          all,
          pin,
        );
      } else {
        const theme = button("k-theme", "", "切换主题", () => {
          openThemes();
          render();
        });
        theme.append(el("span", "k-theme-mark", "◐"), el("span", "k-theme-name", S.theme));
        const kbd = el("span", "k-kbd");
        kbd.append(el("span", "", "tab"), el("span", "", "模式"));
        bar.append(pin, theme, kbd, runButton(false));
      }
      // Keep the current tab in view where the bar scrolls.
      const on = modes.querySelector(".on");
      if (on) {
        const left = on.offsetLeft - modes.offsetLeft;
        if (left < modes.scrollLeft || left + on.offsetWidth > modes.scrollLeft + modes.clientWidth) {
          modes.scrollLeft = left - 8;
        }
      }
    }

    function runButton(op) {
      const b = button("k-run" + (op ? " op" : ""), "<span>⏎</span><span>执行</span>", "", () => actions.run());
      E.run = b;
      return b;
    }

    function renderFollow() {
      E.follow.classList.toggle("focused", S.followFocus);
      const t = E.followText;
      t.replaceChildren();
      if (S.follow) t.append(S.follow);
      if (S.followFocus) t.append(caret());
      if (!S.follow) t.append(el("span", "k-placeholder", "追问…"));
    }

    function render() {
      const notes = isNotes();
      const sess = shown();
      const bar = !(sess && sess.operator && operatorActive());
      E.source.hidden = notes;
      E.note.hidden = !notes;
      if (notes) renderNote();
      else renderSource();
      renderMenu();
      E.fill.hidden = !!sess && !notes;
      E.bar.hidden = !bar;
      renderBar();
      const result = !!sess && !notes;
      E.ruleResult.hidden = !(result && bar);
      E.resultWrap.hidden = E.fillResult.hidden = E.ruleFollow.hidden = E.follow.hidden = !result;
      if (result && E.result.firstChild !== sess.el) E.result.replaceChildren(sess.el);
      E.source.classList.toggle("held", result);
      renderFollow();
      applyTheme();
      grow();
    }

    // grow keeps the panel at the tallest its content has been in this
    // session, as Kami's does: the bar or the follow-up line stays at the
    // bottom. Before a session, and in a note, it follows its content.
    function grow() {
      const cap = parseFloat(getComputedStyle(inner).maxHeight) || Infinity;
      if (isNotes() || (!S.results.size && !S.op)) {
        inner.style.minHeight = S.hold && !isNotes() ? Math.min(S.hold, cap) + "px" : "";
        if (!isNotes()) S.least = 0;
        return;
      }
      // The session keeps the height of its own content, measured without
      // the hold; an open menu passes.
      inner.style.minHeight = S.least ? Math.min(S.least, cap) + "px" : "";
      const h = inner.offsetHeight;
      if (h > S.least && E.menu.hidden) S.least = h;
      const floor = Math.max(S.least, S.hold);
      inner.style.minHeight = floor ? Math.min(floor, cap) + "px" : "";
    }

    function followEnd() {
      const r = E.result;
      if (S.followEnd && r.scrollHeight > r.clientHeight) r.scrollTop = r.scrollHeight;
    }

    E.result.addEventListener("scroll", () => {
      const r = E.result;
      S.followEnd = r.scrollTop + r.clientHeight >= r.scrollHeight - 4;
    });

    // ---------- Sessions and streaming ----------

    function newSession(mode, ask, operator = false) {
      return { mode, ask, operator, turns: [], el: el("div", "k-turns"), undoable: false, cancelled: false };
    }

    function thinking() {
      const row = el("div", "k-thinking");
      row.append(el("i", "", "●"), el("span", "", "思考中"));
      return row;
    }

    function addTurn(sess, ask) {
      const turn = { ask, reply: "", status: "thinking", el: el("div", "k-turn"), body: el("div", "k-body"), foot: el("div", "k-foot") };
      if (ask) turn.el.append(el("div", "k-ask", ask));
      turn.el.append(turn.body, turn.foot);
      sess.turns.push(turn);
      sess.el.append(turn.el);
      turn.body.replaceChildren(thinking());
      renderFoot(sess);
      return turn;
    }

    function renderFoot(sess) {
      sess.turns.forEach((turn, i) => {
        const last = i === sess.turns.length - 1;
        turn.foot.replaceChildren();
        if (turn.status !== "done") return;
        if (last && sess.undoable) turn.foot.append(button("k-icon undo", ICON.undo, "撤销这次修改 ⌘Z", () => actions.undo()));
        // An operator reply would change the settings again; the demo
        // does not ask twice.
        if (last && !sess.operator) turn.foot.append(button("k-icon", ICON.retry, "重新生成", () => actions.retry(sess)));
        const more = el("span", "k-icon");
        more.innerHTML = ICON.more;
        more.title = "复制为…";
        const copy = button("k-icon", ICON.copy, "复制 Markdown", () => {
          if (navigator.clipboard) navigator.clipboard.writeText(turn.reply).catch(() => {});
          copy.innerHTML = ICON.check;
          copy.title = "已复制";
          setTimeout(() => {
            copy.innerHTML = ICON.copy;
            copy.title = "复制 Markdown";
          }, 1500);
        });
        turn.foot.append(more, copy);
      });
    }

    // stream writes a reply into a turn: a moment of thinking, then words
    // arriving at an even pace, each fading in; a block shows from its
    // first word in its final style.
    async function stream(sess, turn, text) {
      const instant = still();
      turn.reply = text;
      turn.status = "thinking";
      if (!instant) await sleep(560 + Math.random() * 260);
      if (sess.cancelled) return;
      const tree = markdown(text);
      const units = unitize(tree);
      turn.status = "streaming";
      turn.body.replaceChildren(tree);
      if (!instant) {
        for (const b of tree.querySelectorAll(BLOCKS)) b.classList.add("pend");
        await reveal(tree, units, () => sess.cancelled, () => {
          grow();
          followEnd();
        });
      } else {
        for (const u of units) u.classList.add("done");
      }
      if (!sess.cancelled) finish(sess, turn);
    }

    function finish(sess, turn) {
      turn.status = "done";
      S.hold = 0;
      // Once faded in, the words stay put when the result is shown again.
      setTimeout(() => {
        for (const u of turn.body.querySelectorAll(".u.in")) u.className = "u done";
      }, 400);
      renderFoot(sess);
      grow();
      followEnd();
    }

    // appendReply adds what the app writes after a reply, at once.
    function appendReply(sess, turn, text) {
      turn.reply += text;
      const extra = markdown(text);
      for (const u of unitize(extra)) u.classList.add("done");
      turn.body.querySelector(".md").append(...extra.childNodes);
      renderFoot(sess);
      grow();
      followEnd();
    }

    function shownValue(v) {
      if (v === "") return "（无）";
      const runes = Array.from(v);
      const clipped = runes.length > 80 ? runes.slice(0, 80).join("") + "…" : v;
      return "`" + clipped.replace(/`/g, "'") + "`";
    }

    // sparkle sends the burst of stars up the result as an operator
    // request goes out (drawSparkles in internal/panel/operator.go).
    function sparkle() {
      if (still()) return;
      const layer = E.sparkles;
      const top = E.resultWrap.offsetTop;
      const h = Math.max(140, E.follow.offsetTop - top);
      const w = root.clientWidth;
      layer.style.top = top + "px";
      layer.style.height = h + "px";
      layer.replaceChildren();
      const n = 18;
      for (let i = 0; i < n; i++) {
        const star = i % 3 !== 2;
        const size = star ? 7 + Math.random() * 6 : 2.5 + Math.random() * 2;
        const x = Math.min(0.96, Math.max(0.04, (i + Math.random()) / n)) * w;
        const delay = (0.15 + Math.random() * 0.95) * 1000;
        const dur = (1.2 + Math.random() * 0.8) * 1000;
        const drift = (Math.random() - 0.5) * 28;
        const s = document.createElementNS("http://www.w3.org/2000/svg", "svg");
        s.setAttribute("viewBox", "-1 -1 2 2");
        s.setAttribute("width", size);
        s.setAttribute("height", size);
        s.innerHTML = star ? '<path fill="currentColor" d="M0 -1Q0 0 1 0Q0 0 0 1Q0 0 -1 0Q0 0 0 -1Z"/>' : '<circle fill="currentColor" r="1"/>';
        layer.append(s);
        const frames = [0, 0.18, 0.45, 0.7, 1].map((t) => {
          const rise = 1 - (1 - t) * (1 - t);
          const y = h + size - rise * (h + 2 * size);
          const scale = star ? 0.4 + 0.6 * Math.min(1, t / 0.18) : 1;
          return {
            offset: t,
            opacity: Math.min(1, t / 0.18) * Math.min(1, (1 - t) / 0.3),
            transform: `translate(${x + drift * t - size / 2}px, ${y - size / 2}px) scale(${scale})`,
          };
        });
        s.animate(frames, { duration: dur, delay, fill: "both" });
      }
      setTimeout(() => layer.replaceChildren(), 3200);
    }

    // ---------- Actions: what a key or a click does ----------

    const actions = {
      setMode(i) {
        S.idx = i;
        S.followFocus = false;
        render();
      },

      cycle(step) {
        const n = S.modes.length + 1;
        actions.setMode((S.idx + step + n) % n);
      },

      // run sends the source in the current mode; the reply is the example's.
      run() {
        if (operatorActive() || isNotes() || !S.source.trim() || !S.example) return;
        const mode = S.modes[S.idx];
        const reply = S.example.replies[mode.id];
        if (!reply) return;
        const old = S.results.get(mode.id);
        if (old) old.cancelled = true;
        const sess = newSession(mode.id, S.source);
        S.results.set(mode.id, sess);
        const turn = addTurn(sess, null);
        S.followEnd = true;
        render();
        return stream(sess, turn, reply);
      },

      // send follows up the current result.
      async send(follow) {
        const sess = shown();
        if (!sess) return;
        S.follow = "";
        S.followFocus = true;
        const turn = addTurn(sess, follow.ask);
        S.followEnd = true;
        render();
        await stream(sess, turn, follow.reply);
      },

      async runOperator(plan) {
        if (S.op) S.op.cancelled = true;
        const sess = newSession("operator", S.source, true);
        sess.plan = plan;
        S.op = sess;
        S.followFocus = true;
        const turn = addTurn(sess, null);
        S.followEnd = true;
        render();
        sparkle();
        await stream(sess, turn, plan.reply);
        if (sess.cancelled) return;
        plan.apply();
        sess.undoable = true;
        appendReply(
          sess,
          turn,
          "\n\n---\n\n已修改（⌘Z 撤销）：\n\n" +
            plan.changes.map(([path, before, after]) => `- \`${path}\`：${shownValue(before)} → ${shownValue(after)}`).join("\n"),
        );
        render();
      },

      undo() {
        const sess = S.op;
        if (!sess || !sess.undoable || !operatorActive()) return;
        sess.plan.revert();
        sess.undoable = false;
        appendReply(sess, sess.turns[sess.turns.length - 1], "\n\n已撤销，配置恢复到修改之前。");
        render();
      },

      retry(sess) {
        const turn = sess.turns[sess.turns.length - 1];
        if (!turn || turn.status !== "done") return;
        turn.status = "thinking";
        turn.body.replaceChildren(thinking());
        renderFoot(sess);
        stream(sess, turn, turn.reply);
      },

      // reset is ⌘N: a new session, the source and every result cleared.
      reset() {
        for (const s of S.results.values()) s.cancelled = true;
        if (S.op) S.op.cancelled = true;
        S.results.clear();
        S.op = null;
        S.source = "";
        S.themeReturn = "";
        S.preview = "";
        S.follow = "";
        S.followFocus = false;
        S.opSince = 0;
        S.menuSel = 0;
        S.least = 0;
        if (S.fresh) S.freshAt = performance.now();
        inner.style.minHeight = "";
        render();
      },
    };

    function setSource(text) {
      const was = operatorActive();
      S.source = text;
      S.menuSel = 0;
      if (!was && operatorActive()) S.opSince = performance.now();
      if (!operatorActive()) S.opSince = 0;
    }

    // ---------- The visitor at the panel ----------

    // takeOver hands the panel to the visitor: the script stops where it
    // is. A command it half typed goes; what it ran stays. The panel is
    // drawn again only then, so that what the pointer pressed is still
    // there when it lets go.
    function takeOver() {
      if (opts.takeOver) opts.takeOver();
      unfade();
      if (S.follow) {
        S.follow = "";
        S.followFocus = false;
        renderFollow();
      }
      if (S.source.startsWith("/") && !S.op && !themeTyped(S.source)) {
        setSource("");
        render();
      }
    }

    if (interactive) {
      root.addEventListener("pointerdown", () => takeOver());
      root.addEventListener("focus", () => render());
      root.addEventListener("blur", () => {
        if (themeTyped(S.source)) closeThemes();
        render();
      });
      root.addEventListener("keydown", (e) => {
        if (e.metaKey || e.ctrlKey || e.altKey) return;
        const key = e.key;
        if (!["Tab", "Enter", "Escape", "ArrowUp", "ArrowDown"].includes(key)) return;
        takeOver();
        if (menu && menu.kind === "themes") {
          e.preventDefault();
          const n = menu.items.length;
          if (key === "ArrowDown" || key === "ArrowUp") choosePreview((S.menuSel + (key === "ArrowDown" ? 1 : n - 1)) % n);
          else if (key === "Enter" || key === "Tab") takeTheme(menu.items[S.menuSel].key);
          else {
            closeThemes();
            render();
          }
          return;
        }
        if (key === "Tab") {
          e.preventDefault();
          actions.cycle(e.shiftKey ? -1 : 1);
        } else if (key === "Enter") {
          e.preventDefault();
          actions.run();
        } else if (key === "Escape") {
          root.blur();
        }
      });
    } else {
      root.removeAttribute("tabindex");
    }

    systemDark.addEventListener("change", () => {
      applied = null;
      render();
    });

    // ---------- Scripts ----------

    // director returns what a scene's script does with the panel. Each
    // step waits while the panel is off screen, and stops when the run is
    // cancelled.
    function director(run, instant) {
      const alive = () => {
        if (run.cancelled) throw CANCELLED;
      };
      const guard = async (p) => {
        await Promise.race([p, run.stopped]);
        alive();
      };
      const keys = (...k) => {
        if (!instant && opts.keys) opts.keys(k);
      };
      const delay = (ch) => {
        if (/[⺀-鿿＀-￯　-〿]/.test(ch)) return 95 + Math.random() * 40;
        return ch === " " ? 70 : 36 + Math.random() * 30;
      };

      const d = {
        async wait(ms) {
          alive();
          if (instant) return;
          await guard(sleep(ms));
          while (opts.visible && !opts.visible()) await guard(sleep(300));
        },

        // fresh starts a scene from a known state: its example and the
        // settings it expects, a new session in the mode the panel is in.
        async fresh(example, config = {}) {
          alive();
          const fading = !instant && !still() && (S.results.size || S.op || S.source || S.note);
          if (fading) {
            await guard(fadeOut());
            S.hold = isNotes() ? 0 : inner.offsetHeight;
          }
          const id = currentId();
          S.example = example;
          S.modes = config.modes || BUILTIN.slice();
          if (config.theme) setTheme(config.theme);
          S.idx = indexFor(id);
          S.fresh = "";
          S.freshAt = 0;
          S.note = "";
          actions.reset();
          if (fading) fadeIn();
        },

        // mode moves to a mode with Tab or ⇧Tab, the shorter way round.
        async mode(id) {
          const n = S.modes.length + 1;
          const to = indexFor(id);
          const ahead = (to - S.idx + n) % n;
          const behind = (S.idx - to + n) % n;
          const step = ahead <= behind ? 1 : -1;
          for (let k = Math.min(ahead, behind); k > 0; k--) {
            alive();
            keys(...(step > 0 ? ["tab"] : ["⇧", "tab"]));
            actions.cycle(step);
            await d.wait(340);
          }
        },

        // bring is a double ⌥: the selection replaces the source and the
        // default mode runs at once.
        async bring(text) {
          keys("⌥", "⌥");
          if (!instant && opts.shortcut) opts.shortcut("selection");
          await d.wait(420);
          S.idx = 0;
          setSource(text);
          render();
          await guard(actions.run());
        },

        // recognize puts what a screenshot held into the source and
        // translates it, as a capture does.
        async recognize(text) {
          alive();
          S.idx = 0;
          setSource(text);
          render();
          await guard(actions.run());
        },

        async paste(text) {
          alive();
          keys("⌘", "V");
          setSource(text);
          render();
        },

        async type(text) {
          alive();
          if (instant) {
            setSource(S.source + text);
            return render();
          }
          S.typing = true;
          try {
            for (const ch of text) {
              setSource(S.source + ch);
              render();
              await d.wait(delay(ch));
            }
          } finally {
            S.typing = false;
          }
          render();
        },

        async typeFollow(text) {
          alive();
          S.followFocus = true;
          if (instant) {
            S.follow = text;
            return render();
          }
          for (const ch of text) {
            S.follow += ch;
            render();
            await d.wait(delay(ch) + 40);
          }
        },

        async typeNote(text) {
          alive();
          if (instant) {
            S.note = text;
            return render();
          }
          for (const ch of text) {
            S.note += ch;
            render();
            await d.wait(ch === "\n" ? 260 : delay(ch) * 0.8);
          }
        },

        async press() {
          if (!instant && E.run && E.run.isConnected) {
            E.run.classList.add("pressed");
            await d.wait(140);
            E.run.classList.remove("pressed");
          }
        },

        async run() {
          keys("⏎");
          await d.press();
          await guard(actions.run());
        },

        async send(follow) {
          keys("⏎");
          await guard(actions.send(follow));
        },

        async operator(plan) {
          keys("⏎");
          await d.press();
          await guard(actions.runOperator(plan));
        },

        async reset() {
          alive();
          keys("⌘", "N");
          actions.reset();
        },

        async openThemes() {
          alive();
          openThemes();
          render();
        },

        async down() {
          alive();
          keys("↓");
          if (menu) choosePreview((S.menuSel + 1) % menu.items.length);
        },

        async pick() {
          alive();
          keys("⏎");
          if (menu) takeTheme(menu.items[S.menuSel].key);
        },
      };
      return d;
    }

    render();
    return { S, render, setTheme, defineTheme, director };
  }

  // reveal shows a tree's units at an even pace, each fading in, a block
  // from its first unit: Chinese by the character, other text by the word.
  function reveal(tree, units, stopped, onFrame) {
    return new Promise((resolve) => {
      let i = 0;
      let due = performance.now();
      const step = (now) => {
        if (stopped()) return resolve();
        while (i < units.length && due <= now) {
          const u = units[i++];
          u.classList.add("in");
          for (let p = u.parentElement; p && p !== tree; p = p.parentElement) p.classList.remove("pend");
          due += pace(u.textContent);
        }
        if (onFrame) onFrame();
        if (i >= units.length) resolve();
        else requestAnimationFrame(step);
      };
      requestAnimationFrame(step);
    });
  }

  function pace(unit) {
    if (!unit.trim()) return 0;
    if (/^[⺀-鿿豈-﫿＀-￯　-〿]$/.test(unit)) return 34;
    return 64;
  }

  // ---------- Stories: the steps scroll by, the panel plays each ----------

  const HOLD = 5200; // a finished step stays this long, then plays again

  const STORIES = {
    modes: [
      async (d) => {
        await d.fresh(TERMINAL);
        await d.mode("translate");
        await d.wait(400);
        await d.bring(TERMINAL.source);
        await d.wait(1500);
        await d.typeFollow(TERMINAL.follow.ask);
        await d.wait(300);
        await d.send(TERMINAL.follow);
      },
      async (d) => {
        await d.fresh(CHAT);
        await d.mode("improve");
        await d.wait(400);
        await d.paste(CHAT.source);
        await d.wait(700);
        await d.run();
      },
      async (d) => {
        await d.fresh(QUESTION);
        await d.mode("ask");
        await d.wait(300);
        await d.type(QUESTION.source);
        await d.wait(350);
        await d.run();
      },
      async (d) => {
        await d.fresh(null);
        await d.mode("note");
        await d.wait(600);
        await d.typeNote(md("# 官网上线", "- 首屏文案周三定", "- 演示面板：手机、减少动态效果", "- 下载 https://github.com/Umupa/Kami/releases/latest"));
      },
    ],
    custom: [
      async (d, p) => {
        await d.fresh(null, { modes: BUILTIN.slice(), theme: "flexoki-light" });
        await d.wait(500);
        await d.type("/operator 加一个把改动写成 commit message 的模式");
        await d.wait(400);
        await d.operator({
          reply: "加好了「提交信息」模式，排在问答后面：把一段改动或 diff 写成一条 commit message，第一行是简短的摘要，正文说明为什么要改。",
          changes: [["modes", "", JSON.stringify([...BUILTIN.map((m) => ({ id: m.id })), { id: COMMIT.id, name: COMMIT.name, prompt: COMMIT.desc }])]],
          apply: () => {
            p.S.modes = [...BUILTIN, COMMIT];
            p.S.fresh = COMMIT.id;
          },
          revert: () => {
            p.S.modes = BUILTIN.slice();
          },
        });
        await d.wait(2400);
        await d.reset();
        await d.wait(1100);
        await d.mode(COMMIT.id);
      },
      async (d, p) => {
        await d.fresh(null, { modes: [...BUILTIN, COMMIT], theme: "flexoki-light" });
        await d.wait(500);
        await d.type("/operator 把强调色改成橙色");
        await d.wait(400);
        await d.operator({
          reply: "好的，基于 flexoki-light 新建了一套 flexoki-light-custom，强调色改成了橙色。",
          changes: [
            ["themes[0]", "", JSON.stringify({ base: ORANGE.base, colors: ORANGE.colors, id: ORANGE.id })],
            ["theme", JSON.stringify("flexoki-light"), JSON.stringify(ORANGE.id)],
          ],
          apply: () => {
            p.defineTheme(ORANGE);
            p.setTheme(ORANGE.id);
          },
          revert: () => p.setTheme("flexoki-light"),
        });
        await d.wait(2400);
        await d.reset();
      },
      async (d, p) => {
        p.defineTheme(ORANGE);
        await d.fresh(null, { modes: [...BUILTIN, COMMIT], theme: ORANGE.id });
        await d.wait(700);
        await d.openThemes();
        await d.wait(800);
        for (let k = 0; k < 15; k++) {
          await d.down();
          await d.wait(380);
        }
        await d.wait(500);
        await d.pick();
      },
    ],
  };

  function flash(box, keys) {
    if (!box) return;
    box.replaceChildren(...keys.map((k) => el("kbd", "", k)));
  }

  // pulse presses a global shortcut at the top of the page, as a scene
  // uses it: in the strip, and the floating keys it takes.
  function pulse(name) {
    if (reduced) return;
    for (const n of document.querySelectorAll(`[data-shortcut="${name}"], [data-keys~="${name}"]`)) {
      n.classList.remove("pulse");
      void n.offsetWidth;
      n.classList.add("pulse");
    }
  }

  const scrollers = [];
  let scrollQueued = false;
  function onScroll(fn) {
    scrollers.push(fn);
  }
  const runScrollers = () => {
    scrollQueued = false;
    for (const fn of scrollers) fn();
  };
  addEventListener(
    "scroll",
    () => {
      if (!scrollQueued) {
        scrollQueued = true;
        requestAnimationFrame(runScrollers);
      }
    },
    { passive: true },
  );
  addEventListener("resize", runScrollers);

  function watch(node, fn, threshold = 0) {
    new IntersectionObserver(([e]) => fn(e.isIntersecting), { threshold }).observe(node);
  }

  function story(section) {
    const defs = STORIES[section.dataset.story];
    const steps = [...section.querySelectorAll(".step")];
    const hint = section.querySelector(".keyhint");
    const header = document.querySelector(".top");
    const ctl = { visible: false, run: null, active: -1 };
    const p = createPanel(section.querySelector(".kami"), {
      keys: (k) => flash(hint, k),
      shortcut: pulse,
      visible: () => ctl.visible && !document.hidden,
      takeOver: () => {
        if (ctl.run) ctl.run.cancel();
      },
    });

    function play(i) {
      if (ctl.run) ctl.run.cancel();
      const run = (ctl.run = newRun());
      (async () => {
        for (;;) {
          await defs[i](p.director(run, reduced), p);
          if (reduced) return;
          await p.director(run, false).wait(HOLD);
        }
      })().catch(quiet);
    }

    // The lengths of page a step takes, and the last one after it, as the
    // stylesheet sets them in vh. Read again whenever the viewport changes
    // (a phone's toolbar, or the layout viewport settling), not every frame.
    const len = { step: 0, tail: 0, header: 0, w: 0, h: 0 };
    const measure = () => {
      if (len.w === innerWidth && len.h === innerHeight) return;
      const css = getComputedStyle(section);
      len.step = ((parseFloat(css.getPropertyValue("--step")) || 58) * innerHeight) / 100;
      len.tail = ((parseFloat(css.getPropertyValue("--tail")) || 28) * innerHeight) / 100;
      len.header = header.offsetHeight;
      len.w = innerWidth;
      len.h = innerHeight;
    };

    const dots = steps.map((step, i) => {
      const b = el("button");
      b.type = "button";
      b.setAttribute("role", "tab");
      b.setAttribute("aria-label", step.querySelector("h2").textContent);
      b.addEventListener("click", () => {
        measure();
        const top = section.getBoundingClientRect().top + scrollY - len.header;
        scrollTo({ top: top + i * len.step + 2, behavior: reduced ? "auto" : "smooth" });
      });
      section.querySelector(".story-dots").append(b);
      return b;
    });

    function show(i) {
      if (i === ctl.active) return;
      ctl.active = i;
      steps.forEach((s, j) => {
        s.classList.toggle("on", j === i);
        s.classList.toggle("past", j < i);
      });
      dots.forEach((d, j) => {
        d.setAttribute("aria-selected", String(j === i));
        d.classList.toggle("past", j < i);
      });
      play(i);
    }

    // scrub moves the steps' text with the page: each is whole around the
    // middle of its length and slides out across the last part of it, as
    // the next slides in, so the text follows every bit of scrolling.
    function scrub(at) {
      const last = steps.length - 1;
      steps.forEach((s, j) => {
        let d = at - (j + 0.5);
        if (j === 0) d = Math.max(d, 0);
        if (j === last) d = Math.min(d, 0);
        const a = Math.max(0, Math.min(1, 1 - (Math.abs(d) - 0.3) * 5));
        s.style.opacity = a.toFixed(3);
        s.style.transform = `translate3d(0, ${(-d * 36).toFixed(1)}px, 0)`;
        s.style.visibility = a > 0.001 ? "visible" : "hidden";
      });
    }

    // The step is how far the stage has been held, one step length each;
    // its line fills across that length as the page scrolls.
    function update() {
      const r = section.getBoundingClientRect();
      if (r.bottom <= 0 || r.top >= innerHeight) return;
      measure();
      const held = Math.max(0, len.header - r.top);
      const last = steps.length - 1;
      const at = held / len.step;
      const i = Math.min(last, Math.floor(at));
      const through = i < last ? at - i : (held - last * len.step) / len.tail;
      show(i);
      if (!reduced) scrub(at);
      dots[i].style.setProperty("--p", Math.max(0, Math.min(1, through)).toFixed(3));
    }
    if (!reduced) section.classList.add("scrub");

    watch(section, (v) => {
      ctl.visible = v;
      if (v) update();
    });
    onScroll(update);
  }

  // ---------- The capture, acted out ----------

  function capture(section) {
    const wrap = section.querySelector(".screen-wrap");
    const screen = section.querySelector(".screen");
    const q = (sel) => screen.querySelector(sel);
    const veil = q(".veil");
    const sel = q(".selection");
    const hint = q(".hint");
    const cross = q(".crosshair");
    const press = q(".press");
    const scenes = [...screen.querySelectorAll(".scene")];
    const buttons = [...section.querySelectorAll(".scenes button")];
    const panelEl = wrap.querySelector(".shot-panel");
    const ctl = { visible: false, run: null, started: false };
    const p = createPanel(panelEl, { interactive: false, visible: () => ctl.visible && !document.hidden });
    panelEl.hidden = true;

    const appName = screen.querySelector(".menubar-app");
    // The device is the 960×600 screen in a 12px bezel.
    const fit = () => wrap.style.setProperty("--s", String(wrap.clientWidth / 984));
    new ResizeObserver(fit).observe(wrap);
    fit();

    // area is where a scene's text sits on the 960×600 screen, with room.
    function area(sc) {
      const box = screen.getBoundingClientRect();
      const s = box.width / 960;
      let x0 = Infinity;
      let y0 = Infinity;
      let x1 = -Infinity;
      let y1 = -Infinity;
      for (const t of sc.target) {
        const r = q(`[data-scene="${sc.id}"] ${t}`).getBoundingClientRect();
        x0 = Math.min(x0, (r.left - box.left) / s);
        y0 = Math.min(y0, (r.top - box.top) / s);
        x1 = Math.max(x1, (r.right - box.left) / s);
        y1 = Math.max(y1, (r.bottom - box.top) / s);
      }
      return { x0: x0 - 12, y0: y0 - 9, x1: x1 + 12, y1: y1 + 9 };
    }

    function place(x0, y0, x1, y1) {
      sel.style.left = Math.min(x0, x1) + "px";
      sel.style.top = Math.min(y0, y1) + "px";
      sel.style.width = Math.abs(x1 - x0) + "px";
      sel.style.height = Math.abs(y1 - y0) + "px";
      cross.style.transform = `translate(${x1}px, ${y1}px)`;
    }

    function animate(run, ms, step) {
      return new Promise((resolve) => {
        const t0 = performance.now();
        const frame = (now) => {
          if (run.cancelled) return resolve();
          const t = Math.min(1, (now - t0) / ms);
          step(t);
          if (t < 1) requestAnimationFrame(frame);
          else resolve();
        };
        requestAnimationFrame(frame);
      });
    }

    function clear() {
      for (const n of [veil, sel, hint, cross, press]) n.classList.remove("on");
    }

    async function act(i, run, instant) {
      const sc = SCENES[i];
      const d = p.director(run, instant);
      scenes.forEach((s) => {
        s.classList.toggle("on", s.dataset.scene === sc.id);
        if (s.dataset.scene === sc.id) appName.textContent = s.dataset.app;
      });
      buttons.forEach((b, j) => {
        b.setAttribute("aria-selected", String(j === i));
        b.tabIndex = j === i ? 0 : -1;
      });
      progress(i, sc, instant);
      clear();
      panelEl.hidden = true;
      await d.fresh({ replies: { translate: sc.reply } });
      if (!instant) {
        await d.wait(1300);
        press.classList.add("on");
        pulse("capture");
        await d.wait(500);
        // The screen freezes under a paper veil; the hint sits where the
        // panel will.
        veil.classList.add("on");
        hint.classList.add("on");
        await d.wait(450);
        press.classList.remove("on");
        const a = area(sc);
        place(a.x0, a.y0, a.x0, a.y0);
        cross.classList.add("on");
        await d.wait(380);
        sel.classList.add("on");
        veil.classList.remove("on");
        const ease = (t) => (t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2);
        await animate(run, 950, (t) => {
          const e = ease(t);
          place(a.x0, a.y0, a.x0 + (a.x1 - a.x0) * e, a.y0 + (a.y1 - a.y0) * e);
        });
        await d.wait(260);
        // Let go: the layer goes, the panel is there at once, translating.
        clear();
      }
      panelEl.hidden = false;
      await d.recognize(sc.source);
      if (instant) return;
      await d.wait(3200);
      panelEl.hidden = true;
      await d.wait(600);
    }

    // progress fills the scene's bar over about the time it plays: the
    // capture, the reply arriving, and the moment it stays.
    function progress(i, sc, instant) {
      if (ctl.progress) ctl.progress.cancel();
      ctl.progress = null;
      const bar = buttons[i].querySelector(".bar");
      if (instant || reduced) return;
      const ms = 8600 + Array.from(sc.reply).length * 34;
      ctl.progress = bar.animate([{ transform: "scaleX(0)" }, { transform: "scaleX(1)" }], { duration: ms, fill: "forwards" });
      if (!ctl.visible || document.hidden) ctl.progress.pause();
    }

    function playFrom(i) {
      if (ctl.run) ctl.run.cancel();
      const run = (ctl.run = newRun());
      ctl.started = true;
      if (reduced) {
        act(i, run, true).catch(quiet);
        return;
      }
      (async () => {
        for (let k = i; ; k = (k + 1) % SCENES.length) await act(k, run, false);
      })().catch(quiet);
    }

    buttons.forEach((b, j) => {
      b.addEventListener("click", () => playFrom(j));
      b.addEventListener("keydown", (e) => {
        const step = { ArrowRight: 1, ArrowLeft: -1 }[e.key];
        if (!step) return;
        e.preventDefault();
        const k = (j + step + buttons.length) % buttons.length;
        buttons[k].focus();
        playFrom(k);
      });
    });
    const paced = () => {
      if (!ctl.progress) return;
      if (ctl.visible && !document.hidden) ctl.progress.play();
      else ctl.progress.pause();
    };
    document.addEventListener("visibilitychange", paced);
    watch(
      wrap,
      (v) => {
        ctl.visible = v;
        paced();
        if (v && !ctl.started) playFrom(0);
      },
      0.35,
    );
  }

  // ---------- Getting Kami ----------

  async function copyText(text) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {}
    try {
      const area = el("textarea");
      area.value = text;
      area.setAttribute("readonly", "");
      area.style.cssText = "position:fixed;top:0;left:0;opacity:0";
      document.body.append(area);
      area.select();
      const ok = document.execCommand("copy");
      area.remove();
      return ok;
    } catch {
      return false;
    }
  }

  // copied shows on a button that it did its job, for a moment.
  function copied(button, label, ok) {
    const icon = button.querySelector("svg");
    const text = button.querySelector("span");
    const was = { icon: icon && icon.outerHTML, text: text && text.textContent };
    clearTimeout(button._undo);
    if (icon) icon.outerHTML = ICON.check;
    if (text) text.textContent = ok ? label : "复制失败，请重试";
    button.classList.add("done");
    button._undo = setTimeout(() => {
      const now = button.querySelector("svg");
      if (now && was.icon) now.outerHTML = was.icon;
      if (text) text.textContent = was.text;
      button.classList.remove("done");
    }, 2400);
  }

  // arch guesses the visitor's Mac: the browser says so in Chromium; in
  // Safari the graphics name it. Apple silicon when nothing tells.
  async function arch() {
    try {
      const ua = navigator.userAgentData;
      if (ua && ua.getHighEntropyValues) {
        const { architecture } = await ua.getHighEntropyValues(["architecture"]);
        if (architecture === "x86") return "amd64";
        if (architecture === "arm") return "arm64";
      }
    } catch {}
    try {
      const gl = document.createElement("canvas").getContext("webgl");
      const info = gl && gl.getExtension("WEBGL_debug_renderer_info");
      if (info && /Intel|AMD|Radeon/i.test(gl.getParameter(info.UNMASKED_RENDERER_WEBGL))) return "amd64";
    } catch {}
    return "arm64";
  }

  // latest asks GitHub for the latest release, once a visit.
  async function latest() {
    try {
      const kept = sessionStorage.getItem("kami-release");
      if (kept) return JSON.parse(kept);
    } catch {}
    const stop = new AbortController();
    const timer = setTimeout(() => stop.abort(), 6000);
    try {
      const r = await fetch("https://api.github.com/repos/Umupa/Kami/releases/latest", {
        headers: { Accept: "application/vnd.github+json" },
        signal: stop.signal,
      });
      if (!r.ok) return null;
      const rel = await r.json();
      const kept = { tag: rel.tag_name, assets: (rel.assets || []).map((a) => ({ name: a.name, url: a.browser_download_url, size: a.size })) };
      try {
        sessionStorage.setItem("kami-release", JSON.stringify(kept));
      } catch {}
      return kept;
    } catch {
      return null;
    } finally {
      clearTimeout(timer);
    }
  }

  // downloads points the download buttons at the disk image for this Mac;
  // without one, they keep to the release page.
  async function downloads() {
    const links = document.querySelectorAll("[data-download]");
    if (!links.length) return;
    const [rel, chip] = await Promise.all([latest(), arch()]);
    const dmg = rel && rel.assets.find((a) => a.name.endsWith(`.${chip}.dmg`));
    if (!dmg) return;
    for (const a of links) {
      a.href = dmg.url;
      a.title = `${dmg.name} · ${(dmg.size / 1e6).toFixed(1)} MB`;
    }
  }

  function getting() {
    for (const b of document.querySelectorAll("[data-agent]")) {
      b.addEventListener("click", async () => {
        const ok = await copyText(window.KAMI_AGENT_PROMPT || "");
        copied(b, "已复制，粘贴给 Agent", ok);
      });
    }
    for (const b of document.querySelectorAll("[data-copy]")) {
      b.addEventListener("click", async () => {
        const ok = await copyText(b.dataset.copy);
        copied(b, "", ok);
        b.setAttribute("aria-label", ok ? "已复制" : "复制失败");
        setTimeout(() => b.setAttribute("aria-label", "复制命令"), 2400);
      });
    }
    downloads();
  }

  // ---------- The hero: the fox, and the keys that bring it up ----------

  // The hero's keys: the ones Kami answers to, beside its icon, which stays
  // still. They press when clicked, can be thrown and spring back, and press
  // when the visitor presses them, or when a demo below uses them.
  function hero() {
    const art = document.querySelector(".hero-art");
    if (!art || getComputedStyle(art).display === "none") return;
    const caps = Object.fromEntries([...art.querySelectorAll(".cap")].map((c) => [c.dataset.key, c]));
    const down = (code) => caps[code] && caps[code].classList.add("down");
    const up = (code) => caps[code] && caps[code].classList.remove("down");
    let visible = false;

    // A key can be picked up and thrown; it springs back to its place.
    for (const wrap of art.querySelectorAll(".cap-wrap")) {
      const code = wrap.querySelector(".cap").dataset.key;
      const m = { x: 0, y: 0, vx: 0, vy: 0, from: null, moved: false, raf: 0, last: null };
      const place = () => {
        wrap.style.setProperty("--dx", m.x.toFixed(1) + "px");
        wrap.style.setProperty("--dy", m.y.toFixed(1) + "px");
      };
      const spring = () => {
        m.vx = m.vx * 0.84 - m.x * 0.09;
        m.vy = m.vy * 0.84 - m.y * 0.09;
        m.x += m.vx;
        m.y += m.vy;
        place();
        if (Math.abs(m.x) + Math.abs(m.y) + Math.abs(m.vx) + Math.abs(m.vy) > 0.3) m.raf = requestAnimationFrame(spring);
        else {
          m.x = m.y = m.vx = m.vy = 0;
          place();
          m.raf = 0;
        }
      };
      wrap.addEventListener("pointerdown", (e) => {
        e.preventDefault();
        wrap.setPointerCapture(e.pointerId);
        cancelAnimationFrame(m.raf);
        m.from = { x: e.clientX - m.x, y: e.clientY - m.y };
        m.last = { x: e.clientX, y: e.clientY, t: performance.now() };
        m.moved = false;
        down(code);
        wrap.classList.add("dragging");
      });
      wrap.addEventListener("pointermove", (e) => {
        if (!m.from) return;
        m.x = e.clientX - m.from.x;
        m.y = e.clientY - m.from.y;
        const now = performance.now();
        const dt = Math.max(1, now - m.last.t);
        m.vx = ((e.clientX - m.last.x) / dt) * 16;
        m.vy = ((e.clientY - m.last.y) / dt) * 16;
        m.last = { x: e.clientX, y: e.clientY, t: now };
        if (!m.moved && Math.hypot(m.x, m.y) > 5) {
          m.moved = true;
          up(code);
        }
        place();
      });
      const release = () => {
        if (!m.from) return;
        m.from = null;
        wrap.classList.remove("dragging");
        up(code);
        if (reduced) {
          m.x = m.y = 0;
          place();
        } else m.raf = requestAnimationFrame(spring);
      };
      wrap.addEventListener("pointerup", release);
      wrap.addEventListener("pointercancel", release);
    }

    const typing = (t) => t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName));
    addEventListener("keydown", (e) => {
      if (!visible || e.repeat || e.metaKey || e.ctrlKey || typing(e.target)) return;
      if (e.key === "Alt") down("Alt");
      else if (e.altKey && (e.code === "Space" || e.code === "KeyS")) down(e.code);
    });
    addEventListener("keyup", (e) => {
      if (e.key === "Alt") up("Alt");
      if (e.code === "Space" || e.code === "KeyS") up(e.code);
    });
    addEventListener("blur", () => Object.keys(caps).forEach(up));
    watch(art, (v) => (visible = v), 0.4);
  }

  // ---------- The page ----------

  function page() {
    const top = document.querySelector(".top");
    const header = () => top && top.classList.toggle("scrolled", scrollY > 4);
    onScroll(header);
    header();

    const io = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (!e.isIntersecting) continue;
          e.target.classList.add("in");
          io.unobserve(e.target);
        }
      },
      { threshold: 0.15, rootMargin: "0px 0px -6% 0px" },
    );
    for (const n of document.querySelectorAll("[data-reveal]")) io.observe(n);

    // The greeting writes itself out the first time it is seen.
    const greeting = document.querySelector(".greeting .md");
    if (greeting && !reduced) {
      const units = unitize(greeting);
      for (const b of greeting.querySelectorAll(BLOCKS)) b.classList.add("pend");
      let done = false;
      watch(
        greeting,
        (v) => {
          if (!v || done) return;
          done = true;
          setTimeout(() => reveal(greeting, units, () => false).then(() => units.forEach((u) => (u.className = "u done"))), 350);
        },
        0.3,
      );
    }

    // The update screen's choices switch like Kami's.
    for (const box of document.querySelectorAll(".kami.update .k-modes")) {
      for (const chip of box.children) {
        chip.addEventListener("click", () => {
          for (const c of box.children) c.classList.toggle("on", c === chip);
        });
      }
    }
  }

  page();
  getting();
  if (!THEMES.length) return;
  hero();
  for (const section of document.querySelectorAll(".story")) story(section);
  const cap = document.querySelector(".capture");
  if (cap) capture(cap);
  runScrollers();
})();
