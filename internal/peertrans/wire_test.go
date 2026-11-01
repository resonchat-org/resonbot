// Copyright (C) 2026 ResonChat contributors.
package peertrans

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"

	"mellium.im/xmpp/jid"
	"mellium.im/xmpp/stanza"
)

func TestOutgoingMarshal(t *testing.T) {
	room := jid.MustParse("room@muc.example")
	parent := Incoming{
		Message: stanza.Message{
			From: jid.MustParse("room@muc.example/alice"),
			ID:   "a1",
			Type: stanza.GroupChatMessage,
		},
		Body: "Good morning",
		StanzaID: []StanzaID{{
			ID: "S1",
			By: "room@muc.example",
		}},
	}
	out := NewOutgoing(room, parent, "zh", "en", "早上好", "DeepL", 3, "b1")
	var buf bytes.Buffer
	if err := xml.NewEncoder(&buf).Encode(out); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	for _, want := range []string{
		`type="groupchat"`,
		`xml:lang="zh"`,
		`早上好`,
		`xmlns="urn:xmpp:langtrans"`,
		`destination_lang="zh"`,
		`source_lang="en"`,
		`engine="DeepL"`,
		`xmlns="urn:xmpp:reply:0"`,
		`id="S1"`,
		`xmlns="urn:xmpp:fallback:0"`,
		`for="urn:xmpp:reply:0"`,
		`xmlns="urn:resonchat:peertrans:0"`,
		`priority="3"`,
		`xmlns="urn:xmpp:sid:0"`,
		`xmlns="urn:xmpp:hints"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
}

func TestOutgoingMarshalPrint(t *testing.T) {
	room := jid.MustParse("room@muc.example")
	parent := Incoming{
		Message: stanza.Message{
			From: jid.MustParse("room@muc.example/alice"),
			ID:   "a1",
			Type: stanza.GroupChatMessage,
		},
		Body: "Good morning",
		StanzaID: []StanzaID{{
			ID: "S1",
			By: "room@muc.example",
		}},
	}
	out := NewOutgoing(room, parent, "zh", "en", "早上好", "DeepL", 3, "b1")
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	_ = enc.Encode(out)
	t.Log("\n" + buf.String())
}
