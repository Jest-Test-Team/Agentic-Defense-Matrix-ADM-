// Package battle defines the canonical event schema and emitter shared by the
// red, blue and green teams. Every attack, defense decision and remediation
// action is expressed as an Event and shipped to the analysis engine (durable
// log) and, best-effort, to a Redis stream for realtime fan-out.
package battle

import (
	"time"
)

// Team identifies which side emitted an event.
type Team string

const (
	TeamRed   Team = "red"
	TeamBlue  Team = "blue"
	TeamGreen Team = "green"
)

// Kind is the category of action an event represents.
type Kind string

const (
	KindAttack      Kind = "attack"
	KindDefense     Kind = "defense"
	KindRemediation Kind = "remediation"
)

// Outcome is the result of an action. Not all outcomes are valid for all kinds;
// the analysis engine treats them as free-form but these are the known values.
const (
	OutcomeBlocked   = "blocked"   // blue: rejected at the boundary
	OutcomeAllowed   = "allowed"   // red: request landed (potential success)
	OutcomeDetected  = "detected"  // blue: SIEM flagged (even if it landed)
	OutcomeRevoked   = "revoked"   // green: session revoked
	OutcomeKilled    = "killed"    // green: agent container killed
	OutcomeRestarted = "restarted" // green: agent container restarted
	OutcomeError     = "error"     // transport/other error
)

// Event is the canonical battle event. session_id is the join key that ties an
// attack to the defense and remediation it triggers.
type Event struct {
	ID        string            `json:"id"`
	TS        time.Time         `json:"ts"`
	Team      Team              `json:"team"`
	Kind      Kind              `json:"kind"`
	Technique string            `json:"technique"`
	Variant   string            `json:"variant,omitempty"`
	SessionID string            `json:"session_id"`
	Target    string            `json:"target,omitempty"`
	Outcome   string            `json:"outcome"`
	Severity  int               `json:"severity"`
	LatencyMS int64             `json:"latency_ms"`
	Detail    string            `json:"detail,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}
