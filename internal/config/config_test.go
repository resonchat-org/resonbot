// Copyright (C) 2026 ResonChat contributors.
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsAndValidate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.toml")
	body := `
[xmpp]
jid = "bot@example.org"
password = "secret"

[room]
jid = "room@muc.example"

[translate]
provider = "openai"
api_key = "sk-test"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Room.Nick != "peertransbot" {
		t.Fatalf("nick default: %q", f.Room.Nick)
	}
	if f.Room.Priority != 3 {
		t.Fatalf("priority default: %d", f.Room.Priority)
	}
	if len(f.Translate.To) != 1 || f.Translate.To[0] != "zh-CN" || f.Translate.From != "auto" {
		t.Fatalf("translate defaults: %+v", f.Translate)
	}
}

func TestLoadToList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.toml")
	body := `
[xmpp]
jid = "bot@example.org"
password = "secret"
[room]
jid = "room@muc.example"
[translate]
provider = "openai"
api_key = "sk"
to = ["zh-CN", "ja", "en"]
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Translate.To) != 3 {
		t.Fatalf("%v", f.Translate.To)
	}
}

func TestLoadToSingleString(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.toml")
	body := `
[xmpp]
jid = "bot@example.org"
password = "secret"
[room]
jid = "room@muc.example"
[translate]
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
	if len(f.Translate.To) != 1 || f.Translate.To[0] != "ja" {
		t.Fatalf("%v", f.Translate.To)
	}
}

func TestLoadExplicitPriorityZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.toml")
	body := `
[xmpp]
jid = "bot@example.org"
password = "secret"
[room]
jid = "room@muc.example"
priority = 0
[translate]
provider = "deepl"
api_key = "k"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Room.Priority != 0 {
		t.Fatalf("want explicit 0, got %d", f.Room.Priority)
	}
}

func TestLoadRequiresProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.toml")
	body := `
[xmpp]
jid = "bot@example.org"
password = "secret"
[room]
jid = "room@muc.example"
[translate]
api_key = "k"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error")
	}
}
