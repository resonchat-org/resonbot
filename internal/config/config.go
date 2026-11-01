// Copyright (C) 2026 ResonChat contributors.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// File is the on-disk TOML layout for peertransbot.
type File struct {
	XMPP      XMPPConfig      `toml:"xmpp"`
	Room      RoomConfig      `toml:"room"`
	Translate TranslateConfig `toml:"translate"`
	Log       LogConfig       `toml:"log"`
}

type XMPPConfig struct {
	JID      string `toml:"jid"`
	Password string `toml:"password"`
}

type RoomConfig struct {
	JID      string `toml:"jid"`
	Nick     string `toml:"nick"`
	Password string `toml:"password"`
	Priority int    `toml:"priority"`
}

type TranslateConfig struct {
	Provider string   `toml:"provider"` // deepl | openai | anthropic
	APIKey   string   `toml:"api_key"`
	APIURL   string   `toml:"api_url"`
	Model    string   `toml:"model"`
	To       LangList `toml:"to"` // one language or several targets
	From     string   `toml:"from"`
}

type LogConfig struct {
	Verbose bool `toml:"verbose"`
	XML     bool `toml:"xml"`
}

// Load reads and validates a TOML config file.
func Load(path string) (File, error) {
	var f File
	meta, err := toml.DecodeFile(path, &f)
	if err != nil {
		return File{}, err
	}
	f.applyDefaults(meta)
	if err := f.validate(); err != nil {
		return File{}, err
	}
	return f, nil
}

func (f *File) applyDefaults(meta toml.MetaData) {
	if strings.TrimSpace(f.Room.Nick) == "" {
		f.Room.Nick = "peertransbot"
	}
	if !meta.IsDefined("room", "priority") {
		f.Room.Priority = 3
	}
	if len(f.Translate.To) == 0 {
		f.Translate.To = LangList{"zh-CN"}
	}
	if strings.TrimSpace(f.Translate.From) == "" {
		f.Translate.From = "auto"
	}
}

func (f File) validate() error {
	if strings.TrimSpace(f.XMPP.JID) == "" {
		return fmt.Errorf("config: xmpp.jid is required")
	}
	if f.XMPP.Password == "" {
		return fmt.Errorf("config: xmpp.password is required")
	}
	if strings.TrimSpace(f.Room.JID) == "" {
		return fmt.Errorf("config: room.jid is required")
	}
	if f.Room.Priority < 0 || f.Room.Priority > 9 {
		return fmt.Errorf("config: room.priority must be 0..9, got %d", f.Room.Priority)
	}
	switch strings.ToLower(strings.TrimSpace(f.Translate.Provider)) {
	case "deepl", "openai", "anthropic", "claude":
	default:
		return fmt.Errorf("config: translate.provider must be deepl|openai|anthropic, got %q", f.Translate.Provider)
	}
	if strings.TrimSpace(f.Translate.APIKey) == "" {
		return fmt.Errorf("config: translate.api_key is required")
	}
	if len(f.Translate.To) == 0 {
		return fmt.Errorf("config: translate.to must list at least one language")
	}
	seen := make(map[string]struct{}, len(f.Translate.To))
	for i, lang := range f.Translate.To {
		lang = strings.TrimSpace(lang)
		if lang == "" {
			return fmt.Errorf("config: translate.to[%d] is empty", i)
		}
		key := strings.ToLower(lang)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("config: translate.to duplicate %q", lang)
		}
		seen[key] = struct{}{}
		f.Translate.To[i] = lang
	}
	return nil
}

// MustExist reports a clearer error when the path is missing.
func MustExist(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return fmt.Errorf("config: %s is a directory", path)
	}
	return nil
}
