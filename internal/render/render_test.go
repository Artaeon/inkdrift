package render

import (
	"html/template"
	"strings"
	"testing"
)

func TestRenderHTML(t *testing.T) {
	tmpl := "<p>Hello {{.SubscriberName}}</p>"
	ctx := Context{SubscriberName: "Alice"}

	result, err := RenderHTML(tmpl, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Hello Alice") {
		t.Errorf("expected 'Hello Alice' in result, got %q", result)
	}
}

func TestRenderHTMLWithContent(t *testing.T) {
	tmpl := "<html>{{.Content}}</html>"
	ctx := Context{Content: template.HTML("<p>Newsletter body</p>")}

	result, err := RenderHTML(tmpl, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "<p>Newsletter body</p>") {
		t.Errorf("expected raw HTML content, got %q", result)
	}
}

func TestRenderHTMLInvalidTemplate(t *testing.T) {
	_, err := RenderHTML("{{.Invalid", Context{})
	if err == nil {
		t.Error("expected error for invalid template")
	}
}

func TestRenderText(t *testing.T) {
	html := "<h1>Title</h1><p>Hello <b>world</b>!</p><p>Second paragraph.</p>"
	text := RenderText(html)

	if !strings.Contains(text, "Title") {
		t.Error("expected title in text")
	}
	if !strings.Contains(text, "Hello world!") {
		t.Error("expected 'Hello world!' in text")
	}
	if strings.Contains(text, "<") {
		t.Error("HTML tags should be stripped")
	}
}

func TestRenderTextLineBreaks(t *testing.T) {
	html := "Line 1<br>Line 2<br/>Line 3"
	text := RenderText(html)

	lines := strings.Split(text, "\n")
	if len(lines) < 3 {
		t.Errorf("expected at least 3 lines, got %d: %q", len(lines), text)
	}
}

func TestRenderTextBrSpace(t *testing.T) {
	html := "Line 1<br />Line 2"
	text := RenderText(html)
	if !strings.Contains(text, "Line 1\nLine 2") {
		t.Errorf("expected br with space to create newline, got %q", text)
	}
}

func TestRenderTextH3(t *testing.T) {
	html := "<h3>Heading</h3><p>Text</p>"
	text := RenderText(html)
	if !strings.Contains(text, "Heading") {
		t.Error("expected h3 content")
	}
}

func TestRenderTextDiv(t *testing.T) {
	html := "<div>Block 1</div><div>Block 2</div>"
	text := RenderText(html)
	if !strings.Contains(text, "Block 1\nBlock 2") {
		t.Errorf("expected divs to create newlines, got %q", text)
	}
}

func TestRenderTextLi(t *testing.T) {
	html := "<ul><li>Item 1</li><li>Item 2</li></ul>"
	text := RenderText(html)
	if !strings.Contains(text, "Item 1\nItem 2") {
		t.Errorf("expected list items to create newlines, got %q", text)
	}
}

func TestRenderTextEmpty(t *testing.T) {
	text := RenderText("")
	if text != "" {
		t.Errorf("expected empty string, got %q", text)
	}
}

func TestRenderTextPlainText(t *testing.T) {
	text := RenderText("No HTML here")
	if text != "No HTML here" {
		t.Errorf("expected plain text through, got %q", text)
	}
}

func TestRenderHTMLAllFields(t *testing.T) {
	tmpl := `{{.SubscriberName}} {{.SubscriberEmail}} {{.UnsubscribeURL}} {{.ListName}} {{.SenderName}} {{.Year}}`
	ctx := Context{
		SubscriberName:  "Alice",
		SubscriberEmail: "alice@example.com",
		UnsubscribeURL:  "https://example.com/unsub",
		ListName:        "News",
		SenderName:      "Admin",
		Year:            2026,
	}

	result, err := RenderHTML(tmpl, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Alice") {
		t.Error("missing subscriber name")
	}
	if !strings.Contains(result, "alice@example.com") {
		t.Error("missing subscriber email")
	}
	if !strings.Contains(result, "https://example.com/unsub") {
		t.Error("missing unsubscribe URL")
	}
	if !strings.Contains(result, "News") {
		t.Error("missing list name")
	}
	if !strings.Contains(result, "2026") {
		t.Error("missing year")
	}
}

func TestRenderHTMLExtraField(t *testing.T) {
	tmpl := `{{index .Extra "key"}}`
	ctx := Context{
		Extra: map[string]string{"key": "value"},
	}

	result, err := RenderHTML(tmpl, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result != "value" {
		t.Errorf("expected 'value', got %q", result)
	}
}

func TestRenderHTMLExecutionError(t *testing.T) {
	// Template that calls a function on a value that will fail at execution time
	tmpl := `{{call .SubscriberName}}`
	ctx := Context{SubscriberName: "Alice"}

	_, err := RenderHTML(tmpl, ctx)
	if err == nil {
		t.Error("expected error for calling non-function")
	}
}
