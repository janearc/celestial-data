package web_test

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
	"pgregory.net/rapid"
)

// longestWait is the longest Retry-After the gentle keeps, in seconds. It
// is defined by time.Duration: the most seconds it holds.
const longestWait = math.MaxInt64 / int64(time.Second)

// restUnsaid is how long the example host is left alone when its Retry-After
// says nothing that can be used, as its numbers say.
const restUnsaid = 300 * time.Second

// retryAfterOf is a Retry-After drawn at random, as the host writes it, and
// the time the gentle must leave the host alone until, or none when the
// text is any text at all and no wait is expected of it.
func retryAfterOf(draw *rapid.T) (asked string, want time.Time, known bool) {
	switch rapid.IntRange(0, 3).Draw(draw, "kind") {
	case 0:
		seconds := rapid.Uint64Range(0, math.MaxInt64).Draw(draw, "seconds")
		asked = strconv.FormatUint(seconds, 10)
		wait := int64(min(seconds, uint64(longestWait)))
		return asked, start.Add(time.Duration(wait) * time.Second), true
	case 1:
		asked = rapid.StringMatching(`[1-9][0-9]{20,40}`).Draw(draw, "long")
		return asked, start.Add(restUnsaid), true
	case 2:
		offset := rapid.Int64Range(-1e9, 1e9).Draw(draw, "offset")
		at := start.Add(time.Duration(offset) * time.Second)
		return at.UTC().Format(http.TimeFormat), at, true
	}
	return rapid.String().Draw(draw, "anything"), time.Time{}, false
}

// Property: whatever a Retry-After says, even text that is no number or
// date, the host is stopped; and when it says a number of seconds, however
// large, or a date, the wait is exactly the one it asked, a date already
// past included, and the rest for a number too large to read.
func TestPropertyARetryAfterGivesExactlyTheWaitAsked(t *testing.T) {
	rapid.Check(t, func(draw *rapid.T) {
		gentle, _ := exampleAt(t)
		asked, want, known := retryAfterOf(draw)
		if err := gentle.Enter(context.Background()); err != nil {
			draw.Fatal(err)
		}
		defer gentle.Leave()
		silent, err := gentle.Judge(http.StatusTooManyRequests, asked,
			errors.New("429"))
		var stopped *web.Stopped
		if silent || !errors.As(err, &stopped) {
			draw.Fatalf("%q: %v", asked, err)
		}
		if known && !stopped.Until.Equal(want) {
			draw.Fatalf("%q: until %v, not %v", asked, stopped.Until, want)
		}
	})
}

// Property: however the clock is set, forward or back, every wait for the
// pace is exactly the time the pace has left, never more than the pace and
// never none while any is left: it fails if a wait were not slept.
func TestPropertyEveryWaitForThePaceIsExactlyWhatIsLeftOfIt(t *testing.T) {
	pace := example(t).Pace
	rapid.Check(t, func(draw *rapid.T) {
		gentle, keeping := exampleAt(t)
		offsets := rapid.SliceOfN(rapid.Int64Range(-1e9, 1e9), 1, 8).
			Draw(draw, "offsets in seconds")
		var want []time.Duration
		var last time.Time
		for at, offset := range offsets {
			now := start.Add(time.Duration(offset) * time.Second)
			keeping.set(now)
			wait := min(max(pace-now.Sub(last), 0), pace)
			if at == 0 {
				wait = 0
			}
			if wait > 0 {
				want = append(want, wait)
			}
			if err := gentle.Enter(context.Background()); err != nil {
				draw.Fatal(err)
			}
			gentle.Leave()
			last = now.Add(wait)
		}
		if len(keeping.slept) != len(want) {
			draw.Fatalf("slept %v, wanted %v", keeping.slept, want)
		}
		for at := range want {
			if keeping.slept[at] != want[at] {
				draw.Fatalf("slept %v, wanted %v", keeping.slept, want)
			}
		}
	})
}

// Property: a request that always fails with no word waits, in all, at
// least the pace before each retry and at most the longest backoff and the
// pace for each, whatever numbers the random gives: it fails if the pace
// or the backoff were not slept, or were slept too long.
func TestPropertyTheWaitsOfARetryAreWithinTheTriesTheCapAndThePace(
	t *testing.T) {
	current := 0.0
	random := mocks.NewMockRandom(gomock.NewController(t))
	random.EXPECT().Float64().DoAndReturn(func() float64 {
		return current
	}).AnyTimes()
	rapid.Check(t, func(draw *rapid.T) {
		current = rapid.Float64().Draw(draw, "random")
		policy := example(t)
		clock, keeping := clockAt(t, start)
		gentle, err := web.NewGentle(policy, clock, random)
		if err != nil {
			draw.Fatal(err)
		}
		inner := mocks.NewMockWeb(gomock.NewController(t))
		inner.EXPECT().Get(address).Return(nil, int64(0),
			resetBy).Times(policy.Tries + 1)
		wrapped, _ := web.Within(context.Background(), inner, gentle)
		if _, _, err := wrapped.Get(address); err == nil {
			draw.Fatal("a failure was not one")
		}
		least := time.Duration(policy.Tries) * policy.Pace
		most := time.Duration(policy.Tries) * (policy.BackoffCap +
			policy.Pace)
		if total := keeping.total(); total < least || total > most {
			draw.Fatalf("slept %v, between %v and %v", total, least, most)
		}
	})
}
