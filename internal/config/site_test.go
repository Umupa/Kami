package config

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the site's copy of the configuration prompt")

// sitePrompt is the site's copy of the configuration prompt, which its
// "交给 Agent 安装" button hands on after the steps that install Kami.
const sitePrompt = "../../site/prompt.js"

// TestSitePrompt keeps the site's prompt the one the app gives, for a copy
// installed in /Applications with the default settings; go test
// ./internal/config -update writes it again.
func TestSitePrompt(t *testing.T) {
	c, err := Decode(fmt.Appendf(nil, `{"version":%d}`, Version))
	if err != nil {
		t.Fatal(err)
	}
	prompt := AgentPrompt(PromptInput{
		Exe:    "/Applications/Kami.app/Contents/MacOS/Kami",
		Dir:    "~/Library/Application Support/Kami",
		Config: c,
	})
	text, err := json.Marshal(prompt)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	b.WriteString("// The configuration prompt Kami gives, for a copy in /Applications with the\n")
	b.WriteString("// default settings. Written by `go test ./internal/config -update`; do not edit.\n")
	fmt.Fprintf(&b, "window.KAMI_SETUP_PROMPT = %s;\n", text)

	if *update {
		if err := os.WriteFile(sitePrompt, b.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(sitePrompt)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, b.Bytes()) {
		t.Errorf("%s is out of date; run go test ./internal/config -update", sitePrompt)
	}
}
