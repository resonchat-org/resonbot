// Copyright (C) 2026 ResonChat contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later
package translate

import (
	"strings"
	"unicode"
)

// Language display names (kiss OPT_LANGS_SPEC_NAME first segment).
var langNames = map[string]string{
	"auto":  "AutoDetect",
	"en":    "English",
	"zh":    "Simplified Chinese",
	"zh-CN": "Simplified Chinese",
	"zh-TW": "Traditional Chinese",
	"ja":    "Japanese",
	"ko":    "Korean",
	"es":    "Spanish",
	"fr":    "French",
	"de":    "German",
	"pt":    "Portuguese",
	"ru":    "Russian",
	"ar":    "Arabic",
	"it":    "Italian",
	"nl":    "Dutch",
	"pl":    "Polish",
	"tr":    "Turkish",
	"vi":    "Vietnamese",
	"th":    "Thai",
	"id":    "Indonesian",
	"uk":    "Ukrainian",
	"sv":    "Swedish",
	"cs":    "Czech",
	"da":    "Danish",
	"fi":    "Finnish",
	"el":    "Greek",
	"he":    "Hebrew",
	"hi":    "Hindi",
	"hu":    "Hungarian",
	"ro":    "Romanian",
	"sk":    "Slovak",
	"bg":    "Bulgarian",
}

// DeepL target/source codes (kiss OPT_LANGS_TO_SPEC for DeepL).
func deeplLang(code string) string {
	c := normalizeLang(code)
	switch c {
	case "auto", "":
		return ""
	case "zh", "zh-CN":
		return "ZH-HANS"
	case "zh-TW":
		return "ZH-HANT"
	default:
		return strings.ToUpper(primaryLang(c))
	}
}

func langName(code string) string {
	c := normalizeLang(code)
	if n, ok := langNames[c]; ok {
		return n
	}
	if n, ok := langNames[primaryLang(c)]; ok {
		return n
	}
	return c
}

func normalizeLang(code string) string {
	c := strings.TrimSpace(code)
	if c == "" {
		return "auto"
	}
	// BCP-47 → kiss-style
	c = strings.ReplaceAll(c, "_", "-")
	parts := strings.SplitN(c, "-", 3)
	if len(parts) == 1 {
		p := strings.ToLower(parts[0])
		if p == "zh" {
			return "zh-CN"
		}
		return p
	}
	p0 := strings.ToLower(parts[0])
	p1 := parts[1]
	if p0 == "zh" {
		switch strings.ToLower(p1) {
		case "tw", "hk", "hant", "mo":
			return "zh-TW"
		default:
			return "zh-CN"
		}
	}
	return p0 + "-" + p1
}

func primaryLang(code string) string {
	c := normalizeLang(code)
	if i := strings.IndexByte(c, '-'); i > 0 {
		return c[:i]
	}
	return c
}

// SameLanguage reports whether two tags share a primary subtag (zh-CN ≈ zh).
// auto / empty never match.
func SameLanguage(a, b string) bool {
	pa := primaryLang(a)
	pb := primaryLang(b)
	if pa == "" || pa == "auto" || pb == "" || pb == "auto" {
		return false
	}
	return pa == pb
}

// xmppLang is a short tag suitable for xml:lang / message_translations.lang.
func xmppLang(code string) string {
	c := normalizeLang(code)
	if c == "auto" || c == "" {
		return ""
	}
	return primaryLang(c)
}

func detectSourceLang(s string) string {
	for _, r := range s {
		if unicode.In(r, unicode.Han) {
			return "zh"
		}
	}
	return "en"
}

func deeplDetectedToXMPP(detected string) string {
	d := strings.ToUpper(strings.TrimSpace(detected))
	switch d {
	case "ZH", "ZH-HANS", "ZH-HANT":
		return "zh"
	case "":
		return ""
	default:
		return strings.ToLower(d)
	}
}
