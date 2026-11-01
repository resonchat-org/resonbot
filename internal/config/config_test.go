// Copyright (C) 2026 ResonChat contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMultiRoomFeatures(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.toml")
	body := `
[xmpp]
jid = "bot@example.org"
password = "secret"

[[rooms]]
jid = "a@muc.example"
[[rooms]]
jid = "b@muc.example"

[translate]
enabled = true
provider = "openai"
api_key = "sk"
to = ["zh-CN", "ja"]

[antispam]
enabled = true

[retract]
enabled = true
reason = "Spam"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Rooms) != 2 || f.Rooms[0].Nick != "resonbot" {
		t.Fatalf("%+v", f.Rooms)
	}
	if !f.Translate.Enabled || !f.Antispam.Enabled || !f.Retract.Enabled {
		t.Fatal("feature flags")
	}
	if len(f.Translate.To) != 2 {
		t.Fatalf("%v", f.Translate.To)
	}
}

func TestLoadLegacyRoom(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.toml")
	body := `
[xmpp]
jid = "bot@example.org"
password = "secret"
[room]
jid = "room@muc.example"
priority = 2
[translate]
enabled = true
provider = "deepl"
api_key = "k"
to = "ja"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Rooms) != 1 || f.Translate.Priority != 2 || f.Translate.To[0] != "ja" {
		t.Fatalf("%+v %+v", f.Rooms, f.Translate)
	}
}

func TestRetractNeedsAntispam(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.toml")
	body := `
[xmpp]
jid = "bot@example.org"
password = "secret"
[[rooms]]
jid = "room@muc.example"
[translate]
enabled = true
provider = "openai"
api_key = "k"
[retract]
enabled = true
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error")
	}
}
