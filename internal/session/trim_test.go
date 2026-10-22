package session

import (
	"slices"
	"testing"

	"github.com/Umupa/Kami/internal/llm"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"one byte", "a", 1},
		{"three bytes", "abc", 1},
		{"four bytes", "abcd", 2},
		{"ASCII", "hello world", 4},
		{"Han", "你好世界", 4},
		{"Hiragana", "こんにちは", 5},
		{"Katakana", "カタカナ", 4},
		{"Hangul", "한국어", 3},
		{"fullwidth", "ＡＢＣ", 3},
		{"mixed runs", "abc中文def", 4},
		{"CJK splits runs", "中a文b", 4},
		{"fullwidth splits runs", "aＡb", 3},
		{"CJK punctuation", "你好，世界。", 6},
		{"CJK punctuation splits runs", "a，b", 3},
		{"invalid byte", "\xff", 1},
		{"emoji", "😀", 2},
		{"emoji run", "😀😀", 3},
		{"mixed emoji", "a😀", 2},
		{"extension B Han", "𠀀", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EstimateTokens(tt.in); got != tt.want {
				t.Errorf("EstimateTokens(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestTrim(t *testing.T) {
	single := []llm.Message{
		{Role: llm.RoleUser, Content: "source"},
	}
	twoTurns := []llm.Message{
		{Role: llm.RoleUser, Content: "source"},
		{Role: llm.RoleAssistant, Content: "reply"},
		{Role: llm.RoleUser, Content: "last"},
	}
	threeTurns := []llm.Message{
		{Role: llm.RoleUser, Content: "source"},
		{Role: llm.RoleAssistant, Content: "reply"},
		{Role: llm.RoleUser, Content: "one"},
		{Role: llm.RoleAssistant, Content: "one reply"},
		{Role: llm.RoleUser, Content: "last"},
	}
	fourTurns := []llm.Message{
		{Role: llm.RoleUser, Content: "source"},
		{Role: llm.RoleAssistant, Content: "reply"},
		{Role: llm.RoleUser, Content: "one"},
		{Role: llm.RoleAssistant, Content: "one reply"},
		{Role: llm.RoleUser, Content: "two"},
		{Role: llm.RoleAssistant, Content: "two reply"},
		{Role: llm.RoleUser, Content: "last"},
	}
	fiveTurns := []llm.Message{
		{Role: llm.RoleUser, Content: "source"},
		{Role: llm.RoleAssistant, Content: "reply"},
		{Role: llm.RoleUser, Content: "one"},
		{Role: llm.RoleAssistant, Content: "one reply"},
		{Role: llm.RoleUser, Content: "two"},
		{Role: llm.RoleAssistant, Content: "two reply"},
		{Role: llm.RoleUser, Content: "three"},
		{Role: llm.RoleAssistant, Content: "three reply"},
		{Role: llm.RoleUser, Content: "last"},
	}

	tests := []struct {
		name        string
		msgs        []llm.Message
		budget      int
		want        []int
		wantDropped int
	}{
		{
			name:   "single turn is never trimmed",
			msgs:   single,
			budget: 1,
			want:   []int{0},
		},
		{
			name:   "everything fits",
			msgs:   threeTurns,
			budget: 30,
			want:   []int{0, 1, 2, 3, 4},
		},
		{
			name:        "drops the only middle turn",
			msgs:        threeTurns,
			budget:      29,
			want:        []int{0, 1, 4},
			wantDropped: 1,
		},
		{
			name:        "base fits exactly",
			msgs:        threeTurns,
			budget:      18,
			want:        []int{0, 1, 4},
			wantDropped: 1,
		},
		{
			name:        "drops the source reply",
			msgs:        threeTurns,
			budget:      17,
			want:        []int{0, 4},
			wantDropped: 1,
		},
		{
			name:        "keeps the last message over budget",
			msgs:        threeTurns,
			budget:      1,
			want:        []int{0, 4},
			wantDropped: 1,
		},
		{
			name:   "zero budget leaves the conversation alone",
			msgs:   threeTurns,
			budget: 0,
			want:   []int{0, 1, 2, 3, 4},
		},
		{
			name:   "negative budget leaves the conversation alone",
			msgs:   threeTurns,
			budget: -1,
			want:   []int{0, 1, 2, 3, 4},
		},
		{
			name:        "two turns drop only the reply",
			msgs:        twoTurns,
			budget:      17,
			want:        []int{0, 2},
			wantDropped: 0,
		},
		{
			name:        "four turns drop the oldest middle turn",
			msgs:        fourTurns,
			budget:      30,
			want:        []int{0, 1, 4, 5, 6},
			wantDropped: 1,
		},
		{
			name:        "four turns keep only the source pair",
			msgs:        fourTurns,
			budget:      29,
			want:        []int{0, 1, 6},
			wantDropped: 2,
		},
		{
			name:        "four turns drop the source reply too",
			msgs:        fourTurns,
			budget:      17,
			want:        []int{0, 6},
			wantDropped: 2,
		},
		{
			name:        "five turns keep the two latest middle turns",
			msgs:        fiveTurns,
			budget:      44,
			want:        []int{0, 1, 4, 5, 6, 7, 8},
			wantDropped: 1,
		},
		{
			name:        "five turns keep the latest middle turn",
			msgs:        fiveTurns,
			budget:      43,
			want:        []int{0, 1, 6, 7, 8},
			wantDropped: 2,
		},
		{
			name:        "five turns keep only the source pair",
			msgs:        fiveTurns,
			budget:      31,
			want:        []int{0, 1, 8},
			wantDropped: 3,
		},
		{
			name:        "five turns drop the source reply",
			msgs:        fiveTurns,
			budget:      17,
			want:        []int{0, 8},
			wantDropped: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := slices.Clone(tt.msgs)
			got, dropped := Trim(tt.msgs, tt.budget)
			if !slices.Equal(tt.msgs, before) {
				t.Errorf("Trim changed its input: %v, want %v", tt.msgs, before)
			}
			want := make([]llm.Message, len(tt.want))
			for i, index := range tt.want {
				want[i] = tt.msgs[index]
			}
			if !slices.Equal(got, want) {
				t.Errorf("Trim() = %v, want %v", got, want)
			}
			if dropped != tt.wantDropped {
				t.Errorf("dropped = %d, want %d", dropped, tt.wantDropped)
			}
			checkTrimShape(t, got, tt.msgs)
			if tt.budget > 0 && len(got) < len(tt.msgs) && len(got) > 2 {
				if cost := messagesTokens(got); cost > tt.budget {
					t.Errorf("kept messages cost %d tokens, want at most %d", cost, tt.budget)
				}
			}
		})
	}
}

func TestTrimEmpty(t *testing.T) {
	got, dropped := Trim(nil, 10)
	if got != nil || dropped != 0 {
		t.Errorf("Trim(nil) = %v, %d, want nil, 0", got, dropped)
	}
	got, dropped = Trim([]llm.Message{}, 10)
	if len(got) != 0 || dropped != 0 {
		t.Errorf("Trim(empty) = %v, %d, want empty, 0", got, dropped)
	}
}

// checkTrimShape verifies that kept starts with the first message, ends
// with the last one, and alternates except for the documented fallback of
// the source and the request with no reply.
func checkTrimShape(t *testing.T, kept, all []llm.Message) {
	t.Helper()
	if len(kept) == 0 {
		t.Fatal("Trim returned no messages")
	}
	if kept[0] != all[0] {
		t.Errorf("first message = %v, want %v", kept[0], all[0])
	}
	if kept[len(kept)-1] != all[len(all)-1] {
		t.Errorf("last message = %v, want %v", kept[len(kept)-1], all[len(all)-1])
	}
	if len(kept) == 2 && kept[0].Role == llm.RoleUser && kept[1].Role == llm.RoleUser {
		return
	}
	for i, m := range kept {
		want := llm.RoleUser
		if i%2 == 1 {
			want = llm.RoleAssistant
		}
		if m.Role != want {
			t.Fatalf("message %d has role %q, want %q (kept %v)", i, m.Role, want, kept)
		}
	}
}

func messagesTokens(msgs []llm.Message) int {
	total := 0
	for _, m := range msgs {
		total += messageTokens(m)
	}
	return total
}
