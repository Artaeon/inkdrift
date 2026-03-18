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
