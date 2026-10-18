package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

// Problem is one thing wrong with config.json. Path is a JSON path such as
// "models[1].endpoint", empty for the file as a whole.
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (p Problem) String() string {
	if p.Path == "" {
		return p.Message
	}
	return p.Path + "：" + p.Message
}

// Error lists every problem Decode found, so that one edit can fix them all.
type Error struct {
	Problems []Problem
}

func (e *Error) Error() string {
	lines := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		lines[i] = p.String()
	}
	return strings.Join(lines, "\n")
}

// Decode parses and validates config.json. On failure the error is an *Error
// naming every problem it found.
func Decode(data []byte) (*Config, error) {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, &Error{Problems: []Problem{{Message: syntaxMessage(data, err)}}}
	}
	if _, ok := raw.(map[string]any); !ok {
		return nil, &Error{Problems: []Problem{{Message: "配置文件必须是一个 JSON 对象"}}}
	}
	var problems []Problem
	unknownFields(raw, reflect.TypeFor[Config](), "", &problems)

	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) {
			problems = append(problems, Problem{Path: te.Field, Message: fmt.Sprintf("类型不对，应为 %s", typeName(te.Type))})
		} else {
			problems = append(problems, Problem{Message: err.Error()})
		}
		return nil, &Error{Problems: problems}
	}
	problems = append(problems, c.validate()...)
	if len(problems) > 0 {
		return nil, &Error{Problems: problems}
	}
	c.applyDefaults()
	return &c, nil
}

// syntaxMessage locates a JSON syntax error by line and column, which agents
// and editors find faster than a byte offset.
func syntaxMessage(data []byte, err error) string {
	var se *json.SyntaxError
	if !errors.As(err, &se) {
		return "不是合法的 JSON：" + err.Error()
	}
	before := data[:min(int(se.Offset), len(data))]
	line := bytes.Count(before, []byte("\n")) + 1
	col := len([]rune(string(before[bytes.LastIndexByte(before, '\n')+1:])))
	return fmt.Sprintf("第 %d 行第 %d 列不是合法的 JSON：%s", line, col, se.Error())
}

func typeName(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "字符串"
	case reflect.Bool:
		return "true 或 false"
	case reflect.Int, reflect.Int64, reflect.Float64:
		return "数字"
	case reflect.Slice:
		return "数组"
	case reflect.Map, reflect.Struct:
		return "对象"
	}
	return t.String()
}

// unknownFields reports keys of v that t has no field for, by their full
// path, so that a misspelled field is not silently ignored. Free-form maps
// (headers, body) accept any key.
func unknownFields(v any, t reflect.Type, path string, out *[]Problem) {
	switch t.Kind() {
	case reflect.Struct:
		obj, ok := v.(map[string]any)
		if !ok {
			return
		}
		fields := map[string]reflect.Type{}
		for f := range t.Fields() {
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name != "" && name != "-" {
				fields[name] = f.Type
			}
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			ft, ok := fields[k]
			if !ok {
				*out = append(*out, Problem{Path: joinPath(path, k), Message: "不认识的字段"})
				continue
			}
			unknownFields(obj[k], ft, joinPath(path, k), out)
		}
	case reflect.Slice:
		arr, ok := v.([]any)
		if !ok {
			return
		}
		for i, e := range arr {
			unknownFields(e, t.Elem(), fmt.Sprintf("%s[%d]", path, i), out)
		}
	}
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// validate checks what JSON types alone cannot, before defaults are applied.
func (c *Config) validate() []Problem {
	var ps []Problem
	add := func(path, format string, args ...any) {
		ps = append(ps, Problem{Path: path, Message: fmt.Sprintf(format, args...)})
	}

	if c.Version != Version {
		add("version", "必须是 %d", Version)
	}

	names := map[string]bool{}
	for i, m := range c.Models {
		p := fmt.Sprintf("models[%d]", i)
		switch {
		case m.Name == "":
			add(p+".name", "不能为空")
		case names[m.Name]:
			add(p+".name", "与前面的模型重名：%s", m.Name)
		}
		names[m.Name] = true
		if u, err := url.Parse(m.Endpoint); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			add(p+".endpoint", "必须是完整的 http(s) 地址")
		}
		switch m.Format {
		case FormatChatCompletions, FormatResponses, FormatAnthropicMessages:
		default:
			add(p+".format", "必须是 chat-completions、responses 或 anthropic-messages")
		}
		if m.Model == "" {
			add(p+".model", "不能为空")
		}
		switch m.Auth {
		case "", AuthBearer, AuthXAPIKey, AuthAPIKey, AuthNone:
		default:
			add(p+".auth", "必须是 bearer、x-api-key、api-key 或 none")
		}
		for k := range m.Headers {
			if !validHeaderName(k) {
				add(p+".headers", "「%s」不是合法的请求头名", k)
			}
		}
		if m.ContextWindow < 0 {
			add(p+".contextWindow", "不能是负数")
		}
	}
	if c.Theme != "" && !slices.Contains(Themes, c.Theme) {
		add("theme", "必须是 %s 之一", strings.Join(Themes, "、"))
	}
	if c.Font != "" && !slices.Contains(Fonts, c.Font) {
		add("font", "必须是 %s 之一", strings.Join(Fonts, "、"))
	}
	if c.FontSize != 0 && (c.FontSize < MinFontSize || c.FontSize > MaxFontSize) {
		add("fontSize", "必须在 %d 到 %d 之间", MinFontSize, MaxFontSize)
	}
	if c.Background != "" && !slices.Contains(Backgrounds, c.Background) {
		add("background", "必须是 %s 之一", strings.Join(Backgrounds, "、"))
	}
	if c.DefaultModel != "" && !names[c.DefaultModel] {
		add("defaultModel", "没有名为「%s」的模型", c.DefaultModel)
	}

	ids := map[string]bool{}
	for i, m := range c.Modes {
		p := fmt.Sprintf("modes[%d]", i)
		switch {
		case !idPattern.MatchString(m.ID):
			add(p+".id", "只能用小写字母、数字和连字符")
		case ids[m.ID]:
			add(p+".id", "与前面的模式重复：%s", m.ID)
		}
		ids[m.ID] = true
		switch m.Input {
		case "", InputMaterial, InputInstruction:
		default:
			add(p+".input", "必须是 material 或 instruction")
		}
		switch m.Output {
		case "", OutputAuto, OutputMarkdown, OutputPlain:
		default:
			add(p+".output", "必须是 auto、markdown 或 plain")
		}
		if m.Model != "" && !names[m.Model] {
			add(p+".model", "没有名为「%s」的模型", m.Model)
		}
	}

	// Shortcuts are compared with the defaults filled in, so that one set to
	// another's default is caught too.
	seen := map[Shortcut]string{}
	for _, s := range []struct{ path, value, fallback string }{
		{"shortcuts.panel", c.Shortcuts.Panel, DefaultPanelShortcut},
		{"shortcuts.selection", c.Shortcuts.Selection, DefaultSelectionShortcut},
		{"shortcuts.capture", c.Shortcuts.Capture, DefaultCaptureShortcut},
	} {
		if s.value == "" {
			s.value = s.fallback
		}
		sc, err := ParseShortcut(s.value)
		if err != nil {
			add(s.path, "%v", err)
			continue
		}
		if other, dup := seen[sc]; dup && !sc.IsZero() {
			add(s.path, "与 %s 相同", other)
		}
		seen[sc] = s.path
	}
	return ps
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		token := '0' <= r && r <= '9' || 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)
		if !token {
			return false
		}
	}
	return true
}
