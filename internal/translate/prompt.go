// Copyright (C) 2026 ResonChat contributors.
package translate

import (
	"regexp"
	"strings"
)

// kiss-translator src/config/apiConstants.js + api.js concise nobatch prompts.
const (
	placeTo       = "{{to}}"
	placeFrom     = "{{from}}"
	placeToLang   = "{{toLang}}"
	placeFromLang = "{{fromLang}}"
	placeText     = "{{text}}"
	placeTone     = "{{tone}}"
	placeGlossary = "{{glossary}}"
)

// DefaultNobatchPromptConcise is kiss defaultNobatchPromptConcise (empty).
const DefaultNobatchPromptConcise = ""

// DefaultNobatchUserPromptConcise is kiss defaultNobatchUserPromptConcise.
const DefaultNobatchUserPromptConcise = `{{glossary}}
{{tone}}
Translate the following text into {{to}}. Preserve all HTML-like tags. Output ONLY the translated text without any explanation:
{{text}}`

var multiNewline = regexp.MustCompile(`\n{3,}`)

// FillNobatchConcise builds the user prompt like kiss genUserPrompt for the
// concise nobatch preset (empty glossary/tone lines removed).
func FillNobatchConcise(text, toName, toLang, fromName, fromLang string) string {
	compiled := DefaultNobatchUserPromptConcise
	compiled = renderBlockPlaceholder(compiled, placeGlossary, "")
	compiled = renderBlockPlaceholder(compiled, placeTone, "")
	compiled = strings.ReplaceAll(compiled, placeFrom, fromName)
	compiled = strings.ReplaceAll(compiled, placeTo, toName)
	compiled = strings.ReplaceAll(compiled, placeFromLang, fromLang)
	compiled = strings.ReplaceAll(compiled, placeToLang, toLang)
	compiled = strings.ReplaceAll(compiled, placeText, text)
	return strings.TrimSpace(multiNewline.ReplaceAllString(compiled, "\n\n"))
}

// System prompt for concise nobatch is empty (kiss defaultNobatchPromptConcise).
func NobatchSystemPrompt() string {
	return DefaultNobatchPromptConcise
}

// renderBlockPlaceholder mirrors kiss-translator renderBlockPlaceholder for
// standalone lines: empty content deletes the line; otherwise replaces.
func renderBlockPlaceholder(template, placeholder, blockContent string) string {
	if !strings.Contains(template, placeholder) {
		return template
	}
	esc := regexp.QuoteMeta(placeholder)
	standalone := regexp.MustCompile(`(?m)^[ \t]*` + esc + `[ \t]*(?:\r?\n|$)`)
	if strings.TrimSpace(blockContent) != "" {
		template = standalone.ReplaceAllString(template, strings.TrimSpace(blockContent)+"\n\n")
	} else {
		template = standalone.ReplaceAllString(template, "")
	}
	return strings.ReplaceAll(template, placeholder, strings.TrimSpace(blockContent))
}

// StripLLMFences removes optional markdown code fences around model output.
func StripLLMFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.LastIndex(s, "```"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
