// Copyright (C) 2026 ResonChat contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later
package peertrans

import (
	"encoding/xml"

	"mellium.im/xmpp/jid"
	"mellium.im/xmpp/stanza"
)

// Namespaces used on the wire (docs/12-peertrans.md).
const (
	NSPeerTrans = "urn:resonchat:peertrans:0"
	NSLangTrans = "urn:xmpp:langtrans"
	NSReply     = "urn:xmpp:reply:0"
	NSFallback  = "urn:xmpp:fallback:0"
	NSHints     = "urn:xmpp:hints"
	NSXML       = "http://www.w3.org/XML/1998/namespace"
)

// Incoming is enough of a groupchat message to decide whether to translate.
type Incoming struct {
	stanza.Message
	Body      string     `xml:"body"`
	OriginID  *OriginID  `xml:"urn:xmpp:sid:0 origin-id"`
	StanzaID  []StanzaID `xml:"urn:xmpp:sid:0 stanza-id"`
	PeerTrans *struct {
		XMLName xml.Name `xml:"urn:resonchat:peertrans:0 peertrans"`
	} `xml:"urn:resonchat:peertrans:0 peertrans"`
}

// OriginID is XEP-0359 origin-id.
type OriginID struct {
	ID string `xml:"id,attr"`
}

// StanzaID is XEP-0359 stanza-id.
type StanzaID struct {
	ID string `xml:"id,attr"`
	By string `xml:"by,attr"`
}

// ParentID returns the id a peertrans <reply/> should target (MUC stanza-id).
func (m Incoming) ParentID(roomBare jid.JID) string {
	room := roomBare.Bare().String()
	for _, sid := range m.StanzaID {
		if sid.By == room && sid.ID != "" {
			return sid.ID
		}
	}
	if m.OriginID != nil && m.OriginID.ID != "" {
		return m.OriginID.ID
	}
	return m.ID
}

// Outgoing is a plaintext peertrans groupchat stanza.
type Outgoing struct {
	stanza.Message
	Body      Body
	LangTrans LangTrans
	Reply     Reply
	Fallback  Fallback
	PeerTrans Marker
	OriginID  stanza.OriginID
	Store     StoreHint
}

// Body is a destination-language body (may include reply quote fallback text).
type Body struct {
	XMLName xml.Name `xml:"body"`
	Lang    string   `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
	Text    string   `xml:",chardata"`
}

// LangTrans is XEP-0171 Direct Translation marking.
type LangTrans struct {
	XMLName     xml.Name    `xml:"urn:xmpp:langtrans x"`
	Translation Translation `xml:"translation"`
}

// Translation is one XEP-0171 translation hint.
type Translation struct {
	DestinationLang string `xml:"destination_lang,attr"`
	SourceLang      string `xml:"source_lang,attr,omitempty"`
	Engine          string `xml:"engine,attr,omitempty"`
}

// Reply is XEP-0461.
type Reply struct {
	XMLName xml.Name `xml:"urn:xmpp:reply:0 reply"`
	ID      string   `xml:"id,attr"`
	To      string   `xml:"to,attr,omitempty"`
}

// Fallback is XEP-0428 for the reply quote.
type Fallback struct {
	XMLName xml.Name     `xml:"urn:xmpp:fallback:0 fallback"`
	For     string       `xml:"for,attr"`
	Body    FallbackBody `xml:"body"`
}

// FallbackBody carries code-point offsets.
type FallbackBody struct {
	Start int `xml:"start,attr"`
	End   int `xml:"end,attr"`
}

// Marker is <peertrans/>.
type Marker struct {
	XMLName  xml.Name `xml:"urn:resonchat:peertrans:0 peertrans"`
	Priority int      `xml:"priority,attr"`
}

// StoreHint requests archiving (XEP-0334).
type StoreHint struct {
	XMLName xml.Name `xml:"urn:xmpp:hints store"`
}

// NewOutgoing builds a peertrans stanza for [parent].
func NewOutgoing(
	roomBare jid.JID,
	parent Incoming,
	destLang, sourceLang, translation, engine string,
	priority int,
	originID string,
) Outgoing {
	fb := BuildReplyFallback(parent.Body, translation)
	parentID := parent.ParentID(roomBare)
	return Outgoing{
		Message: stanza.Message{
			ID:   originID,
			To:   roomBare.Bare(),
			Type: stanza.GroupChatMessage,
		},
		Body: Body{
			Lang: destLang,
			Text: fb.WireBody,
		},
		LangTrans: LangTrans{
			Translation: Translation{
				DestinationLang: destLang,
				SourceLang:      sourceLang,
				Engine:          engine,
			},
		},
		Reply: Reply{
			ID: parentID,
			To: parent.From.String(),
		},
		Fallback: Fallback{
			For: NSReply,
			Body: FallbackBody{
				Start: fb.Start,
				End:   fb.End,
			},
		},
		PeerTrans: Marker{Priority: priority},
		OriginID:  stanza.OriginID{ID: originID},
		Store:     StoreHint{},
	}
}
