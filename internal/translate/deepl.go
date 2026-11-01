// Copyright (C) 2026 ResonChat contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later
package translate

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Official DeepL API (kiss genDeepl). Not DeepL Free JSON-RPC / DeepLX.
const defaultDeepLURL = "https://api-free.deepl.com/v2/translate"

type deepLTranslator struct {
	base baseConfig
	url  string
}

func newDeepL(b baseConfig) (*deepLTranslator, error) {
	url := b.apiURL
	if url == "" {
		url = defaultDeepLURL
	}
	return &deepLTranslator{base: b, url: url}, nil
}

func (d *deepLTranslator) Name() string { return "deepl" }

func (d *deepLTranslator) Translate(ctx context.Context, text, fromLang, toLang string) (Result, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Result{}, fmt.Errorf("deepl: empty text")
	}
	to := deeplLang(d.base.resolveTo(toLang))
	if to == "" {
		return Result{}, fmt.Errorf("deepl: missing target language")
	}
	from := deeplLang(d.base.resolveFrom(fromLang))

	body := map[string]any{
		"text":        []string{text},
		"target_lang": to,
	}
	if from != "" {
		body["source_lang"] = from
	}
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "DeepL-Auth-Key " + d.base.apiKey,
	}
	raw, err := doJSON(ctx, "POST", d.url, headers, body, d.base.timeout)
	if err != nil {
		return Result{}, fmt.Errorf("deepl: %w", err)
	}
	var resp struct {
		Translations []struct {
			Text                   string `json:"text"`
			DetectedSourceLanguage string `json:"detected_source_language"`
		} `json:"translations"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return Result{}, fmt.Errorf("deepl: decode: %w", err)
	}
	if len(resp.Translations) == 0 || strings.TrimSpace(resp.Translations[0].Text) == "" {
		return Result{}, fmt.Errorf("deepl: empty translations")
	}
	tr := resp.Translations[0]
	src := deeplDetectedToXMPP(tr.DetectedSourceLanguage)
	if src == "" && from != "" {
		src = xmppLang(fromLang)
	}
	return Result{
		Text:       tr.Text,
		SourceLang: src,
		DestLang:   xmppLang(d.base.resolveTo(toLang)),
		Engine:     "DeepL",
	}, nil
}
