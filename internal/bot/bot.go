// Copyright (C) 2026 ResonChat contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later
package bot

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/resonchat-org/resonbot/internal/antispam"
	"github.com/resonchat-org/resonbot/internal/config"
	"github.com/resonchat-org/resonbot/internal/peertrans"
	"github.com/resonchat-org/resonbot/internal/translate"
	"mellium.im/xmpp"
	"mellium.im/xmpp/jid"
	"mellium.im/xmpp/moderation"
	"mellium.im/xmpp/muc"
	"mellium.im/xmpp/sm"
	"mellium.im/xmpp/stanza"
)

const (
	adminOpTimeout = 20 * time.Second
	sendTimeout    = 15 * time.Second
)

// Room is a joined MUC.
type Room struct {
	Bare     jid.JID
	Full     jid.JID
	Nick     string
	Password string
}

// Occupant is tracked from muc#user presence.
type Occupant struct {
	RoomNick    string
	RealJID     jid.JID
	IsAdmin     bool
	Affiliation muc.Affiliation
	Role        muc.Role
}

// Options configures optional features for a session.
type Options struct {
	Rooms      []Room
	Translate  *config.TranslateConfig // nil = off
	Antispam   *antispam.Detector      // nil = off
	Retract    config.RetractConfig
	Translator translate.Translator // required when Translate != nil
}

// Runtime is one XMPP session serving one or more rooms.
type Runtime struct {
	ctx        context.Context
	session    *xmpp.Session
	muc        *muc.Client
	sm         *sm.Handler
	logger     *log.Logger
	debug      *log.Logger
	rooms      []Room
	translate  *config.TranslateConfig
	translator translate.Translator
	spam       *antispam.Detector
	retract    config.RetractConfig

	mu        sync.RWMutex
	occupants map[string]Occupant
}

// New wires presence handlers for a session.
func New(
	ctx context.Context,
	sess *xmpp.Session,
	mucClient *muc.Client,
	smHandler *sm.Handler,
	opt Options,
	logger, debug *log.Logger,
) *Runtime {
	r := &Runtime{
		ctx:        ctx,
		session:    sess,
		muc:        mucClient,
		sm:         smHandler,
		logger:     logger,
		debug:      debug,
		rooms:      opt.Rooms,
		translate:  opt.Translate,
		translator: opt.Translator,
		spam:       opt.Antispam,
		retract:    opt.Retract,
		occupants:  make(map[string]Occupant),
	}
	mucClient.HandleUserPresence = r.onUserPresence
	mucClient.HandleUserUnpresence = r.onUserUnpresence
	if r.spam != nil {
		go r.cleanSpamLoop()
	}
	return r
}

func (r *Runtime) cleanSpamLoop() {
	defer r.recoverLogged("antispam clean loop")
	t := time.NewTicker(r.spam.CleanInterval())
	defer t.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-t.C:
			r.spam.Clear()
			r.debug.Printf("antispam: cleared user state caches")
		}
	}
}

func (r *Runtime) recoverLogged(where string) {
	if rec := recover(); rec != nil {
		r.logger.Printf("panic in %s: %v", where, rec)
	}
}

func (r *Runtime) roomByBare(bare string) (Room, bool) {
	for _, room := range r.rooms {
		if strings.EqualFold(room.Bare.String(), bare) {
			return room, true
		}
	}
	return Room{}, false
}

func (r *Runtime) occupant(roomFull jid.JID) (Occupant, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	o, ok := r.occupants[roomFull.String()]
	return o, ok
}

func (r *Runtime) coversTranslate(roomBare string) bool {
	if r.translate == nil {
		return false
	}
	if len(r.translate.Rooms) == 0 {
		return true
	}
	for _, x := range r.translate.Rooms {
		if strings.EqualFold(strings.TrimSpace(x), roomBare) {
			return true
		}
	}
	return false
}

func (r *Runtime) onUserPresence(p stanza.Presence, item muc.Item) {
	defer r.recoverLogged("presence")
	roomBare := p.From.Bare()
	room, ok := r.roomByBare(roomBare.String())
	if !ok {
		return
	}
	nick := p.From.Resourcepart()
	self := nick == room.Nick
	real := item.JID.Bare()
	isAdmin := item.Affiliation == muc.AffiliationOwner || item.Affiliation == muc.AffiliationAdmin
	occ := Occupant{
		RoomNick:    nick,
		RealJID:     real,
		IsAdmin:     isAdmin,
		Affiliation: item.Affiliation,
		Role:        item.Role,
	}
	r.mu.Lock()
	r.occupants[p.From.String()] = occ
	r.mu.Unlock()

	if self || isAdmin || r.spam == nil || !r.spam.Covers(roomBare.String()) {
		return
	}
	// Nick rules only here; rate/repeat state is seeded on first message,
	// so presence must not create checkers early.
	v := r.spam.CheckNick(roomBare.String(), nick, real)
	r.applyVerdict(roomBare, v)
}

func (r *Runtime) onUserUnpresence(p stanza.Presence, _ muc.Item) {
	defer r.recoverLogged("unpresence")
	r.mu.Lock()
	delete(r.occupants, p.From.String())
	r.mu.Unlock()
}

// HandleIncoming runs antispam then translate for a groupchat body.
// It never returns a non-nil error: application failures must not tear down Serve.
func (r *Runtime) HandleIncoming(in peertrans.Incoming) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Printf("panic in message handler: %v", rec)
			err = nil
		}
	}()
	if in.Type != stanza.GroupChatMessage {
		return nil
	}
	roomBare := in.From.Bare()
	room, ok := r.roomByBare(roomBare.String())
	if !ok {
		return nil
	}
	nick := in.From.Resourcepart()
	self := nick == room.Nick
	occ, _ := r.occupant(in.From)

	stanzaID := ""
	for _, sid := range in.StanzaID {
		if sid.By == roomBare.String() && sid.ID != "" {
			stanzaID = sid.ID
			break
		}
	}
	originID := ""
	if in.OriginID != nil {
		originID = in.OriginID.ID
	}
	sid := stanzaID
	if sid == "" {
		sid = originID
	}
	if sid == "" {
		sid = in.ID
	}

	if !self && !occ.IsAdmin && r.spam != nil && r.spam.Covers(roomBare.String()) {
		v := r.spam.CheckMessage(roomBare.String(), nick, in.Body, sid, occ.RealJID)
		r.applyVerdict(roomBare, v)
		if v.Ban {
			return nil
		}
	}

	if !self && in.PeerTrans == nil && r.coversTranslate(roomBare.String()) {
		// Never block the XMPP serve loop on HTTP translation.
		inCopy := in
		go func() {
			defer r.recoverLogged("translate")
			r.publishTranslations(inCopy, roomBare, stanzaID, originID)
		}()
	}
	return nil
}

// swallow every admin/send error so a missing moderator affiliation cannot crash
// the bot.
func (r *Runtime) applyVerdict(roomBare jid.JID, v antispam.Verdict) {
	defer r.recoverLogged("applyVerdict")
	if v.Warn == "" && !v.Ban && len(v.RetractIDs) == 0 && v.Announce == "" {
		return
	}

	if v.Warn != "" {
		if err := r.sendGroupchat(roomBare, v.Warn); err != nil {
			r.logger.Printf("antispam warn: %v", err)
		}
	}

	if v.Ban && !v.BanUser.Equal(jid.JID{}) {
		// Ban this room first, then other covered rooms.
		if err := r.banOutcast(roomBare.Bare(), v.BanUser, "spam"); err != nil {
			r.logger.Printf("antispam ban %s in %s: %v (continuing)", v.BanUser, roomBare.Bare(), err)
		}
		if r.spam != nil && r.spam.BanAllRooms() {
			for _, rm := range r.rooms {
				if rm.Bare.Equal(roomBare.Bare()) || !r.spam.Covers(rm.Bare.String()) {
					continue
				}
				if err := r.banOutcast(rm.Bare, v.BanUser, "spam"); err != nil {
					r.logger.Printf("antispam ban %s in %s: %v (continuing)", v.BanUser, rm.Bare, err)
				}
			}
		}
		if r.spam != nil {
			r.spam.ForgetUser(roomBare.String(), v.BanUser)
		}
	}

	if r.retract.Enabled {
		for _, id := range v.RetractIDs {
			if id == "" {
				continue
			}
			ctx, cancel := context.WithTimeout(r.ctx, adminOpTimeout)
			err := moderation.Request(ctx, r.session, roomBare.Bare(), id, r.retract.Reason)
			cancel()
			if err != nil {
				r.logger.Printf("retract %s: %v (continuing)", id, err)
			}
		}
	}

	if v.Announce != "" {
		if err := r.sendGroupchat(roomBare, v.Announce); err != nil {
			r.logger.Printf("antispam announce: %v", err)
		}
	}
}

func (r *Runtime) encode(ctx context.Context, v interface{}) error {
	if r.sm != nil && r.sm.Enabled() {
		return r.sm.Encode(ctx, r.session, v)
	}
	return r.session.Encode(ctx, v)
}

func (r *Runtime) sendGroupchat(roomBare jid.JID, body string) error {
	ctx, cancel := context.WithTimeout(r.ctx, sendTimeout)
	defer cancel()
	return r.encode(ctx, struct {
		stanza.Message
		Body string `xml:"body"`
	}{
		Message: stanza.Message{
			ID:   randomID(),
			To:   roomBare.Bare(),
			Type: stanza.GroupChatMessage,
		},
		Body: body,
	})
}

func (r *Runtime) banOutcast(roomBare, userBare jid.JID, reason string) error {
	ch, ok := r.muc.GetManagedChannel(roomBare.Bare())
	if !ok {
		return fmt.Errorf("no channel for %s", roomBare.Bare())
	}
	ctx, cancel := context.WithTimeout(r.ctx, adminOpTimeout)
	defer cancel()
	return ch.SetAffiliation(ctx, muc.AffiliationOutcast, userBare.Bare(), "", reason)
}

// JoinAll joins every configured room. A single room failure is logged; others
// still join.
func (r *Runtime) JoinAll(ctx context.Context) error {
	var joined int
	var lastErr error
	for _, room := range r.rooms {
		opts := []muc.Option{muc.MaxBytes(0)}
		if room.Password != "" {
			opts = append(opts, muc.Password(room.Password))
		}
		r.logger.Printf("joining %s as %s…", room.Bare, room.Nick)
		if _, err := r.muc.Join(ctx, room.Full, r.session, opts...); err != nil {
			r.logger.Printf("join %s failed: %v (continuing)", room.Bare, err)
			lastErr = err
			continue
		}
		joined++
	}
	if joined == 0 {
		if lastErr != nil {
			return fmt.Errorf("join: no rooms joined: %w", lastErr)
		}
		return fmt.Errorf("join: no rooms configured")
	}
	return nil
}

func randomID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%x", b[:])
}
