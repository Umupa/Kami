// Package config reads, validates and watches config.json, the single source
// of the app's settings. Agents edit the file directly, guided by the JSON
// Schema written next to it; the app reloads it on every change and keeps the
// last valid configuration while the file is broken.
package config

// Version is the only config.json version this build understands.
const Version = 1

// Config is a validated configuration with every default filled in. It is
// shared between goroutines and must not be changed after Decode returns it.
type Config struct {
	Schema          string    `json:"$schema,omitempty"`
	Version         int       `json:"version"`
	MyLanguage      string    `json:"myLanguage,omitempty"`
	ForeignLanguage string    `json:"foreignLanguage,omitempty"`
	DefaultModel    string    `json:"defaultModel,omitempty"`
	Models          []Model   `json:"models,omitempty"`
	Modes           []Mode    `json:"modes,omitempty"`
	Shortcuts       Shortcuts `json:"shortcuts,omitzero"`
	LaunchAtLogin   bool      `json:"launchAtLogin,omitempty"`
}

// Model is one named model service. Its API key is not here: it lives in the
// keychain under the model's name.
type Model struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	Format   Format `json:"format"`
	Model    string `json:"model"`
	// Auth is the header that carries the key; empty picks the format's.
	Auth    Auth              `json:"auth,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	// Body is merged into every request body, level by level; a null value
	// removes the key the app would send.
	Body map[string]any `json:"body,omitempty"`
	// ContextWindow is the model's context size in tokens; zero means
	// unknown, and the conversation is trimmed only when the service says so.
	ContextWindow int `json:"contextWindow,omitempty"`
}

// Format is the wire format of a model service.
type Format string

const (
	FormatChatCompletions   Format = "chat-completions"
	FormatResponses         Format = "responses"
	FormatAnthropicMessages Format = "anthropic-messages"
)

// Auth names the request header that carries the API key.
type Auth string

const (
	AuthBearer  Auth = "bearer"
	AuthXAPIKey Auth = "x-api-key"
	AuthAPIKey  Auth = "api-key"
	AuthNone    Auth = "none"
)

// DefaultAuth is the header a format's services expect.
func (f Format) DefaultAuth() Auth {
	if f == FormatAnthropicMessages {
		return AuthXAPIKey
	}
	return AuthBearer
}

// Mode is one entry of the mode list. An entry whose ID is a built-in mode
// overrides only the fields it sets; any other entry is a custom mode and
// needs a name and a prompt. Package mode resolves the list.
type Mode struct {
	ID     string `json:"id"`
	Name   string `json:"name,omitempty"`
	Prompt string `json:"prompt,omitempty"`
	Input  Input  `json:"input,omitempty"`
	Output Output `json:"output,omitempty"`
	// Model is the name of the mode's default model; empty uses the
	// built-in mode's choice, else defaultModel.
	Model string `json:"model,omitempty"`
}

// Input says how the first message of a session is read.
type Input string

const (
	// InputMaterial is text to work on, never instructions.
	InputMaterial Input = "material"
	// InputInstruction is the user's own question or request.
	InputInstruction Input = "instruction"
)

// Output is the format the mode asks the model to answer in.
type Output string

const (
	// OutputAuto follows the source: Markdown in, Markdown out.
	OutputAuto     Output = "auto"
	OutputMarkdown Output = "markdown"
	OutputPlain    Output = "plain"
)

// Shortcuts are the global shortcuts, in the syntax of ParseShortcut.
type Shortcuts struct {
	Panel     string `json:"panel,omitempty"`
	Selection string `json:"selection,omitempty"`
	Capture   string `json:"capture,omitempty"`
}

// Defaults for fields left out of config.json.
const (
	DefaultMyLanguage      = "简体中文"
	DefaultForeignLanguage = "English"
	DefaultPanelShortcut   = "double-option"
	// Selection and capture use ⌃⌥ until Cida is retired, because macOS
	// delivers a hotkey to every app that registered it.
	DefaultSelectionShortcut = "control+option+a"
	DefaultCaptureShortcut   = "control+option+s"
)

// Model returns the model with the given name.
func (c *Config) Model(name string) (Model, bool) {
	for _, m := range c.Models {
		if m.Name == name {
			return m, true
		}
	}
	return Model{}, false
}

// applyDefaults fills in what the file left out, so that readers of a decoded
// Config never handle empty values themselves.
func (c *Config) applyDefaults() {
	if c.MyLanguage == "" {
		c.MyLanguage = DefaultMyLanguage
	}
	if c.ForeignLanguage == "" {
		c.ForeignLanguage = DefaultForeignLanguage
	}
	if c.DefaultModel == "" && len(c.Models) > 0 {
		c.DefaultModel = c.Models[0].Name
	}
	for i := range c.Models {
		if c.Models[i].Auth == "" {
			c.Models[i].Auth = c.Models[i].Format.DefaultAuth()
		}
	}
	if c.Shortcuts.Panel == "" {
		c.Shortcuts.Panel = DefaultPanelShortcut
	}
	if c.Shortcuts.Selection == "" {
		c.Shortcuts.Selection = DefaultSelectionShortcut
	}
	if c.Shortcuts.Capture == "" {
		c.Shortcuts.Capture = DefaultCaptureShortcut
	}
}
