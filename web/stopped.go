package web

import (
	"fmt"
	"strings"
	"time"
)

// Stopped is a host that is not being asked: because it asked us to
// stop, because the wait it asked for has not passed, or because it
// failed too often with no word and its circuit is open for this run.
//
// It says which host, why, and until when. An override for one run lets
// a caller ask anyway.
type Stopped struct {
	// Host is the host as a person says it.
	Host string
	// Why is what happened, as the request that was refused says it.
	Why string
	// Until is when the host may be asked again. It is the zero time for
	// an open circuit, which lasts as long as the run.
	Until time.Time
	// RetryAfter is what the host's Retry-After said, as written, or
	// empty when it said none.
	RetryAfter string
	// Ours is true when the stop is our own circuit, not the host's word.
	Ours bool
	// Defaulted is true when the host has no pace of its own picked, and
	// the default was used.
	Defaulted bool
	// Insisting is true when the override was set for the run.
	Insisting bool
	// Tried is true when the override was tried: the request was asked
	// once more, and the host refused it again.
	Tried bool
}

// Error is which host, why, until when, and when the pace is the default,
// says so. It does not name the override: a caller that has one names it
// in its own words. When one is set, it says what the override did, or
// why it did not ask again.
func (stopped *Stopped) Error() string {
	var text strings.Builder
	if stopped.Ours {
		fmt.Fprintf(&text, "we stopped asking %s: %s", stopped.Host,
			stopped.Why)
	} else {
		fmt.Fprintf(&text, "%s asked us to stop: %s", stopped.Host,
			stopped.Why)
	}
	if stopped.RetryAfter != "" {
		fmt.Fprintf(&text, "; its Retry-After is %s",
			stopped.RetryAfter)
	}
	if !stopped.Until.IsZero() {
		fmt.Fprintf(&text, "; it may be asked again at %s",
			stopped.Until.Format(time.RFC3339))
	}
	if stopped.Defaulted {
		text.WriteString("; no pace was picked for this host, so the " +
			"default pace was used")
	}
	text.WriteString(stopped.override())
	return text.String()
}

// override is what an error says of the override that is set: what it
// did, or why it did not ask again; nothing when none is set.
func (stopped *Stopped) override() string {
	switch {
	case stopped.Tried:
		return "; the override was tried, and it refused again"
	case stopped.Insisting:
		return "; the override is set, but this request may not be " +
			"asked again"
	}
	return ""
}
