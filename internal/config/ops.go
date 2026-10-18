package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// Op sets the value at a path of config.json. Path is dotted keys and
// [index] steps, as in Problem paths: "shortcuts.capture", "modes",
// "modes[2].prompt", "defaultModel". A JSON null Value removes the key, or
// the array element at the last [index].
type Op struct {
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

// Change is one path an edit changed, its values before and after as
// compact JSON; "" for absent.
type Change struct {
	Path, Before, After string
}

// ApplyOps applies ops in order to config.json as written (data, without
// defaults) and returns the new file contents, formatted as Encode formats
// them, and what changed. Paths under "models", and "$schema" and "version",
// are refused: model services are configured by an agent. The result must pass
// Decode and, when check is set, check; otherwise nothing is returned but the
// error, an *Error naming the problems.
func ApplyOps(data []byte, ops []Op, check func(*Config) error) ([]byte, []Change, error) {
	doc, err := decodeJSON(data)
	if err != nil {
		return nil, nil, &Error{Problems: []Problem{{Message: syntaxMessage(data, err)}}}
	}
	root, ok := doc.(map[string]any)
	if !ok {
		return nil, nil, &Error{Problems: []Problem{{Message: "配置文件必须是一个 JSON 对象"}}}
	}
	original := cloneJSON(root)

	seen := make(map[string]bool, len(ops))
	var paths []pathEntry
	for _, op := range ops {
		steps, err := parsePath(op.Path)
		if err != nil {
			return nil, nil, opError(op.Path, "%v", err)
		}
		if reason, refused := refusedPath(steps); refused {
			return nil, nil, opError(op.Path, "%s", reason)
		}
		value, remove, err := parseValue(op.Value)
		if err != nil {
			return nil, nil, opError(op.Path, "值不是合法的 JSON")
		}
		if !seen[op.Path] {
			seen[op.Path] = true
			paths = append(paths, pathEntry{path: op.Path, steps: steps})
		}

		if remove {
			if _, ok := lookupPath(root, steps); !ok {
				continue
			}
			edited, err := removeAt(root, steps)
			if err != nil {
				return nil, nil, opError(op.Path, "%v", err)
			}
			root = edited.(map[string]any)
			continue
		}
		edited, err := setAt(root, true, steps, value)
		if err != nil {
			return nil, nil, opError(op.Path, "%v", err)
		}
		root = edited.(map[string]any)
	}

	edited, err := json.Marshal(root)
	if err != nil {
		return nil, nil, opError("", "不能生成编辑后的配置：%v", err)
	}
	if _, err := Decode(edited); err != nil {
		return nil, nil, err
	}
	var c Config
	if err := json.Unmarshal(edited, &c); err != nil {
		return nil, nil, opError("", "不能读取编辑后的配置：%v", err)
	}
	out, err := Encode(&c)
	if err != nil {
		return nil, nil, opError("", "不能格式化配置：%v", err)
	}
	// Encode may leave out empty fields, so validate what is actually written.
	decoded, err := Decode(out)
	if err != nil {
		return nil, nil, err
	}
	if check != nil {
		if err := check(decoded); err != nil {
			return nil, nil, checkError(err)
		}
	}

	var changes []Change
	for _, p := range paths {
		before, err := changedValue(original, p.steps)
		if err != nil {
			return nil, nil, opError(p.path, "%v", err)
		}
		after, err := changedValue(root, p.steps)
		if err != nil {
			return nil, nil, opError(p.path, "%v", err)
		}
		if before == after {
			continue
		}
		changes = append(changes, Change{Path: p.path, Before: before, After: after})
	}
	return out, changes, nil
}

// pathEntry remembers a unique op path and the steps parsed from it.
type pathEntry struct {
	path  string
	steps []pathStep
}

// pathStep is one key or array index of an op path.
type pathStep struct {
	key     string
	index   int
	isIndex bool
}

// parsePath reads dotted keys and [index] steps. Keys start with a letter or
// $ and continue with letters and digits, matching the fields of Config.
func parsePath(path string) ([]pathStep, error) {
	if path == "" {
		return nil, errors.New("路径不能为空")
	}
	key, i, ok := scanKey(path, 0)
	if !ok {
		return nil, errors.New("路径必须以字段名开头")
	}
	steps := []pathStep{{key: key}}
	for i < len(path) {
		switch path[i] {
		case '.':
			i++
			key, next, ok := scanKey(path, i)
			if !ok {
				return nil, fmt.Errorf("第 %d 个字符处应为字段名", i+1)
			}
			steps = append(steps, pathStep{key: key})
			i = next
		case '[':
			start := i + 1
			i = start
			for i < len(path) && '0' <= path[i] && path[i] <= '9' {
				i++
			}
			if i == start || i == len(path) || path[i] != ']' {
				return nil, fmt.Errorf("第 %d 个字符处的数组下标不合法", start+1)
			}
			n, err := strconv.Atoi(path[start:i])
			if err != nil {
				return nil, fmt.Errorf("第 %d 个字符处的数组下标不合法", start+1)
			}
			steps = append(steps, pathStep{index: n, isIndex: true})
			i++
		default:
			return nil, fmt.Errorf("第 %d 个字符处应为 . 或 [", i+1)
		}
	}
	return steps, nil
}

func scanKey(path string, i int) (string, int, bool) {
	start := i
	if i >= len(path) {
		return "", i, false
	}
	c := path[i]
	if !('A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || c == '$') {
		return "", i, false
	}
	i++
	for i < len(path) {
		c := path[i]
		if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' {
			i++
			continue
		}
		break
	}
	return path[start:i], i, true
}

// refusedPath names the roots an edit may not touch. Models carry API keys
// and are configured by an agent; the schema and version describe the file
// itself.
func refusedPath(steps []pathStep) (string, bool) {
	if len(steps) == 0 || steps[0].isIndex {
		return "", false
	}
	switch steps[0].key {
	case "models":
		return "不能修改 models：模型服务由 agent 配置", true
	case "$schema", "version":
		return "不能修改 " + steps[0].key, true
	}
	return "", false
}

// parseValue reads an op value; a JSON null means removal.
func parseValue(raw json.RawMessage) (value any, remove bool, err error) {
	value, err = decodeJSON(raw)
	if err != nil {
		return nil, false, err
	}
	return value, value == nil, nil
}

// decodeJSON reads exactly one JSON value, keeping numbers as written.
func decodeJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("有多余的内容")
		}
		return nil, err
	}
	return v, nil
}

// setAt sets steps within v and returns the new value. present distinguishes a
// missing object, which setting a key creates, from an explicit JSON null,
// which is not an object.
func setAt(v any, present bool, steps []pathStep, value any) (any, error) {
	step := steps[0]
	last := len(steps) == 1

	if step.isIndex {
		if !present {
			return nil, errors.New("路径经过的数组不存在")
		}
		arr, ok := v.([]any)
		if !ok {
			return nil, errors.New("路径经过的不是数组")
		}
		if step.index > len(arr) {
			return nil, fmt.Errorf("数组下标 %d 超出范围（长度 %d）", step.index, len(arr))
		}
		if last {
			if step.index == len(arr) {
				return append(arr, value), nil
			}
			arr[step.index] = value
			return arr, nil
		}
		if step.index == len(arr) {
			// Appending an element and setting inside it needs an object to
			// hold the next key.
			if steps[1].isIndex {
				return nil, fmt.Errorf("数组下标 %d 的元素不存在", step.index)
			}
			elem, err := setAt(map[string]any{}, true, steps[1:], value)
			if err != nil {
				return nil, err
			}
			return append(arr, elem), nil
		}
		elem, err := setAt(arr[step.index], true, steps[1:], value)
		if err != nil {
			return nil, err
		}
		arr[step.index] = elem
		return arr, nil
	}

	if !present {
		m := map[string]any{}
		if last {
			m[step.key] = value
			return m, nil
		}
		elem, err := setAt(nil, false, steps[1:], value)
		if err != nil {
			return nil, err
		}
		m[step.key] = elem
		return m, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("路径经过的「%s」不是对象", step.key)
	}
	if last {
		m[step.key] = value
		return m, nil
	}
	elem, present := m[step.key]
	elem, err := setAt(elem, present, steps[1:], value)
	if err != nil {
		return nil, err
	}
	m[step.key] = elem
	return m, nil
}

// removeAt removes the value at an existing path and returns the new value.
func removeAt(v any, steps []pathStep) (any, error) {
	step := steps[0]
	last := len(steps) == 1

	if step.isIndex {
		arr, ok := v.([]any)
		if !ok {
			return nil, errors.New("路径经过的不是数组")
		}
		if step.index >= len(arr) {
			return nil, fmt.Errorf("数组下标 %d 超出范围（长度 %d）", step.index, len(arr))
		}
		if last {
			return append(arr[:step.index], arr[step.index+1:]...), nil
		}
		elem, err := removeAt(arr[step.index], steps[1:])
		if err != nil {
			return nil, err
		}
		arr[step.index] = elem
		return arr, nil
	}

	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("路径经过的「%s」不是对象", step.key)
	}
	if last {
		delete(m, step.key)
		return m, nil
	}
	elem, err := removeAt(m[step.key], steps[1:])
	if err != nil {
		return nil, err
	}
	m[step.key] = elem
	return m, nil
}

// lookupPath returns the value at steps and whether the path exists. A stored
// JSON null exists but has a nil value.
func lookupPath(v any, steps []pathStep) (any, bool) {
	for _, step := range steps {
		if step.isIndex {
			arr, ok := v.([]any)
			if !ok || step.index >= len(arr) {
				return nil, false
			}
			v = arr[step.index]
			continue
		}
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok = m[step.key]
		if !ok {
			return nil, false
		}
	}
	return v, true
}

// changedValue formats a value for Change; an absent path is the empty string.
func changedValue(v any, steps []pathStep) (string, error) {
	value, ok := lookupPath(v, steps)
	if !ok {
		return "", nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func cloneJSON(v any) any {
	switch v := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(v))
		for key, elem := range v {
			m[key] = cloneJSON(elem)
		}
		return m
	case []any:
		arr := make([]any, len(v))
		for i, elem := range v {
			arr[i] = cloneJSON(elem)
		}
		return arr
	default:
		return v
	}
}

func opError(path, format string, args ...any) *Error {
	return &Error{Problems: []Problem{{Path: path, Message: fmt.Sprintf(format, args...)}}}
}

func checkError(err error) error {
	var ce *Error
	if errors.As(err, &ce) && ce != nil {
		return ce
	}
	return &Error{Problems: []Problem{{Message: err.Error()}}}
}
