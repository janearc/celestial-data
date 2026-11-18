package web_test

import (
	"context"
	"errors"
	"math"
	"os"
	"testing"
	"time"

	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
	"pgregory.net/rapid"
)

// readFile is a file's bytes.
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The wall clock tells the time, and sleeps no longer than it is told.
func TestTheWallClockIsTheTimeAndSleepsToItsEnd(t *testing.T) {
	clock := web.Wall()
	before := time.Now()
	if clock.Now().Before(before) {
		t.Error("the wall clock is behind the wall")
	}
	if err := clock.Sleep(context.Background(), 0); err != nil {
		t.Error(err)
	}
}

// Anti-test: a sleep that is cut off by its context gives the context's
// error and does not wait out the hour it was asked.
func TestTheWallClockSleepEndsWithItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := web.Wall().Sleep(ctx, time.Hour)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("%v", err)
	}
}

// The standard library's random numbers are numbers from zero up to one.
func TestUnixGivesNumbersFromZeroToOne(t *testing.T) {
	random := web.Unix()
	for range 100 {
		if got := random.Float64(); got < 0 || got >= 1 {
			t.Errorf("%v", got)
		}
	}
}

// policyBacking is a policy with only the backoff set.
func policyBacking(base, limit time.Duration) web.Policy {
	return web.Policy{BackoffBase: base, BackoffCap: limit}
}

// The first retry's ceiling is the base, and each one after is twice the
// one before, up to the cap.
func TestTheCeilingDoublesToTheCap(t *testing.T) {
	policy := policyBacking(time.Second, 10*time.Second)
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second,
		8 * time.Second, 10 * time.Second, 10 * time.Second}
	for attempt, wait := range want {
		if got := web.Ceiling(policy, attempt); got != wait {
			t.Errorf("attempt %d: %v, not %v", attempt, got, wait)
		}
	}
}

// Anti-test: a ceiling with no base, a cap under the base, or an attempt
// before the first, is never more than the policy allows, nor negative.
func TestTheCeilingOfAPolicyThatMakesNoSenseIsStillBounded(t *testing.T) {
	if got := web.Ceiling(policyBacking(0, time.Second), 3); got != 0 {
		t.Errorf("no base: %v", got)
	}
	capped := policyBacking(time.Minute, time.Second)
	if got := web.Ceiling(capped, 3); got != time.Second {
		t.Errorf("cap under base: %v", got)
	}
	early := policyBacking(time.Second, time.Minute)
	if got := web.Ceiling(early, -2); got != time.Second {
		t.Errorf("before the first: %v", got)
	}
}

// A jittered wait is the fraction the random gives of the ceiling.
func TestJitterIsTheRandomFractionOfTheCeiling(t *testing.T) {
	got := web.Jitter(randomOf(t, half), 10*time.Second)
	if got != 5*time.Second {
		t.Errorf("%v", got)
	}
}

// Anti-test: no ceiling, a random that is not a number, one below zero and
// one past one all give a wait inside the ceiling.
func TestJitterHoldsToItsBoundsWhateverTheRandomGives(t *testing.T) {
	second := time.Second
	cases := []struct {
		name     string
		fraction float64
		ceiling  time.Duration
		want     time.Duration
	}{
		{"no ceiling", half, 0, 0},
		{"a ceiling below nothing", half, -second, 0},
		{"not a number", math.NaN(), second, 0},
		{"below zero", -4, second, 0},
		{"past one", 7, second, second},
		{"exactly one", 1, math.MaxInt64, math.MaxInt64},
	}
	for _, one := range cases {
		got := web.Jitter(randomOf(t, one.fraction), one.ceiling)
		if got != one.want {
			t.Errorf("%s: %v, not %v", one.name, got, one.want)
		}
	}
}

// Property: a jittered wait is never negative and never over its
// ceiling, whatever the ceiling and whatever number the random gives,
// including numbers that are not fractions at all.
func TestPropertyJitterIsWithinNothingAndTheCeiling(t *testing.T) {
	current := 0.0
	random := mocks.NewMockRandom(gomock.NewController(t))
	random.EXPECT().Float64().DoAndReturn(func() float64 {
		return current
	}).AnyTimes()
	rapid.Check(t, func(draw *rapid.T) {
		ceiling := time.Duration(rapid.Int64().Draw(draw, "ceiling"))
		current = rapid.Float64().Draw(draw, "random")
		got := web.Jitter(random, ceiling)
		if got < 0 || got > max(ceiling, 0) {
			draw.Fatalf("%v of a ceiling of %v", got, ceiling)
		}
	})
}

// Property: a ceiling is never over the cap and never shrinks as the
// attempts go on, and no attempt, however many, overflows it.
func TestPropertyTheCeilingNeverPassesTheCapAndNeverShrinks(t *testing.T) {
	rapid.Check(t, func(draw *rapid.T) {
		base := time.Duration(rapid.Int64Min(1).Draw(draw, "base"))
		limit := time.Duration(rapid.Int64Min(int64(base)).
			Draw(draw, "cap"))
		attempt := rapid.IntRange(0, 5000).Draw(draw, "attempt")
		policy := policyBacking(base, limit)
		got := web.Ceiling(policy, attempt)
		before := web.Ceiling(policy, max(attempt-1, 0))
		if got > limit || got < base || got < before {
			draw.Fatalf("attempt %d: %v of %v to %v", attempt, got,
				base, limit)
		}
	})
}
