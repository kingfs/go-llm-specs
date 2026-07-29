package aiclient

import (
	"strings"
	"testing"
)

func TestDecodeResponsesIgnoresReasoning(t *testing.T) {
	input := `{"output":[{"type":"reasoning","content":[{"type":"reasoning_text","text":"hidden"}]},{"type":"message","content":[{"type":"output_text","text":"{\"ok\":true}"}]}]}`
	text, err := decodeText(strings.NewReader(input), "responses")
	if err != nil || text != `{"ok":true}` {
		t.Fatalf("text=%q err=%v", text, err)
	}
}

func TestStripFence(t *testing.T) {
	if got := stripFence("```json\n{\"ok\":true}\n```"); got != `{"ok":true}` {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeEscapedJSON(t *testing.T) {
	if got := normalizeJSONText(`{\"claims\":[]}`); got != `{"claims":[]}` {
		t.Fatalf("got %q", got)
	}
	if got := normalizeJSONText(`"{\"claims\":[]}"`); got != `{"claims":[]}` {
		t.Fatalf("got %q", got)
	}
}
