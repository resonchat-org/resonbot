// Copyright (C) 2026 ResonChat contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later
package translate

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// OpenAI Chat Completions (kiss buildOpenAIRequest / parseChatResponse).
// Also covers DeepSeek, OpenRouter, Ollama, Gemini OpenAI-compat, etc.
const defaultOpenAIURL = "https://api.openai.com/v1/chat/completions"
const defaultOpenAIModel = "gpt-4o-mini"

type openAITranslator struct {
	base  baseConfig
	url   string
	model string
}

func newOpenAI(b baseConfig) (*openAITranslator, error) {
	url := b.apiURL
	if url == "" {
		url = defaultOpenAIURL
	}
	model := b.model
	if model == "" {
		model = defaultOpenAIModel
	}
	return &openAITranslator{base: b, url: url, model: model}, nil
}

func (o *openAITranslator) Name() string { return "openai" }

func (o *openAITranslator) Translate(ctx context.Context, text, fromLang, toLang string) (Result, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Result{}, fmt.Errorf("openai: empty text")
	}
	toCode := normalizeLang(o.base.resolveTo(toLang))
	fromCode := normalizeLang(o.base.resolveFrom(fromLang))
	user := FillNobatchConcise(text, langName(toCode), toCode, langName(fromCode), fromCode)
	sys := NobatchSystemPrompt()

	messages := make([]map[string]string, 0, 2)
	if strings.TrimSpace(sys) != "" {
		messages = append(messages, map[string]string{"role": "system", "content": sys})
	}
	messages = append(messages, map[string]string{"role": "user", "content": user})

	// max_tokens is the widely supported OpenAI-compat field (DeepSeek, Ollama,
	// OpenRouter, …). Official OpenAI still accepts it.
	body := map[string]any{
		"model":       o.model,
		"messages":    messages,
		"temperature": 0.0,
		"max_tokens":  20480,
		"stream":      false,
	}
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + o.base.apiKey,
	}
	raw, err := doJSON(ctx, "POST", o.url, headers, body, o.base.timeout)
	if err != nil {
		return Result{}, fmt.Errorf("openai: %w", err)
	}
	var resp struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return Result{}, fmt.Errorf("openai: decode: %w", err)
	}
	if resp.Error != nil && resp.Error.Message != "" {
		return Result{}, fmt.Errorf("openai: %s", resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		return Result{}, fmt.Errorf("openai: no choices")
	}
	out := StripLLMFences(resp.Choices[0].Message.Content)
	if out == "" {
		return Result{}, fmt.Errorf("openai: empty content")
	}
	src := xmppLang(fromCode)
	if src == "" || src == "auto" {
		src = detectSourceLang(text)
	}
	engine := o.model
	if engine == "" {
		engine = "OpenAI"
	}
	return Result{
		Text:       out,
		SourceLang: src,
		DestLang:   xmppLang(toCode),
		Engine:     engine,
	}, nil
}
