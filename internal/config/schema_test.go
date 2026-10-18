package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestSchemaIsValidJSON(t *testing.T) {
	data := Schema()
	if !json.Valid(data) {
		t.Fatalf("Schema() is not valid JSON:\n%s", data)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("unmarshal Schema(): %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("schema type = %v, want object", schema["type"])
	}
}

func TestSchemaCoversStructFields(t *testing.T) {
	schema := loadSchema(t)
	tests := []struct {
		name string
		typ  reflect.Type
		got  map[string]any
	}{
		{"Config", reflect.TypeFor[Config](), schemaObject(t, schema, "properties")},
		{"Model", reflect.TypeFor[Model](), schemaObject(t, schema, "$defs", "model", "properties")},
		{"Mode", reflect.TypeFor[Mode](), schemaObject(t, schema, "$defs", "mode", "properties")},
		{"Shortcuts", reflect.TypeFor[Shortcuts](), schemaObject(t, schema, "properties", "shortcuts", "properties")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := jsonFieldNames(tt.typ)
			got := sortedKeys(tt.got)
			if !slices.Equal(got, want) {
				t.Errorf("schema properties = %v, want struct fields %v", got, want)
			}
		})
	}
}

func TestSchemaEnums(t *testing.T) {
	schema := loadSchema(t)
	got := map[string]any{}
	collectKeyValues(schema, "enum", "", got)

	tests := []struct {
		path   string
		values []string
	}{
		{
			"$defs.model.properties.format.enum",
			[]string{string(FormatChatCompletions), string(FormatResponses), string(FormatAnthropicMessages)},
		},
		{
			"$defs.model.properties.auth.enum",
			[]string{string(AuthBearer), string(AuthXAPIKey), string(AuthAPIKey), string(AuthNone)},
		},
		{
			"$defs.mode.properties.input.enum",
			[]string{string(InputMaterial), string(InputInstruction)},
		},
		{
			"$defs.mode.properties.output.enum",
			[]string{string(OutputAuto), string(OutputMarkdown), string(OutputPlain)},
		},
		{"properties.theme.enum", Themes},
		{"properties.font.enum", Fonts},
		{"properties.background.enum", Backgrounds},
	}
	if len(got) != len(tests) {
		t.Fatalf("schema has %d enums at %v, want %d", len(got), sortedKeys(got), len(tests))
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			raw, ok := got[tt.path]
			if !ok {
				t.Fatalf("schema has no enum at %s", tt.path)
			}
			values, ok := raw.([]any)
			if !ok {
				t.Fatalf("enum at %s is %T, want an array", tt.path, raw)
			}
			var gotValues []string
			for _, v := range values {
				s, ok := v.(string)
				if !ok {
					t.Fatalf("enum value %v is %T, want a string", v, v)
				}
				gotValues = append(gotValues, s)
			}
			if !equalStringSets(gotValues, tt.values) {
				t.Errorf("enum = %v, want %v", gotValues, tt.values)
			}
		})
	}
}

func TestSchemaDefaults(t *testing.T) {
	schema := loadSchema(t)
	got := map[string]any{}
	collectKeyValues(schema, "default", "", got)

	want := map[string]any{
		"properties.myLanguage.default":                     DefaultMyLanguage,
		"properties.foreignLanguage.default":                DefaultForeignLanguage,
		"properties.shortcuts.properties.panel.default":     DefaultPanelShortcut,
		"properties.shortcuts.properties.selection.default": DefaultSelectionShortcut,
		"properties.shortcuts.properties.capture.default":   DefaultCaptureShortcut,
		"properties.launchAtLogin.default":                  false,
		"properties.theme.default":                          "auto",
		"properties.font.default":                           Fonts[0],
		"properties.fontSize.default":                       float64(DefaultFontSize),
		"properties.background.default":                     Backgrounds[0],
	}
	if len(got) != len(want) {
		t.Errorf("schema has %d defaults at %v, want %d", len(got), sortedKeys(got), len(want))
	}
	for path, wantValue := range want {
		gotValue, ok := got[path]
		if !ok {
			t.Errorf("schema has no default at %s", path)
			continue
		}
		if !reflect.DeepEqual(gotValue, wantValue) {
			t.Errorf("default at %s = %#v, want %#v", path, gotValue, wantValue)
		}
	}
}

func TestSchemaExamplesDecode(t *testing.T) {
	schema := loadSchema(t)
	modelExamples := schemaExamples(t, schema, "models")
	if len(modelExamples) == 0 {
		t.Fatal("schema models examples are empty")
	}
	t.Run("models", func(t *testing.T) {
		for i, example := range modelExamples {
			data := marshalConfig(t, map[string]any{
				"version": Version,
				"models":  example,
			})
			if _, err := Decode(data); err != nil {
				t.Errorf("models example %d does not decode: %v\n%s", i, err, data)
			}
		}
	})

	modeExamples := schemaExamples(t, schema, "modes")
	if len(modeExamples) == 0 {
		t.Fatal("schema modes examples are empty")
	}
	models := []any{
		map[string]any{
			"name":     "fast",
			"endpoint": "https://example.com/v1/chat/completions",
			"format":   string(FormatChatCompletions),
			"model":    "example",
		},
		map[string]any{
			"name":     "strong",
			"endpoint": "https://example.com/v1/messages",
			"format":   string(FormatAnthropicMessages),
			"model":    "example",
		},
	}
	t.Run("modes", func(t *testing.T) {
		for i, example := range modeExamples {
			data := marshalConfig(t, map[string]any{
				"version": Version,
				"models":  models,
				"modes":   example,
			})
			if _, err := Decode(data); err != nil {
				t.Errorf("modes example %d does not decode: %v\n%s", i, err, data)
			}
		}
	})
}

func loadSchema(t *testing.T) map[string]any {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal(Schema(), &schema); err != nil {
		t.Fatalf("unmarshal Schema(): %v", err)
	}
	return schema
}

func schemaObject(t *testing.T, root map[string]any, path ...string) map[string]any {
	t.Helper()
	var node any = root
	for i, key := range path {
		obj, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("schema path %s: parent is %T, want an object", strings.Join(path[:i], "."), node)
		}
		child, ok := obj[key]
		if !ok {
			t.Fatalf("schema path %s: missing", strings.Join(path[:i+1], "."))
		}
		node = child
	}
	obj, ok := node.(map[string]any)
	if !ok {
		t.Fatalf("schema path %s: %T, want an object", strings.Join(path, "."), node)
	}
	return obj
}

func schemaExamples(t *testing.T, schema map[string]any, property string) []any {
	t.Helper()
	examples, ok := schemaObject(t, schema, "properties", property)["examples"].([]any)
	if !ok {
		t.Fatalf("schema properties.%s.examples is missing or not an array", property)
	}
	return examples
}

func jsonFieldNames(typ reflect.Type) []string {
	names := make([]string, 0, typ.NumField())
	for field := range typ.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// collectKeyValues records the value under every occurrence of key, keyed by
// its dotted JSON path.
func collectKeyValues(node any, key, path string, out map[string]any) {
	switch n := node.(type) {
	case map[string]any:
		for _, k := range sortedKeys(n) {
			childPath := k
			if path != "" {
				childPath = path + "." + k
			}
			if k == key {
				out[childPath] = n[k]
				continue
			}
			collectKeyValues(n[k], key, childPath, out)
		}
	case []any:
		for i, child := range n {
			collectKeyValues(child, key, fmt.Sprintf("%s[%d]", path, i), out)
		}
	}
}

func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	a = slices.Clone(a)
	b = slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

func marshalConfig(t *testing.T, cfg map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return data
}
