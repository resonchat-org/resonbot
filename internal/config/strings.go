// Copyright (C) 2026 ResonChat contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later
package config

import (
	"fmt"
	"strings"
)

// StringList accepts a single TOML string or an array of strings.
type StringList []string

// UnmarshalTOML implements github.com/BurntSushi/toml.Unmarshaler.
func (l *StringList) UnmarshalTOML(data interface{}) error {
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
		*l = StringList{s}
		return nil
	case []interface{}:
		out := make(StringList, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return fmt.Errorf("want string, got %T at index %d", item, i)
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
		return fmt.Errorf("want string or array of strings, got %T", data)
	}
}

// LangList is an alias kept for translate.to clarity.
type LangList = StringList
