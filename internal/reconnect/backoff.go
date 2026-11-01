// Copyright (C) 2026 ResonChat contributors.
//
// Conversations XmppConnection.getTimeToNextAttempt (no jitter), mirrored by
// ResonChat app/lib/xmpp/reconnect_backoff.dart.

package reconnect

import (
	"math"
	"time"
)

// Conversations Config timers used by the connection controller.
const (
	ConnectTimeout     = 90 * time.Second
	ConnectDiscoTimeout = 20 * time.Second
	PingMaxInterval    = 300 * time.Second
	PingMinInterval    = 30 * time.Second
	PingTimeout        = 15 * time.Second
)

// State mirrors Conversations Account.State for reconnect decisions.
type State int

const (
	StateOnline State = iota
	StateConnecting
	StateOffline
	StateUnauthorized
	StateIncompatibleServer
	StateTLSError
	StateUnreachable
	StatePolicyViolation
	StateSeeOtherHost
	StateConnectionTimeout
	StateStreamError
	StateBindFailure
)

// AttemptReconnect is false for terminal auth / incompatible-server states.
func AttemptReconnect(s State) bool {
	switch s {
	case StateOnline, StateConnecting, StateUnauthorized, StateIncompatibleServer:
		return false
	default:
		return true
	}
}

// ReconnectImmediately matches Conversations OFFLINE (no backoff wait).
func ReconnectImmediately(s State) bool {
	return s == StateOffline
}

// AggressiveBackoff is SEE_OTHER_HOST (bots have no Jingle RTP).
func AggressiveBackoff(s State) bool {
	return s == StateSeeOtherHost
}

// PolicyViolationBackoff adds Conversations' +3 to the attempt exponent.
func PolicyViolationBackoff(s State) bool {
	return s == StatePolicyViolation
}

// TimeToNextAttempt is Conversations getTimeToNextAttempt.
//
//	non-aggressive: min(25 * 1.3^(additionalTime + attempt), 300) - connectionDuration
//	aggressive:     min(3 * 1.3^attempt, 60) - connectionDuration
//
// A non-positive result means connect now.
func TimeToNextAttempt(attempt int, connectionDuration time.Duration, aggressive, policyViolation bool) time.Duration {
	additional := 0
	if policyViolation {
		additional = 3
	}
	var raw float64
	if aggressive {
		raw = math.Min(3*math.Pow(1.3, float64(attempt)), 60)
	} else {
		raw = math.Min(25*math.Pow(1.3, float64(additional+attempt)), 300)
	}
	wait := raw - connectionDuration.Seconds()
	if wait <= 0 {
		return 0
	}
	return time.Duration(wait * float64(time.Second))
}

// PingIsDue matches ResonChat pingIsDue / Conversations lastPacket clocks.
func PingIsDue(now, lastPacket, lastPing time.Time, interval time.Duration) bool {
	last := latest(lastPacket, lastPing)
	if last.IsZero() {
		return true
	}
	return !now.Before(last.Add(interval))
}

// PingTimedOut is true when a ping/<r/> was sent and nothing arrived in time.
func PingTimedOut(now, lastPacket, lastPing time.Time, timeout time.Duration) bool {
	if lastPing.IsZero() {
		return false
	}
	if !lastPacket.IsZero() && !lastPacket.Before(lastPing) {
		return false
	}
	return !now.Before(lastPing.Add(timeout))
}

func latest(a, b time.Time) time.Time {
	switch {
	case a.IsZero():
		return b
	case b.IsZero():
		return a
	case a.After(b):
		return a
	default:
		return b
	}
}
