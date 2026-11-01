// Copyright (C) 2026 ResonChat contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later
package antispam

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/resonchat-org/resonbot/internal/config"
	"mellium.im/xmpp/jid"
)

// Verdict is what the bot should do after inspecting a message or nick.
type Verdict struct {
	Warn       string   // non-empty → send groupchat warning
	Ban        bool     // set affiliation=outcast
	BanUser    jid.JID  // real bare JID
	BanNick    string   // display nick for announce
	RetractIDs []string // MUC stanza-ids to moderate (if retract enabled)
	Announce   string   // ban announce body (optional)
}

// Detector holds per-room / per-user checker state.
type Detector struct {
	cfg config.AntispamConfig

	mu       sync.Mutex
	checkers map[string]map[string]userCheckers // roomBare → userBare
	recent   map[string]time.Time               // ban dedupe
}

// New creates a detector from config.
func New(cfg config.AntispamConfig) *Detector {
	return &Detector{
		cfg:      cfg,
		checkers: make(map[string]map[string]userCheckers),
		recent:   make(map[string]time.Time),
	}
}

// Covers reports whether roomBare is in the detector's room list (empty = all).
func (d *Detector) Covers(roomBare string) bool {
	if len(d.cfg.Rooms) == 0 {
		return true
	}
	for _, r := range d.cfg.Rooms {
		if strings.EqualFold(strings.TrimSpace(r), roomBare) {
			return true
		}
	}
	return false
}

// BanAllRooms mirrors config.
func (d *Detector) BanAllRooms() bool { return d.cfg.BanAllRooms }

// CleanInterval is how often state should be wiped.
func (d *Detector) CleanInterval() time.Duration {
	sec := d.cfg.CleanCache
	if sec <= 0 {
		sec = 3600
	}
	return time.Duration(sec) * time.Second
}

// Clear drops all per-user checkers (periodic memory hygiene).
func (d *Detector) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.checkers = make(map[string]map[string]userCheckers)
	d.recent = make(map[string]time.Time)
}

// ForgetUser drops state after a successful ban.
func (d *Detector) ForgetUser(roomBare string, userBare jid.JID) {
	if userBare.Equal(jid.JID{}) {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if m := d.checkers[roomBare]; m != nil {
		delete(m, userBare.Bare().String())
	}
	delete(d.recent, userBare.Bare().String())
}

// EnsureUser prepares checkers when an occupant joins. Returns true if newly created.
func (d *Detector) EnsureUser(roomBare string, userBare jid.JID) (created bool) {
	if userBare.Equal(jid.JID{}) {
		return false
	}
	room := roomBare
	user := userBare.Bare().String()
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.checkers[room] == nil {
		d.checkers[room] = make(map[string]userCheckers)
	}
	if _, ok := d.checkers[room][user]; !ok {
		d.checkers[room][user] = newUserCheckers()
		return true
	}
	return false
}

// CheckNick returns a ban verdict when ban_nicks matches.
func (d *Detector) CheckNick(roomBare, nick string, userBare jid.JID) Verdict {
	if userBare.Equal(jid.JID{}) || nick == "" {
		return Verdict{}
	}
	for _, rule := range d.cfg.BanNicks {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		if strings.Contains(nick, rule) || strings.Contains(userBare.Localpart(), rule) {
			return d.banVerdict(nick, userBare, nil)
		}
	}
	return Verdict{}
}

// CheckMessage evaluates body against rate/repeat/meaningless rules.
// The first message from a user only creates state, no penalty.
func (d *Detector) CheckMessage(roomBare, nick, body, sid string, userBare jid.JID) Verdict {
	if userBare.Equal(jid.JID{}) {
		return Verdict{}
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return Verdict{}
	}
	if d.EnsureUser(roomBare, userBare) {
		return Verdict{}
	}

	d.mu.Lock()
	uc := d.checkers[roomBare][userBare.Bare().String()]
	d.mu.Unlock()
	if uc == nil {
		return Verdict{}
	}

	result := uc.onMessage(body, sid, d.cfg)
	if result.isSpam() {
		return d.banVerdict(nick, userBare, result.spamSIDs())
	}
	if result.isSkeptical() {
		warn := result.warning()
		if warn == "" {
			return Verdict{}
		}
		// Tolerate bad printf templates so a config typo cannot panic.
		return Verdict{Warn: safeSprintf(warn, nick)}
	}
	return Verdict{}
}

func (d *Detector) banVerdict(nick string, userBare jid.JID, sids []string) Verdict {
	key := userBare.Bare().String()
	d.mu.Lock()
	dup := false
	if t, ok := d.recent[key]; ok && time.Since(t) < 10*time.Second {
		dup = true
	} else {
		d.recent[key] = time.Now()
	}
	d.mu.Unlock()

	v := Verdict{
		BanUser:    userBare.Bare(),
		BanNick:    nick,
		RetractIDs: append([]string(nil), sids...),
	}
	if dup {
		return v
	}
	v.Ban = true
	if d.cfg.AnnounceBan {
		v.Announce = safeSprintf(d.cfg.Replies.BanLog, nick, userBare.Bare())
	}
	return v
}

func safeSprintf(format string, args ...any) (out string) {
	defer func() {
		if recover() != nil {
			out = format
		}
	}()
	return fmt.Sprintf(format, args...)
}
