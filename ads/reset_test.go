package ads_test

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/janearc/celestial-data/ads"
	"github.com/janearc/celestial-data/web"
)

// threeHours is how far ahead the resets here are. It's defined: any
// time past the pace and under a day will do.
const threeHours = 3 * time.Hour

// unixAt is a time as X-RateLimit-Reset writes it: seconds since 1970.
func unixAt(at time.Time) string { return strconv.FormatInt(at.Unix(), 10) }

// resetSearch is a search through the gentle, with the reset read, on
// the doer given, and the clock's gentle.
func resetSearch(t *testing.T, gentle *web.Gentle, doer ads.Doer,
	clock web.Clock) func() (*ads.Found, error) {
	t.Helper()
	return gated(t, gentle, ads.Resetting(doer, clock))
}

// the test: a 429 with a reset and no Retry-After is kept until the
// reset, in the record too, so a later run asks nothing before it.
func TestA429WithAResetIsKeptUntilTheReset(t *testing.T) {
	record := filepath.Join(t.TempDir(), "ads-gate")
	reset := start.Add(threeHours)
	first, _ := gentleAt(t)
	if err := first.Hold(record); err != nil {
		t.Fatal(err)
	}
	clock, _ := clockAt(t)
	doer := replies(t, answering(t, http.StatusTooManyRequests,
		"X-RateLimit-Reset", unixAt(reset)))
	_, err := resetSearch(t, first, doer, clock)()
	stopped := stopOf(err)
	if stopped == nil || !stopped.Until.Equal(reset) {
		t.Fatalf("%v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	next, _ := gentleAt(t)
	if err := next.Hold(record); err != nil {
		t.Fatal(err)
	}
	_, again := resetSearch(t, next, replies(t), clock)()
	if stop := stopOf(again); stop == nil || !stop.Until.Equal(reset) {
		t.Errorf("the next run: %v", again)
	}
}

// the anti-test: a 429 with both keeps the server's own Retry-After.
func TestA429WithBothKeepsTheRetryAfter(t *testing.T) {
	gentle, _ := gentleAt(t)
	clock, _ := clockAt(t)
	doer := replies(t, answering(t, http.StatusTooManyRequests,
		"Retry-After", "120", "X-RateLimit-Reset",
		unixAt(start.Add(threeHours))))
	_, err := resetSearch(t, gentle, doer, clock)()
	stopped := stopOf(err)
	if stopped == nil || stopped.RetryAfter != "120" ||
		!stopped.Until.Equal(start.Add(twoMinutes)) {
		t.Errorf("%v", err)
	}
}

// the anti-test: a reset that cannot be read, or is not ahead, leaves
// the answer as it came, and ADS rests the default.
func TestAResetThatIsNoUseRestsTheDefault(t *testing.T) {
	for says, reset := range map[string]string{
		"unreadable": "tomorrow", "in the past": unixAt(start.Add(-time.Hour)),
		"now": unixAt(start), "empty": ""} {
		gentle, _ := gentleAt(t)
		clock, _ := clockAt(t)
		doer := replies(t, answering(t, http.StatusTooManyRequests,
			"X-RateLimit-Reset", reset))
		_, err := resetSearch(t, gentle, doer, clock)()
		stopped := stopOf(err)
		if stopped == nil || stopped.RetryAfter != "" ||
			!stopped.Until.Equal(start.Add(adsRest)) {
			t.Errorf("%s: %v", says, err)
		}
	}
}

// the anti-test: any answer but a 429 is exactly as it came, a reset
// and all, and a failure to send is given as it was.
func TestOnlyA429IsGivenARetryAfter(t *testing.T) {
	clock, _ := clockAt(t)
	reset := unixAt(start.Add(threeHours))
	for _, code := range []int{http.StatusOK, http.StatusServiceUnavailable,
		http.StatusForbidden} {
		answer := answering(t, code, "X-RateLimit-Reset", reset)
		got, err := ads.Resetting(replies(t, answer), clock).Do(
			mustRequest(t))
		if err != nil || got != answer ||
			got.Header.Get("Retry-After") != "" ||
			got.Header.Get("X-RateLimit-Reset") != reset {
			t.Errorf("%d: %v, %v", code, got.Header, err)
		}
	}
	failure := errors.New("the network")
	if got, err := ads.Resetting(replies(t, failure), clock).Do(
		mustRequest(t)); got != nil || err != failure {
		t.Errorf("%v, %v", got, err)
	}
}

// mustRequest is a request to ADS's search endpoint.
func mustRequest(t *testing.T) *http.Request {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(),
		http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	return request
}
