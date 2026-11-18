package ads

import (
	"net/http"
	"strconv"
	"time"

	"github.com/janearc/celestial-data/web"
)

// resetting is a Doer that tells the web package when ADS's limit
// resets, in the one header it reads.
type resetting struct {
	doer  Doer
	clock web.Clock
}

// Resetting is a Doer for the web package's gentle to wrap. ADS says
// when the day's limit resets in X-RateLimit-Reset, as seconds since
// 1970, and the web package reads only Retry-After.
//
// So a 429 that has a reset ahead and no Retry-After is given one, as an
// HTTP date at that reset, and the gentle leaves ADS alone until then.
// Any other answer, and any failure to send, is given as it came.
//
// A reset that cannot be read, or is not ahead of the clock, is left as
// it is, and the gentle rests as long as it rests when ADS says no time.
// A date that has passed is never handed on: the gentle reads such a date
// as no wait, and ADS is to rest here.
func Resetting(doer Doer, clock web.Clock) Doer {
	return resetting{doer: doer, clock: clock}
}

// Do sends the request, and gives a 429 its Retry-After.
func (wrapped resetting) Do(request *http.Request) (*http.Response,
	error) {
	answer, err := wrapped.doer.Do(request)
	if err != nil || answer.StatusCode != http.StatusTooManyRequests ||
		answer.Header.Get(retryAfter) != "" {
		return answer, err
	}
	seconds, err := strconv.ParseInt(answer.Header.Get(rateReset),
		decimal, wholeBits)
	reset := time.Unix(seconds, noNanoseconds)
	if err == nil && reset.After(wrapped.clock.Now()) {
		date := reset.UTC().Format(http.TimeFormat)
		answer.Header.Set(retryAfter, date)
	}
	return answer, nil
}

// retryAfter is the header the web package reads a wait from.
const retryAfter = "Retry-After"

// How a reset is read as a whole number, and the time it names.
const (
	decimal       = 10 // defined by strconv: base ten
	wholeBits     = 64 // defined by strconv: an int64
	noNanoseconds = 0  // defined by time.Unix: a whole second
)
