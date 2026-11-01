// Copyright (C) 2026 ResonChat contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later
package translate

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Anthropic Messages API (kiss genClaude / claudeResponseText).
const defaultAnthropicURL = "https://api.anthropic.com/v1/messages"
const defaultAnthropicModel = "claude-3-5-haiku-latest"
const anthropicVersion = "2023-06-01"

type anthropicTranslator struct {
	base  baseConfig
	url   string
	model string
}

func newAnthropic(b baseConfig) (*anthropicTranslator, error) {
	url := b.apiURL
	if url == "" {
		url = defaultAnthropicURL
	}
	model := b.model
	if model == "" {
		model = defaultAnthropicModel
	}
	return &anthropicTranslator{base: b, url: url, model: model}, nil
}

func (a *anthropicTranslator) Name() string { return "anthropic" }

func (a *anthropicTranslator) Translate(ctx context.Context, text, fromLang, toLang string) (Result, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Result{}, fmt.Errorf("anthropic: empty text")
	}
	toCode := normalizeLang(a.base.resolveTo(toLang))
	fromCode := normalizeLang(a.base.resolveFrom(fromLang))
	user := FillNobatchConcise(text, langName(toCode), toCode, langName(fromCode), fromCode)
	sys := NobatchSystemPrompt()

	body := map[string]any{
		"model":       a.model,
		"messages":    []map[string]string{{"role": "user", "content": user}},
		"temperature": 0.0,
		"max_tokens":  20480,
		"stream":      false,
	}
	if strings.TrimSpace(sys) != "" {
		body["system"] = sys
	}
	headers := map[string]string{
		"Content-Type":      "application/json",
		"anthropic-version": anthropicVersion,
		"x-api-key":         a.base.apiKey,
	}
	raw, err := doJSON(ctx, "POST", a.url, headers, body, a.base.timeout)
	if err != nil {
		return Result{}, fmt.Errorf("anthropic: %w", err)
	}
	var resp struct {
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return Result{}, fmt.Errorf("anthropic: decode: %w", err)
	}
	if resp.Error != nil && resp.Error.Message != "" {
		return Result{}, fmt.Errorf("anthropic: %s", resp.Error.Message)
	}
	var out string
	for _, b := range resp.Content {
		if b.Type == "text" || b.Text != "" {
			if b.Text != "" {
				out = b.Text
				break
			}
		}
	}
	out = StripLLMFences(out)
	if out == "" {
		return Result{}, fmt.Errorf("anthropic: empty content")
	}
	src := xmppLang(fromCode)
	if src == "" || src == "auto" {
		src = detectSourceLang(text)
	}
	engine := a.model
	if engine == "" {
		engine = "Claude"
	}
	return Result{
		Text:       out,
		SourceLang: src,
		DestLang:   xmppLang(toCode),
		Engine:     engine,
	}, nil
}
