package web

import (
	"context"
	"math"
	"math/rand/v2"
	"time"
)

// Clock is the time a gentle keeps. Its Sleep ends early when the
// context ends, and gives the context's error.
type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, wait time.Duration) error
}

// wall is the clock on the wall.
type wall struct{}

// Wall is the clock on the wall.
func Wall() Clock { return wall{} }

// Now is the time now.
func (wall) Now() time.Time { return time.Now() }

// Sleep waits so long, or until the context ends.
func (wall) Sleep(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Random is where jitter comes from. Float64 gives a number from zero up
// to but not including one. A test gives it numbers it chose, so no test
// waits on chance.
type Random interface {
	Float64() float64
}

// unix is the random numbers the standard library makes.
type unix struct{}

// Unix is the random numbers the standard library makes, for a run that
// is not a test.
func Unix() Random { return unix{} }

// Float64 is a random number from zero up to but not including one.
func (unix) Float64() float64 { return rand.Float64() }

// Ceiling is the longest the wait before a retry may be: the policy's
// backoff base for the first retry, doubled for each one after, and
// never more than the policy's backoff cap. It does not overflow however
// many retries are asked for.
func Ceiling(policy Policy, attempt int) time.Duration {
	ceiling := min(policy.BackoffBase, policy.BackoffCap)
	if ceiling <= noWait {
		return noWait
	}
	for range max(attempt, 0) {
		// doubling a ceiling past half the cap would pass the cap,
		// and might overflow
		if ceiling > policy.BackoffCap/doubling {
			return policy.BackoffCap
		}
		ceiling *= doubling
	}
	return ceiling
}

// doubling is how much longer each retry's ceiling is than the one
// before. It's defined: exponential backoff doubles.
const doubling = 2

// noWait is a wait with nothing left of it. It's defined: none.
const noWait time.Duration = 0

// Jitter is a wait picked at random from none up to the ceiling, every
// wait as likely as another ("full jitter"), so many callers that failed
// together do not all come back together. It is never negative and never
// more than the ceiling, whatever the random source gives.
func Jitter(random Random, ceiling time.Duration) time.Duration {
	if ceiling <= noWait {
		return noWait
	}
	fraction := random.Float64()
	if math.IsNaN(fraction) {
		return noWait
	}
	// a fraction outside zero to one is clamped before it is
	// multiplied, so no conversion to a duration overflows
	fraction = max(0, min(fraction, 1))
	// the whole ceiling as a float can be one more than a duration holds
	if fraction == 1 {
		return ceiling
	}
	return min(time.Duration(fraction*float64(ceiling)), ceiling)
}
