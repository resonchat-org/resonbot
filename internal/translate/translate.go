// Copyright (C) 2026 ResonChat contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// Machine translation for peertransbot, aligned with kiss-translator's
// public DeepL API and OpenAI / Anthropic chat shapes (prompts included).

package translate

import (
	"context"
	"fmt"
	"strings"
)

// Result is one translated string plus metadata for peertrans attribution.
type Result struct {
	Text       string
	SourceLang string // BCP-47 primary when known (e.g. "en", "zh")
	DestLang   string // BCP-47 primary requested (e.g. "zh")
	Engine     string // shown in the bubble marker (DeepL / model / …)
}

// Translator turns source text into a destination language.
type Translator interface {
	Translate(ctx context.Context, text, fromLang, toLang string) (Result, error)
	// Name is the configured provider id (deepl, openai, anthropic).
	Name() string
}

// Config selects a provider. Provider is required (deepl|openai|anthropic).
type Config struct {
	Provider string // deepl | openai | anthropic
	APIKey   string
	APIURL   string // optional; provider default when empty
	Model    string // LLM model id
	ToLang   string // default target (zh-CN, en, …); kiss-style codes
	FromLang string // default source; "auto" or empty = detect/omit
	Timeout  int    // HTTP seconds; 0 → 120 (kiss DEFAULT_HTTP_TIMEOUT)
}

// New builds a Translator from Config.
func New(cfg Config) (Translator, error) {
	p := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if p == "" {
		return nil, fmt.Errorf("translate: provider required (deepl|openai|anthropic)")
	}
	to := strings.TrimSpace(cfg.ToLang)
	if to == "" {
		to = "zh-CN"
	}
	from := strings.TrimSpace(cfg.FromLang)
	if from == "" {
		from = "auto"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 120
	}
	base := baseConfig{
		apiKey:   strings.TrimSpace(cfg.APIKey),
		apiURL:   strings.TrimSpace(cfg.APIURL),
		model:    strings.TrimSpace(cfg.Model),
		toLang:   to,
		fromLang: from,
		timeout:  timeout,
	}
	switch p {
	case "deepl":
		if base.apiKey == "" {
			return nil, fmt.Errorf("translate: deepl requires API key")
		}
		return newDeepL(base)
	case "openai":
		if base.apiKey == "" {
			return nil, fmt.Errorf("translate: openai requires API key")
		}
		return newOpenAI(base)
	case "anthropic", "claude":
		if base.apiKey == "" {
			return nil, fmt.Errorf("translate: anthropic requires API key")
		}
		return newAnthropic(base)
	default:
		return nil, fmt.Errorf("translate: unknown provider %q (deepl|openai|anthropic)", cfg.Provider)
	}
}

type baseConfig struct {
	apiKey   string
	apiURL   string
	model    string
	toLang   string
	fromLang string
	timeout  int
}

func (b baseConfig) resolveTo(toLang string) string {
	if strings.TrimSpace(toLang) != "" {
		return toLang
	}
	return b.toLang
}

func (b baseConfig) resolveFrom(fromLang string) string {
	if strings.TrimSpace(fromLang) != "" {
		return fromLang
	}
	return b.fromLang
}
