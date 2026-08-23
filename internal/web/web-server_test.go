package web

import (
	"strings"
	"testing"
)

func TestRenderChatMessageWrapsAPIResultWithDiagramButton(t *testing.T) {
	message := "Here are the results.\n\nAPI Result (get_virtual_services):\n```json\n{\n  \"count\": 1\n}\n```"

	out := string(renderChatMessage(message))

	if !strings.Contains(out, `<div class="api-result-block" data-tool="get_virtual_services">`) {
		t.Errorf("expected api-result-block wrapper with tool name, got: %s", out)
	}
	if !strings.Contains(out, "&#34;count&#34;: 1") {
		t.Errorf("expected escaped JSON content preserved, got: %s", out)
	}

	toolbarIdx := strings.Index(out, `api-result-toolbar`)
	downloadIdx := strings.Index(out, `diagram-download-btn`)
	toggleIdx := strings.Index(out, `data-bs-toggle="collapse"`)
	preIdx := strings.Index(out, `<pre class="bg-light p-3 rounded collapse"`)
	if toolbarIdx == -1 || downloadIdx == -1 || toggleIdx == -1 || preIdx == -1 {
		t.Fatalf("expected toolbar, download button, collapse toggle and collapsed pre, got: %s", out)
	}
	if !(toolbarIdx < downloadIdx && downloadIdx < preIdx && toggleIdx < preIdx) {
		t.Errorf("expected toolbar (with download + toggle buttons) to appear before the JSON block, got: %s", out)
	}
}

func TestRenderChatMessageOrdinaryCodeBlockHasNoDiagramButton(t *testing.T) {
	message := "Some notes.\n\n```go\nfmt.Println(\"hi\")\n```"

	out := string(renderChatMessage(message))

	if strings.Contains(out, "diagram-download-btn") {
		t.Errorf("did not expect a diagram button on a non-API-result code block, got: %s", out)
	}
	if strings.Contains(out, "api-result-block") {
		t.Errorf("did not expect api-result-block wrapper on a non-API-result code block, got: %s", out)
	}
}
