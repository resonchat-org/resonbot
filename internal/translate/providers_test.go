// Copyright (C) 2026 ResonChat contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later
package translate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeepLRequest(t *testing.T) {
	var gotBody map[string]any
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = w.Write([]byte(`{"translations":[{"text":"你好","detected_source_language":"EN"}]}`))
	}))
	defer srv.Close()

	tr, err := New(Config{Provider: "deepl", APIKey: "k", APIURL: srv.URL, ToLang: "zh-CN"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := tr.Translate(context.Background(), "hello", "auto", "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if r.Text != "你好" || r.Engine != "DeepL" || r.SourceLang != "en" {
		t.Fatalf("%+v", r)
	}
	if gotAuth != "DeepL-Auth-Key k" {
		t.Fatalf("auth %q", gotAuth)
	}
	if gotBody["target_lang"] != "ZH-HANS" {
		t.Fatalf("body %+v", gotBody)
	}
	if _, ok := gotBody["source_lang"]; ok {
		t.Fatalf("auto must omit source_lang: %+v", gotBody)
	}
}

func TestOpenAIRequestPrompt(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"你好"}}]}`))
	}))
	defer srv.Close()

	tr, err := New(Config{
		Provider: "openai",
		APIKey:   "sk",
		APIURL:   srv.URL,
		Model:    "gpt-test",
		ToLang:   "zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := tr.Translate(context.Background(), "hello", "en", "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if r.Text != "你好" || r.Engine != "gpt-test" {
		t.Fatalf("%+v", r)
	}
	msgs, _ := got["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("concise system is empty → one user msg, got %#v", got["messages"])
	}
	user := msgs[0].(map[string]any)["content"].(string)
	if !strings.Contains(user, "Simplified Chinese") || !strings.HasSuffix(strings.TrimSpace(user), "hello") {
		t.Fatalf("user prompt: %q", user)
	}
	if got["temperature"] != 0.0 {
		t.Fatalf("temperature %+v", got["temperature"])
	}
}

func TestAnthropicRequest(t *testing.T) {
	var gotAuth, gotVer string
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("x-api-key")
		gotVer = r.Header.Get("anthropic-version")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"你好"}]}`))
	}))
	defer srv.Close()

	tr, err := New(Config{
		Provider: "anthropic",
		APIKey:   "ak",
		APIURL:   srv.URL,
		Model:    "claude-test",
		ToLang:   "zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := tr.Translate(context.Background(), "hello", "auto", "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if r.Text != "你好" {
		t.Fatalf("%+v", r)
	}
	if gotAuth != "ak" || gotVer != anthropicVersion {
		t.Fatalf("headers %q %q", gotAuth, gotVer)
	}
	if _, ok := got["system"]; ok {
		t.Fatalf("empty system must be omitted: %+v", got)
	}
}
