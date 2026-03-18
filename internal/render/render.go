package render

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
)

type Context struct {
	SubscriberName  string
	SubscriberEmail string
	UnsubscribeURL  string
	WebVersion      string
	ListName        string
	SenderName      string
	Content         template.HTML
	Year            int
	Extra           map[string]string
}

func RenderHTML(tmpl string, ctx Context) (string, error) {
	t, err := template.New("email").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

func RenderText(html string) string {
	text := html
	text = strings.ReplaceAll(text, "<br>", "\n")
	text = strings.ReplaceAll(text, "<br/>", "\n")
	text = strings.ReplaceAll(text, "<br />", "\n")
	text = strings.ReplaceAll(text, "</p>", "\n\n")
	text = strings.ReplaceAll(text, "</div>", "\n")
	text = strings.ReplaceAll(text, "</li>", "\n")
	text = strings.ReplaceAll(text, "</h1>", "\n\n")
	text = strings.ReplaceAll(text, "</h2>", "\n\n")
	text = strings.ReplaceAll(text, "</h3>", "\n\n")

	// Strip remaining HTML tags
	var result strings.Builder
	inTag := false
	for _, r := range text {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}

	// Clean up whitespace
	lines := strings.Split(result.String(), "\n")
	var cleaned []string
	for _, line := range lines {
		cleaned = append(cleaned, strings.TrimSpace(line))
	}

	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}
