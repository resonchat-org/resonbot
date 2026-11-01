// Copyright (C) 2026 ResonChat contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later
package bot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/resonchat-org/resonbot/internal/peertrans"
	"github.com/resonchat-org/resonbot/internal/translate"
	"mellium.im/xmpp/jid"
	"mellium.im/xmpp/stanza"
)

func (r *Runtime) publishTranslations(in peertrans.Incoming, roomBare jid.JID, stanzaID, originID string) {
	if r.translator == nil || r.translate == nil {
		return
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		return
	}
	parentID := stanzaID
	if parentID == "" {
		parentID = originID
	}
	if parentID == "" {
		parentID = in.ID
	}
	if parentID == "" {
		r.debug.Printf("translate: skip message without id from %s", in.From)
		return
	}

	msg := peertrans.Incoming{
		Message: stanza.Message{
			ID:   in.ID,
			From: in.From,
			Type: stanza.GroupChatMessage,
		},
		Body: body,
	}
	if originID != "" {
		msg.OriginID = &peertrans.OriginID{ID: originID}
	}
	if stanzaID != "" {
		msg.StanzaID = []peertrans.StanzaID{{ID: stanzaID, By: roomBare.Bare().String()}}
	}
	if msg.ParentID(roomBare.Bare()) == "" {
		return
	}

	trCtx, cancel := context.WithTimeout(r.ctx, 120*time.Second)
	defer cancel()

	type hit struct {
		to  string
		res translate.Result
		err error
	}
	langs := r.translate.To
	hits := make([]hit, len(langs))
	var wg sync.WaitGroup
	for i, to := range langs {
		wg.Add(1)
		go func(i int, to string) {
			defer wg.Done()
			res, err := r.translator.Translate(trCtx, body, r.translate.From, to)
			hits[i] = hit{to: to, res: res, err: err}
		}(i, to)
	}
	wg.Wait()

	for _, h := range hits {
		if h.err != nil {
			r.logger.Printf("translate → %s: %v", h.to, h.err)
			continue
		}
		res := h.res
		src := res.SourceLang
		if src == "" {
			src = r.translate.From
		}
		if translate.SameLanguage(src, h.to) || translate.SameLanguage(src, res.DestLang) {
			r.debug.Printf("translate skip %s: source already %s", h.to, src)
			continue
		}
		origin := fmt.Sprintf("pt-%d", time.Now().UnixNano())
		out := peertrans.NewOutgoing(
			roomBare.Bare(),
			msg,
			res.DestLang,
			res.SourceLang,
			res.Text,
			res.Engine,
			r.translate.Priority,
			origin,
		)
		r.logger.Printf("peertrans %s %q → %q (%s)", roomBare.Bare(), body, res.Text, res.DestLang)
		sendCtx, sendCancel := context.WithTimeout(r.ctx, 15*time.Second)
		err := r.encode(sendCtx, out)
		sendCancel()
		if err != nil {
			r.logger.Printf("send peertrans (%s): %v", res.DestLang, err)
		}
	}
}
