// Copyright (C) 2026 ResonChat contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// File is the on-disk TOML layout for resonbot.
type File struct {
	XMPP      XMPPConfig      `toml:"xmpp"`
	Rooms     []RoomConfig    `toml:"rooms"`
	Translate TranslateConfig `toml:"translate"`
	Antispam  AntispamConfig  `toml:"antispam"`
	Retract   RetractConfig   `toml:"retract"`
	Log       LogConfig       `toml:"log"`

	// Legacy single-room entry (migrated to Rooms).
	Room *RoomConfig `toml:"room"`
}

type XMPPConfig struct {
	JID      string `toml:"jid"`
	Password string `toml:"password"`
}

type RoomConfig struct {
	JID      string `toml:"jid"`
	Nick     string `toml:"nick"`
	Password string `toml:"password"`
	// Priority is legacy; prefer translate.priority.
	Priority int `toml:"priority"`
}

type TranslateConfig struct {
	Enabled  bool       `toml:"enabled"`
	Provider string     `toml:"provider"`
	APIKey   string     `toml:"api_key"`
	APIURL   string     `toml:"api_url"`
	Model    string     `toml:"model"`
	To       LangList   `toml:"to"`
	From     string     `toml:"from"`
	Priority int        `toml:"priority"`
	Rooms    StringList `toml:"rooms"` // empty = all joined rooms
}

type AntispamConfig struct {
	Enabled bool       `toml:"enabled"`
	Rooms   StringList `toml:"rooms"`

	MaxFreqCount   int        `toml:"max_freq_count"`
	SendInterval   float64    `toml:"send_interval"`
	MaxTypingSpeed float64    `toml:"max_typing_speed"`
	MaxRepeatCount int        `toml:"max_repeat_count"`
	MinEditSpeed   float64    `toml:"min_edit_speed"`
	RepeatRatio    float64    `toml:"repeat_ratio"`
	ResetLimit     float64    `toml:"reset_limit"`
	BanNicks       StringList `toml:"ban_nicks"`
	AnnounceBan    bool       `toml:"announce_ban"`
	BanAllRooms    bool       `toml:"ban_all_rooms"`
	CleanCache     int        `toml:"clean_cache"`

	Replies AntispamReplies `toml:"replies"`
}

type AntispamReplies struct {
	TooFastWarning string `toml:"too_fast_warning"`
	RepeatWarning  string `toml:"repeat_warning"`
	BanLog         string `toml:"ban_log"`
}

// RetractConfig toggles XEP-0425 moderate IQs (mellium moderation).
type RetractConfig struct {
	Enabled bool   `toml:"enabled"`
	Reason  string `toml:"reason"`
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
	if len(f.Rooms) == 0 && f.Room != nil && strings.TrimSpace(f.Room.JID) != "" {
		f.Rooms = []RoomConfig{*f.Room}
	}
	for i := range f.Rooms {
		if strings.TrimSpace(f.Rooms[i].Nick) == "" {
			f.Rooms[i].Nick = "resonbot"
		}
	}

	tr := &f.Translate
	if len(tr.To) == 0 {
		tr.To = LangList{"zh-CN"}
	}
	if strings.TrimSpace(tr.From) == "" {
		tr.From = "auto"
	}
	if !meta.IsDefined("translate", "priority") {
		if f.Room != nil && meta.IsDefined("room", "priority") {
			tr.Priority = f.Room.Priority
		} else if tr.Priority == 0 {
			tr.Priority = 3
		}
	}

	as := &f.Antispam
	if as.MaxFreqCount == 0 {
		as.MaxFreqCount = 3
	}
	if as.SendInterval == 0 {
		as.SendInterval = 0.6
	}
	if as.MaxTypingSpeed == 0 {
		as.MaxTypingSpeed = 8
	}
	if as.MaxRepeatCount == 0 {
		as.MaxRepeatCount = 3
	}
	if as.MinEditSpeed == 0 {
		as.MinEditSpeed = 0.6
	}
	if as.RepeatRatio == 0 {
		as.RepeatRatio = 20
	}
	if as.ResetLimit == 0 {
		as.ResetLimit = 60
	}
	if as.CleanCache == 0 {
		as.CleanCache = 3600
	}
	if !meta.IsDefined("antispam", "announce_ban") {
		as.AnnounceBan = true
	}
	if !meta.IsDefined("antispam", "ban_all_rooms") {
		as.BanAllRooms = true
	}
	if as.Replies.TooFastWarning == "" {
		as.Replies.TooFastWarning = "%v: Please stop sending lots of messages quickly"
	}
	if as.Replies.RepeatWarning == "" {
		as.Replies.RepeatWarning = "%v: Please stop sending repetitive messages"
	}
	if as.Replies.BanLog == "" {
		as.Replies.BanLog = "Spammer %v expelled, whose JID is %v"
	}

	if strings.TrimSpace(f.Retract.Reason) == "" {
		f.Retract.Reason = "Spam"
	}
}

func (f File) validate() error {
	if strings.TrimSpace(f.XMPP.JID) == "" {
		return fmt.Errorf("config: xmpp.jid is required")
	}
	if f.XMPP.Password == "" {
		return fmt.Errorf("config: xmpp.password is required")
	}
	if len(f.Rooms) == 0 {
		return fmt.Errorf("config: at least one [[rooms]] entry is required")
	}
	seenRoom := make(map[string]struct{}, len(f.Rooms))
	for i, r := range f.Rooms {
		if strings.TrimSpace(r.JID) == "" {
			return fmt.Errorf("config: rooms[%d].jid is required", i)
		}
		key := strings.ToLower(strings.TrimSpace(r.JID))
		if _, ok := seenRoom[key]; ok {
			return fmt.Errorf("config: duplicate room %q", r.JID)
		}
		seenRoom[key] = struct{}{}
	}

	if !f.Translate.Enabled && !f.Antispam.Enabled {
		return fmt.Errorf("config: enable at least one of translate / antispam")
	}
	if f.Retract.Enabled && !f.Antispam.Enabled {
		return fmt.Errorf("config: retract.enabled requires antispam.enabled")
	}

	if f.Translate.Enabled {
		tr := f.Translate
		switch strings.ToLower(strings.TrimSpace(tr.Provider)) {
		case "deepl", "openai", "anthropic", "claude":
		default:
			return fmt.Errorf("config: translate.provider must be deepl|openai|anthropic, got %q", tr.Provider)
		}
		if strings.TrimSpace(tr.APIKey) == "" {
			return fmt.Errorf("config: translate.api_key is required")
		}
		if len(tr.To) == 0 {
			return fmt.Errorf("config: translate.to must list at least one language")
		}
		if tr.Priority < 0 || tr.Priority > 9 {
			return fmt.Errorf("config: translate.priority must be 0..9, got %d", tr.Priority)
		}
		seen := make(map[string]struct{}, len(tr.To))
		for i, lang := range tr.To {
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
	}
	if f.Antispam.Enabled {
		as := f.Antispam
		if as.MaxFreqCount < 2 {
			return fmt.Errorf("config: antispam.max_freq_count must be >= 2")
		}
		if as.MaxRepeatCount < 2 {
			return fmt.Errorf("config: antispam.max_repeat_count must be >= 2")
		}
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
