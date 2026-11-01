// Copyright (C) 2026 ResonChat contributors.
package peertrans

import (
	"testing"
	"unicode/utf8"
)

func TestBuildReplyFallback(t *testing.T) {
	fb := BuildReplyFallback("Good morning", "早上好")
	wantQuote := "> Good morning\n"
	if fb.End != utf8.RuneCountInString(wantQuote) {
		t.Fatalf("end=%d want %d", fb.End, utf8.RuneCountInString(wantQuote))
	}
	if fb.WireBody != wantQuote+"\n早上好" {
		t.Fatalf("wire=%q", fb.WireBody)
	}
	if got := fb.WireBody[fb.Start:fb.End]; got != wantQuote {
		// Offsets are code points; for ASCII start/end equal byte indices.
		t.Fatalf("quoted region %q", got)
	}
}
