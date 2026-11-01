// Copyright (C) 2026 ResonChat contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later
package translate

import (
	"strings"
	"testing"
)

func TestFillNobatchConciseAlignsKiss(t *testing.T) {
	got := FillNobatchConcise("Hello", "Simplified Chinese", "zh-CN", "English", "en")
	if strings.Contains(got, "{{") {
		t.Fatalf("leftover placeholders: %q", got)
	}
	if !strings.Contains(got, "Simplified Chinese") {
		t.Fatalf("missing target name: %q", got)
	}
	if !strings.HasSuffix(got, "Hello") {
		t.Fatalf("text not at end: %q", got)
	}
	if strings.Contains(got, "Glossary") || strings.Contains(got, "Tone:") {
		t.Fatalf("empty glossary/tone should be stripped: %q", got)
	}
	wantPrefix := "Translate the following text into Simplified Chinese."
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("prefix:\n got %q\nwant %q…", got, wantPrefix)
	}
}

func TestSameLanguage(t *testing.T) {
	if !SameLanguage("zh-CN", "zh") || !SameLanguage("en-US", "en") {
		t.Fatal("expected same primary")
	}
	if SameLanguage("auto", "zh") || SameLanguage("ja", "en") {
		t.Fatal("expected different / auto")
	}
}

func TestDeepLLang(t *testing.T) {
	if deeplLang("zh-CN") != "ZH-HANS" {
		t.Fatal(deeplLang("zh-CN"))
	}
	if deeplLang("zh-TW") != "ZH-HANT" {
		t.Fatal(deeplLang("zh-TW"))
	}
	if deeplLang("en") != "EN" {
		t.Fatal(deeplLang("en"))
	}
	if deeplLang("auto") != "" {
		t.Fatal("auto should omit source")
	}
}

func TestStripLLMFences(t *testing.T) {
	in := "```\n你好\n```"
	if StripLLMFences(in) != "你好" {
		t.Fatal(StripLLMFences(in))
	}
}

func TestProviderRequired(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected error for empty provider")
	}
}
