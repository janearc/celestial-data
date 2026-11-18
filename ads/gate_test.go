package ads_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/janearc/celestial-data/ads"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
)

// start is the time the gate tests begin at. It's defined: any time
// will do, and this is a plain one.
var start = time.Date(2026, time.November, 11, 12, 0, 0, 0, time.UTC)

// The numbers the gate tests expect of ADS's policy. They're defined by
// ads/numbers.yaml, which says why each was picked.
const (
	adsPace    = time.Second       // defined: ads-pace
	adsRest    = 5 * time.Minute   // defined: the web default rest
	halfway    = 0.5               // defined: the middle, for jitter
	twoMinutes = 120 * time.Second // defined: a Retry-After the tests give
)

// moment is the time a mock clock keeps. A Sleep passes the time it is
// asked to sleep and is written down, so a test sees what was waited
// without waiting.
type moment struct {
	lock  sync.Mutex
	now   time.Time
	slept []time.Duration
}

// read is the time now.
func (at *moment) read() time.Time {
	at.lock.Lock()
	defer at.lock.Unlock()
	return at.now
}

// sleep passes the time.
func (at *moment) sleep(_ context.Context, wait time.Duration) error {
	at.lock.Lock()
	defer at.lock.Unlock()
	at.slept = append(at.slept, wait)
	at.now = at.now.Add(wait)
	return nil
}

// clockAt is a mock clock at the start, and the moment it keeps.
func clockAt(t *testing.T) (*mocks.MockClock, *moment) {
	t.Helper()
	keeping := &moment{now: start}
	clock := mocks.NewMockClock(gomock.NewController(t))
	clock.EXPECT().Now().DoAndReturn(keeping.read).AnyTimes()
	clock.EXPECT().Sleep(gomock.Any(), gomock.Any()).
		DoAndReturn(keeping.sleep).AnyTimes()
	return clock, keeping
}

// gentleAt is ADS's gentle on a clock at the start.
func gentleAt(t *testing.T) (*web.Gentle, *moment) {
	t.Helper()
	clock, keeping := clockAt(t)
	random := mocks.NewMockRandom(gomock.NewController(t))
	random.EXPECT().Float64().Return(halfway).AnyTimes()
	gentle, err := ads.NewGentle(clock, random)
	if err != nil {
		t.Fatal(err)
	}
	return gentle, keeping
}

// scripted is a mock Doer that answers each request with the next of
// its answers, and keeps the requests it was sent. A request past its
// answers fails the test, as a mock does.
type scripted struct {
	*mocks.MockDoer
	sent []*http.Request
}

// replies is a mock Doer that answers with each of the answers in turn.
// An answer that is an error is given as one.
func replies(t *testing.T, answers ...any) *scripted {
	t.Helper()
	doer := &scripted{MockDoer: mocks.NewMockDoer(gomock.NewController(t))}
	for _, answer := range answers {
		doer.EXPECT().Do(gomock.Any()).DoAndReturn(doer.give(answer))
	}
	return doer
}

// give is the answer a request is given, kept with the request.
func (doer *scripted) give(answer any) func(*http.Request) (
	*http.Response, error) {
	return func(request *http.Request) (*http.Response, error) {
		doer.sent = append(doer.sent, request)
		if failure, ok := answer.(error); ok {
			return nil, failure
		}
		return answer.(*http.Response), nil
	}
}

// answering is an answer of a status, with the headers given as a name
// and its value, one after another.
func answering(t *testing.T, code int, headers ...string) *http.Response {
	t.Helper()
	answer, _ := reply(t, code, "", `{"response":{"numFound":0,"docs":[]}}`)
	for at := 0; at+1 < len(headers); at += 2 {
		answer.Header.Set(headers[at], headers[at+1])
	}
	return answer
}

// gated is a search through ADS's gentle: the Doer wrapped as the
// command wraps it.
func gated(t *testing.T, gentle *web.Gentle, doer ads.Doer) func() (
	*ads.Found, error) {
	t.Helper()
	through, err := web.Through(doer, gentle)
	if err != nil {
		t.Fatal(err)
	}
	return func() (*ads.Found, error) {
		return ads.Search(context.Background(), through, token, query,
			aFew, agent)
	}
}

// stopOf is the *web.Stopped in an error, or nil.
func stopOf(err error) *web.Stopped {
	var stopped *web.Stopped
	if errors.As(err, &stopped) {
		return stopped
	}
	return nil
}

// the test: ADS's policy is its own numbers, picked, one request at a
// time at a second's pace, no tries, no circuit and no repeated POST;
// what it does not name is the web package's.
func TestADSsPolicyIsItsOwnNumbers(t *testing.T) {
	policy, err := ads.Policy()
	if err != nil {
		t.Fatal(err)
	}
	if policy.Host != "ADS" || policy.Pace != adsPace ||
		policy.Tries != 0 || policy.Opens != 0 || policy.RepeatsPosts ||
		policy.Defaulted || policy.Rest != adsRest {
		t.Errorf("%+v", policy)
	}
}

// the anti-test: numbers with no pace, or a pace that makes no sense,
// are refused, saying for which host, and so is a gentle without a
// clock.
func TestADSsPolicyRefusesNumbersThatMakeNoSense(t *testing.T) {
	if _, err := ads.PolicyFrom(numbersOf{}); err == nil ||
		!strings.Contains(err.Error(), "ADS") {
		t.Errorf("no pace: %v", err)
	}
	clock, _ := clockAt(t)
	random := mocks.NewMockRandom(gomock.NewController(t))
	if _, err := ads.GentleFrom(numbersOf{}, clock, random); err == nil {
		t.Error("a gentle on numbers with no pace")
	}
	if _, err := ads.NewGentle(nil, random); err == nil {
		t.Error("a gentle with no clock")
	}
}

// numbersOf is numbers by name, as web.Numbers.
type numbersOf map[string]float64

// Get is the number of a name, or why there is none.
func (held numbersOf) Get(name string) (float64, error) {
	if value, ok := held[name]; ok {
		return value, nil
	}
	return 0, fmt.Errorf("no number %s", name)
}

// the test: a 429 with a Retry-After stops asking ADS, keeps the wait it
// asked for, and a second search sends nothing.
func TestA429WithARetryAfterStopsAndIsKept(t *testing.T) {
	gentle, _ := gentleAt(t)
	doer := replies(t, answering(t, http.StatusTooManyRequests,
		"Retry-After", "120"))
	search := gated(t, gentle, doer)
	_, err := search()
	stopped := stopOf(err)
	if stopped == nil || stopped.RetryAfter != "120" ||
		!stopped.Until.Equal(start.Add(twoMinutes)) || stopped.Host != "ADS" {
		t.Fatalf("%v", err)
	}
	if _, again := search(); stopOf(again) == nil {
		t.Errorf("a second search was not stopped: %v", again)
	}
}

// the anti-test: a 429 with only X-RateLimit-Reset, which the web
// package does not read, rests ADS as long as the default says, and
// what ADS wrote as the reset is not what it is left until.
func TestA429WithOnlyAResetRestsTheDefault(t *testing.T) {
	gentle, _ := gentleAt(t)
	doer := replies(t, answering(t, http.StatusTooManyRequests,
		"X-RateLimit-Reset", "1794009600"))
	_, err := gated(t, gentle, doer)()
	stopped := stopOf(err)
	if stopped == nil || stopped.RetryAfter != "" ||
		!stopped.Until.Equal(start.Add(adsRest)) {
		t.Errorf("%v", err)
	}
}

// the test: a 403, and a 503 with or without a Retry-After, stop asking
// ADS after one request, and none is asked again.
func TestA403Or503StopsAfterOneRequest(t *testing.T) {
	unavailable := http.StatusServiceUnavailable
	for says, answer := range map[string]*http.Response{
		"403": answering(t, http.StatusForbidden),
		"503": answering(t, unavailable),
		"503 and a Retry-After": answering(t, unavailable,
			"Retry-After", "30"),
	} {
		gentle, _ := gentleAt(t)
		search := gated(t, gentle, replies(t, answer))
		_, err := search()
		_, again := search()
		if stopOf(err) == nil || stopOf(again) == nil {
			t.Errorf("%s: %v, then %v", says, err, again)
		}
	}
}

// the anti-test: a failure with no word from ADS, a timeout, is not
// asked again, since ADS counts each request, and is not a stop: the
// next search is asked, because the circuit never opens.
func TestATimeoutIsNotAskedAgainAndStopsNothing(t *testing.T) {
	gentle, _ := gentleAt(t)
	timedOut := fmt.Errorf("Get: %w", &net.OpError{Op: "read",
		Err: os.ErrDeadlineExceeded})
	search := gated(t, gentle, replies(t, timedOut,
		answering(t, http.StatusOK)))
	if _, err := search(); err == nil || stopOf(err) != nil {
		t.Fatalf("a timeout: %v", err)
	}
	if _, err := search(); err != nil {
		t.Errorf("the search after a timeout: %v", err)
	}
}

// the anti-test: any other 5xx is an answer, not a stop: it is an error
// naming its status and the next search is asked.
func TestAnotherServerErrorIsNotAStop(t *testing.T) {
	gentle, _ := gentleAt(t)
	search := gated(t, gentle, replies(t,
		answering(t, http.StatusInternalServerError),
		answering(t, http.StatusOK)))
	if _, err := search(); err == nil || stopOf(err) != nil ||
		!strings.Contains(err.Error(), "status 500") {
		t.Fatalf("%v", err)
	}
	if _, err := search(); err != nil {
		t.Errorf("%v", err)
	}
}

// the test: two searches are a pace apart, the first with nothing to
// wait for, and ADS's answer reaches the caller whole, its headers read.
func TestSearchesAreAPaceApart(t *testing.T) {
	gentle, keeping := gentleAt(t)
	search := gated(t, gentle, replies(t,
		answering(t, http.StatusOK, "X-RateLimit-Remaining", remaining),
		answering(t, http.StatusOK)))
	found, err := search()
	if err != nil || found.Remaining() != remaining ||
		len(keeping.slept) != 0 {
		t.Fatalf("%v, %v, %v", found, err, keeping.slept)
	}
	if _, err := search(); err != nil ||
		len(keeping.slept) != 1 || keeping.slept[0] != adsPace {
		t.Errorf("%v, %v", err, keeping.slept)
	}
}

// the test: a search through the gentle is a GET without a body, which
// is why it needs no word on repeating a POST.
func TestASearchThroughTheGentleIsAGet(t *testing.T) {
	gentle, _ := gentleAt(t)
	doer := replies(t, answering(t, http.StatusOK))
	if _, err := gated(t, gentle, doer)(); err != nil ||
		doer.sent[0].Method != http.MethodGet ||
		doer.sent[0].Body != nil {
		t.Errorf("%v, %v", err, doer.sent[0])
	}
}

// the test: neither the token nor the header it travels in is in any
// string the gentle makes for an ADS request: a stop, what an override
// says, and the record kept on disk.
func TestNothingTheGentleWritesHoldsTheToken(t *testing.T) {
	gentle, _ := gentleAt(t)
	record := filepath.Join(t.TempDir(), "ads-gate")
	if err := gentle.Hold(record); err != nil {
		t.Fatal(err)
	}
	var said bytes.Buffer
	doer := replies(t, answering(t, http.StatusTooManyRequests,
		"Retry-After", "120"), answering(t, http.StatusOK))
	search := gated(t, gentle, doer)
	_, stop := search()
	gentle.Insist(&said)
	_, overridden := search()
	if err := gentle.Release(); err != nil {
		t.Fatal(err)
	}
	kept, err := os.ReadFile(record)
	if err != nil || overridden != nil {
		t.Fatalf("%v, %v", err, overridden)
	}
	// the proof means something only if the token did go in the request
	if sent := doer.sent[0].Header.Get("Authorization"); sent !=
		"Bearer "+token {
		t.Fatalf("the request carried %q", sent)
	}
	for name, text := range map[string]string{"the stop": stop.Error(),
		"the override": said.String(), "the record": string(kept)} {
		if text == "" || strings.Contains(text, token) ||
			strings.Contains(strings.ToLower(text), "authorization") ||
			strings.Contains(strings.ToLower(text), "bearer") {
			t.Errorf("%s: %q", name, text)
		}
	}
	if !strings.Contains(said.String(), "asking anyway") ||
		!strings.Contains(string(kept), "overrode") {
		t.Errorf("nothing was overridden: %q, %q", said.String(), kept)
	}
}

// A gentle stop stays discoverable after ADS removes the token from the
// fields that its Error prints. The raw error is not kept in the chain.
func TestAGentleStopKeepsItsTypeAndSanitizesItsFields(t *testing.T) {
	gentle, _ := gentleAt(t)
	doer := replies(t, answering(t, http.StatusTooManyRequests,
		"Retry-After", token))
	through, err := web.Through(doer, gentle)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ads.Search(context.Background(), through, token, token,
		aFew, agent)
	stopped := stopOf(err)
	if stopped == nil || stopped.Host != "ADS" ||
		stopped.RetryAfter != "[the token]" ||
		!strings.Contains(stopped.Why, "[the token]") ||
		strings.Contains(err.Error(), token) {
		t.Fatalf("gentle stop was not safely preserved: %v", err)
	}
	heldNowhere(t, err, "a gentle stop")
	if len(doer.sent) != 1 {
		t.Errorf("the gentle sent %d requests, want one", len(doer.sent))
	}
}
