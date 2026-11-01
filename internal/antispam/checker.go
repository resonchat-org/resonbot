// Copyright (C) 2026 ResonChat contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later
package antispam

import (
	"time"

	"github.com/resonchat-org/resonbot/internal/config"
)

type checkResult interface {
	isSpam() bool
	spamSIDs() []string
	isSkeptical() bool
	warning() string
}

type passed struct{}

func (passed) isSpam() bool       { return false }
func (passed) spamSIDs() []string { return nil }
func (passed) isSkeptical() bool  { return false }
func (passed) warning() string    { return "" }

type skeptical struct{ msg string }

func (skeptical) isSpam() bool       { return false }
func (skeptical) spamSIDs() []string { return nil }
func (skeptical) isSkeptical() bool  { return true }
func (s skeptical) warning() string  { return s.msg }

type failed struct{ sids []string }

func (failed) isSpam() bool         { return true }
func (f failed) spamSIDs() []string { return f.sids }
func (failed) isSkeptical() bool    { return false }
func (failed) warning() string      { return "" }

type subChecker interface {
	onMessage(body, sid string, cfg config.AntispamConfig) checkResult
}

type meaningless struct{}

func (*meaningless) onMessage(body, sid string, cfg config.AntispamConfig) checkResult {
	if body == "" {
		return passed{}
	}
	charSet := make(map[rune]struct{})
	for _, ch := range body {
		charSet[ch] = struct{}{}
	}
	// Avoid divide-by-zero style edge cases (empty charset).
	if len(charSet) == 0 {
		return passed{}
	}
	if float64(len(body)) > cfg.RepeatRatio*float64(len(charSet)) {
		ids := []string{}
		if sid != "" {
			ids = []string{sid}
		}
		return failed{sids: ids}
	}
	return passed{}
}

type typingSpeed struct {
	msgCount     int
	lastSendTime time.Time
	lastSpamTime time.Time
	sids         []string
}

func (c *typingSpeed) onMessage(body, sid string, cfg config.AntispamConfig) checkResult {
	now := time.Now()
	interval := now.Sub(c.lastSendTime).Seconds() - cfg.SendInterval
	c.lastSendTime = now

	if c.lastSpamTime.Unix() == 0 {
		c.lastSpamTime = now
		c.sids = []string{sid}
		return passed{}
	}

	if interval > 1e-7 {
		speed := float64(len([]rune(body))) / interval
		if speed < cfg.MaxTypingSpeed {
			if c.msgCount == 0 {
				return passed{}
			}
			if now.Sub(c.lastSpamTime).Seconds() > cfg.ResetLimit {
				c.msgCount = 0
				c.sids = nil
			} else {
				c.sids = append(c.sids, sid)
			}
			return passed{}
		}
	}

	c.lastSpamTime = now
	c.msgCount++
	c.sids = append(c.sids, sid)
	if c.msgCount == cfg.MaxFreqCount-1 {
		return skeptical{msg: cfg.Replies.TooFastWarning}
	}
	if c.msgCount == cfg.MaxFreqCount {
		return failed{sids: append([]string(nil), c.sids...)}
	}
	return passed{}
}

type repeatChecker struct {
	msgCount int
	lastMsg  string
	sid      string
	sids     []string
}

func editDistanceGreaterThan(str1, str2 string, limit float64) bool {
	if limit <= 0 {
		return true
	}
	if str1 == str2 {
		return false
	}
	s1 := []rune(str1)
	s2 := []rune(str2)
	if len(s1) == 0 || len(s2) == 0 {
		return limit < 1
	}
	if len(s1) < len(s2) {
		s1, s2 = s2, s1
	}
	unnorm := int(limit * float64(len(s1)))
	if len(s1)-len(s2) > unnorm {
		return true
	}
	v0 := make([]int, len(s2)+1)
	v1 := make([]int, len(s2)+1)
	for i := range v0 {
		v0[i] = i
	}
	for i := range s1 {
		v1[0] = i + 1
		minv1 := v1[0]
		for j := range s2 {
			if s1[i] == s2[j] {
				v1[j+1] = v0[j]
			} else {
				v1[j+1] = 1 + min(v0[j], v1[j], v0[j+1])
			}
			minv1 = min(minv1, v1[j+1])
		}
		if minv1 > unnorm {
			return true
		}
		v0, v1 = v1, v0
	}
	return v0[len(s2)] > unnorm
}

func (c *repeatChecker) onMessage(body, sid string, cfg config.AntispamConfig) checkResult {
	if editDistanceGreaterThan(c.lastMsg, body, cfg.MinEditSpeed) {
		c.lastMsg = body
		c.sid = sid
		c.msgCount = 0
		c.sids = nil
		return passed{}
	}
	if c.msgCount == 0 {
		c.sids = []string{c.sid, sid}
	} else {
		c.sids = append(c.sids, sid)
	}
	c.lastMsg = body
	c.msgCount++
	if c.msgCount == cfg.MaxRepeatCount-1 {
		return skeptical{msg: cfg.Replies.RepeatWarning}
	}
	if c.msgCount == cfg.MaxRepeatCount {
		return failed{sids: append([]string(nil), c.sids...)}
	}
	return passed{}
}

type userCheckers []subChecker

func newUserCheckers() userCheckers {
	return userCheckers{
		&meaningless{},
		&typingSpeed{lastSendTime: time.Now(), lastSpamTime: time.Unix(0, 0)},
		&repeatChecker{},
	}
}

func (u userCheckers) onMessage(body, sid string, cfg config.AntispamConfig) checkResult {
	var final checkResult = passed{}
	for _, c := range u {
		r := c.onMessage(body, sid, cfg)
		if r.isSpam() {
			return r
		}
		if r.isSkeptical() {
			final = r
		}
	}
	return final
}
