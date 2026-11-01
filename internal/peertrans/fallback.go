// Copyright (C) 2026 ResonChat contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later
package peertrans

import (
	"strings"
	"unicode/utf8"
)

// ReplyFallback matches Conversations / ResonChat reply quote shape:
// each line of the parent prefixed with "> ", then a blank line, then the
// translation. Offsets are Unicode code points (XEP-0461).
type ReplyFallback struct {
	WireBody string
	Start    int
	End      int
}

// BuildReplyFallback builds the destination body and fallback offsets.
func BuildReplyFallback(quote, translation string) ReplyFallback {
	if quote == "" {
		return ReplyFallback{WireBody: translation, Start: 0, End: 0}
	}
	var b strings.Builder
	for _, line := range strings.Split(quote, "\n") {
		b.WriteString("> ")
		b.WriteString(strings.TrimSpace(line))
		b.WriteByte('\n')
	}
	quoted := b.String()
	return ReplyFallback{
		WireBody: quoted + "\n" + translation,
		Start:    0,
		End:      utf8.RuneCountInString(quoted),
	}
}
