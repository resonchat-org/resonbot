// Copyright (C) 2026 ResonChat contributors.
package config

import (
	"fmt"
	"strings"
)

// LangList accepts either a single TOML string or an array of strings for
// translate.to (multi-target languages).
type LangList []string

// UnmarshalTOML implements github.com/BurntSushi/toml.Unmarshaler.
func (l *LangList) UnmarshalTOML(data interface{}) error {
	switch v := data.(type) {
	case nil:
		*l = nil
		return nil
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			*l = nil
			return nil
		}
		*l = LangList{s}
		return nil
	case []interface{}:
		out := make(LangList, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return fmt.Errorf("translate.to[%d]: want string, got %T", i, item)
			}
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			out = append(out, s)
		}
		*l = out
		return nil
	default:
		return fmt.Errorf("translate.to: want string or array of strings, got %T", data)
	}
}
