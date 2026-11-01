// Copyright (C) 2026 ResonChat contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later

package reconnect

import (
	"math"
	"testing"
	"time"
)

func TestTimeToNextAttemptNonAggressive(t *testing.T) {
	// attempt 0, no duration: 25s
	got := TimeToNextAttempt(0, 0, false, false)
	if math.Abs(got.Seconds()-25) > 0.01 {
		t.Fatalf("attempt0: got %v want ~25s", got)
	}
	// capped at 300
	got = TimeToNextAttempt(40, 0, false, false)
	if got != 300*time.Second {
		t.Fatalf("cap: got %v want 300s", got)
	}
	// connection duration subtracts
	got = TimeToNextAttempt(0, 10*time.Second, false, false)
	if math.Abs(got.Seconds()-15) > 0.01 {
		t.Fatalf("duration: got %v want ~15s", got)
	}
	// already due
	got = TimeToNextAttempt(0, 30*time.Second, false, false)
	if got != 0 {
		t.Fatalf("due: got %v want 0", got)
	}
}

func TestTimeToNextAttemptAggressive(t *testing.T) {
	got := TimeToNextAttempt(0, 0, true, false)
	if math.Abs(got.Seconds()-3) > 0.01 {
		t.Fatalf("aggressive0: got %v want ~3s", got)
	}
	got = TimeToNextAttempt(40, 0, true, false)
	if got != 60*time.Second {
		t.Fatalf("aggressive cap: got %v want 60s", got)
	}
}

func TestTimeToNextAttemptPolicyViolation(t *testing.T) {
	// additionalTime=3 → 25 * 1.3^3
	want := 25 * math.Pow(1.3, 3)
	got := TimeToNextAttempt(0, 0, false, true)
	if math.Abs(got.Seconds()-want) > 0.01 {
		t.Fatalf("policy: got %v want ~%vs", got, want)
	}
}

func TestAttemptReconnect(t *testing.T) {
	if AttemptReconnect(StateUnauthorized) {
		t.Fatal("unauthorized must not reconnect")
	}
	if !AttemptReconnect(StateOffline) {
		t.Fatal("offline must reconnect")
	}
	if !ReconnectImmediately(StateOffline) {
		t.Fatal("offline is immediate")
	}
}

func TestPingClocks(t *testing.T) {
	now := time.Unix(1000, 0)
	if !PingIsDue(now, time.Time{}, time.Time{}, PingMinInterval) {
		t.Fatal("no activity → due")
	}
	last := now.Add(-10 * time.Second)
	if PingIsDue(now, last, time.Time{}, PingMinInterval) {
		t.Fatal("recent packet → not due")
	}
	sent := now.Add(-20 * time.Second)
	if !PingTimedOut(now, time.Time{}, sent, PingTimeout) {
		t.Fatal("no packet after ping → timeout")
	}
	if PingTimedOut(now, now.Add(-1*time.Second), sent, PingTimeout) {
		t.Fatal("packet after ping → ok")
	}
}
