package web_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
)

// address is where the requests of these tests are sent. The mock web
// answers them, so nothing goes there. It's defined: a name that is
// reserved to never be anyone's.
const address = "https://example.org/file"

// within is a web through a gentle of a policy, over a mock web, and the
// moment the gentle's clock keeps.
func within(t *testing.T, policy web.Policy) (web.Web, *mocks.MockWeb,
	*web.Gentle, *moment) {
	t.Helper()
	clock, keeping := clockAt(t, start)
	gentle := gentleOn(t, policy, clock)
	inner := mocks.NewMockWeb(gomock.NewController(t))
	wrapped, err := web.Within(t.Context(), inner, gentle)
	if err != nil {
		t.Fatal(err)
	}
	return wrapped, inner, gentle, keeping
}

// defaulted is the policy of a host with no numbers of its own, which
// asks nothing again that carries a body.
func defaulted(t *testing.T) web.Policy {
	t.Helper()
	policy, err := web.DefaultPolicy("example.org")
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

// A web within a gentle is a web, and gives what the web inside gives.
func TestWithinGivesWhatTheWebInsideGives(t *testing.T) {
	wrapped, inner, _, _ := within(t, example(t))
	form := url.Values{"a": {"b"}}
	inner.EXPECT().Get(address).Return(answer(), int64(1), nil)
	inner.EXPECT().Head(address).Return(int64(7), nil)
	inner.EXPECT().Post(address, form).Return("where", nil)
	body, length, err := wrapped.Get(address)
	if err != nil || length != 1 {
		t.Fatalf("%v %d", err, length)
	}
	body.Close()
	if got, err := wrapped.Head(address); err != nil || got != 7 {
		t.Errorf("%v %d", err, got)
	}
	if got, err := wrapped.Post(address, form); err != nil ||
		got != "where" {
		t.Errorf("%v %q", err, got)
	}
}

// Anti-test: a web with no context, no web inside or no chooser is
// refused.
func TestWithinRefusesWhatItNeeds(t *testing.T) {
	gentle, _ := exampleAt(t)
	inner := mocks.NewMockWeb(gomock.NewController(t))
	cases := map[string]func() (web.Web, error){
		"no context": func() (web.Web, error) {
			return web.Within(nil, inner, gentle)
		},
		"no web": func() (web.Web, error) {
			return web.Within(t.Context(), nil, gentle)
		},
		"no chooser": func() (web.Web, error) {
			return web.Within(t.Context(), inner, nil)
		},
	}
	for name, build := range cases {
		if got, err := build(); err == nil || got != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// holds is a policy that holds the gentle until a body is closed.
func holds(t *testing.T) web.Policy {
	t.Helper()
	policy := example(t)
	policy.HoldsUntilClosed = true
	return policy
}

// Anti-test: with a policy that does not hold, the gentle is let go as
// the answer arrives, so a second request to the host does not wait for
// the first body to be closed.
func TestABodyDoesNotHoldTheGentleWhenThePolicyDoesNot(t *testing.T) {
	wrapped, inner, gentle, _ := within(t, example(t))
	inner.EXPECT().Get(address).Return(answer(), int64(1), nil).Times(2)
	first, _, err := wrapped.Get(address)
	if err != nil || web.Holding(gentle) {
		t.Fatalf("held before the body closed: %v", err)
	}
	second, _, err := wrapped.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	first.Close()
	second.Close()
	if web.Holding(gentle) {
		t.Error("held after the bodies closed")
	}
}

// A Get or a Head that fails gives the length the web inside gave beside
// the error.
func TestAFailureGivesTheLengthTheWebInsideGave(t *testing.T) {
	wrapped, inner, _, _ := within(t, example(t))
	const unsaid = -1
	inner.EXPECT().Get(address).Return(nil, int64(unsaid), malformed)
	inner.EXPECT().Head(address).Return(int64(unsaid), malformed)
	_, length, err := wrapped.Get(address)
	if err != malformed || length != unsaid {
		t.Errorf("get: %d %v", length, err)
	}
	if length, err := wrapped.Head(address); err != malformed ||
		length != unsaid {
		t.Errorf("head: %d %v", length, err)
	}
}

// Anti-test: a request the web inside was never asked, because the
// context had ended, gives no length.
func TestARequestNeverAskedGivesNoLength(t *testing.T) {
	clock, _ := clockAt(t, start)
	gentle := gentleOn(t, example(t), clock)
	ended, end := context.WithCancel(t.Context())
	end()
	inner := mocks.NewMockWeb(gomock.NewController(t))
	wrapped, err := web.Within(ended, inner, gentle)
	if err != nil {
		t.Fatal(err)
	}
	if _, length, err := wrapped.Get(address); err == nil || length != 0 {
		t.Errorf("get: %d %v", length, err)
	}
	if length, err := wrapped.Head(address); err == nil || length != 0 {
		t.Errorf("head: %d %v", length, err)
	}
}

// A body holds the gentle until it is closed, and closing it twice lets
// go of nothing more.
func TestABodyHoldsTheGentleUntilItIsClosed(t *testing.T) {
	wrapped, inner, gentle, _ := within(t, holds(t))
	inner.EXPECT().Get(address).Return(answer(), int64(1), nil)
	body, _, err := wrapped.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	if !web.Holding(gentle) {
		t.Error("the gentle was let go before the body closed")
	}
	body.Close()
	body.Close()
	if web.Holding(gentle) {
		t.Error("the gentle is held after the body closed")
	}
	if _, err := body.Read(make([]byte, 1)); err != nil {
		t.Errorf("a read of a body: %v", err)
	}
}

// Anti-test: a Head and a Post let the gentle go when they are answered,
// and a Get that failed does not hold it.
func TestAHeadAPostAndAFailedGetDoNotHoldTheGentle(t *testing.T) {
	wrapped, inner, gentle, _ := within(t, example(t))
	inner.EXPECT().Head(address).Return(int64(1), nil)
	inner.EXPECT().Post(address, gomock.Any()).Return("", nil)
	inner.EXPECT().Get(address).Return(nil, int64(0), malformed)
	wrapped.Head(address)
	wrapped.Post(address, nil)
	if _, _, err := wrapped.Get(address); err != malformed {
		t.Error("a failure was not given as it was")
	}
	if web.Holding(gentle) {
		t.Error("held")
	}
}

// A 429 with a Retry-After stops the host: the answer is a Stopped, and
// the next request goes nowhere.
func TestARefusalStopsTheHostAndTheNextRequestGoesNowhere(t *testing.T) {
	wrapped, inner, _, _ := within(t, example(t))
	refused := statusError(http.StatusTooManyRequests, "120")
	inner.EXPECT().Get(address).Return(nil, int64(0), refused).Times(1)
	var stopped *web.Stopped
	_, _, err := wrapped.Get(address)
	if !errors.As(err, &stopped) || stopped.RetryAfter != "120" {
		t.Fatalf("%v", err)
	}
	if _, _, err := wrapped.Get(address); !errors.As(err, &stopped) {
		t.Errorf("%v", err)
	}
	if _, err := wrapped.Head(address); !errors.As(err, &stopped) {
		t.Errorf("%v", err)
	}
	if _, err := wrapped.Post(address, nil); !errors.As(err, &stopped) {
		t.Errorf("%v", err)
	}
}

// A Get that fails with no word is asked again after a jittered wait
// that doubles, and the pace is waited out between.
func TestASilentFailureIsAskedAgainAfterABackoff(t *testing.T) {
	wrapped, inner, _, keeping := within(t, example(t))
	gomock.InOrder(
		inner.EXPECT().Get(address).Return(nil, int64(0), resetBy),
		inner.EXPECT().Get(address).Return(nil, int64(0), resetBy),
		inner.EXPECT().Get(address).Return(answer(), int64(1), nil))
	body, _, err := wrapped.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	body.Close()
	want := []time.Duration{500 * time.Millisecond, 2500 * time.Millisecond,
		time.Second, 2 * time.Second}
	if len(keeping.slept) != len(want) {
		t.Fatalf("%v", keeping.slept)
	}
	for at, wait := range want {
		if keeping.slept[at] != wait {
			t.Errorf("sleep %d: %v, not %v", at, keeping.slept[at], wait)
		}
	}
}

// Each silent failure is asked again, a Get and a Head both.
func TestEachSilentFailureIsAskedAgain(t *testing.T) {
	for name, failure := range silentKinds() {
		wrapped, inner, _, _ := within(t, example(t))
		gomock.InOrder(
			inner.EXPECT().Get(address).Return(nil, int64(0), failure),
			inner.EXPECT().Get(address).Return(answer(), int64(1), nil),
			inner.EXPECT().Head(address).Return(int64(0), failure),
			inner.EXPECT().Head(address).Return(int64(9), nil))
		body, _, err := wrapped.Get(address)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		body.Close()
		if got, err := wrapped.Head(address); err != nil || got != 9 {
			t.Errorf("%s: %v %d", name, err, got)
		}
	}
}

// Anti-test: a malformed address, a refused connection, a name not found
// and too many redirects are given at once, as they were, asked once, and
// do not move the breaker: after many the host is still asked.
func TestFailuresThatAreOursOrLastingAreNotAskedAgainOrCounted(t *testing.T) {
	for name, failure := range lastingKinds() {
		wrapped, inner, _, _ := within(t, example(t))
		inner.EXPECT().Get(address).Return(nil, int64(0), failure).Times(3)
		for range 3 {
			if _, _, err := wrapped.Get(address); err != failure {
				t.Fatalf("%s: %v", name, err)
			}
		}
		inner.EXPECT().Get(address).Return(answer(), int64(1), nil)
		body, _, err := wrapped.Get(address)
		if err != nil {
			t.Fatalf("%s: stopped: %v", name, err)
		}
		body.Close()
	}
}

// A Head that fails with no word is asked again too.
func TestAHeadThatFailsWithNoWordIsAskedAgain(t *testing.T) {
	wrapped, inner, _, _ := within(t, example(t))
	gomock.InOrder(
		inner.EXPECT().Head(address).Return(int64(0), timedOut),
		inner.EXPECT().Head(address).Return(int64(9), nil))
	if got, err := wrapped.Head(address); err != nil || got != 9 {
		t.Errorf("%v %d", err, got)
	}
}

// Anti-test: a status that is no word of the host's, a 404, is not asked
// again, and does not count against the host.
func TestANotFoundIsNotAskedAgain(t *testing.T) {
	wrapped, inner, gentle, _ := within(t, example(t))
	missing := statusError(http.StatusNotFound, "")
	inner.EXPECT().Get(address).Return(nil, int64(0), missing).Times(1)
	if _, _, err := wrapped.Get(address); err != missing {
		t.Errorf("%v", err)
	}
	if web.Holding(gentle) {
		t.Error("held")
	}
}

// A request that uses up its tries gives the failure as it was, and two
// of them in a row open the circuit, so a third asks nothing.
func TestTriesUsedUpTwiceOpenTheCircuit(t *testing.T) {
	wrapped, inner, _, _ := within(t, example(t))
	inner.EXPECT().Get(address).Return(nil, int64(0), resetBy).Times(6)
	for range 2 {
		if _, _, err := wrapped.Get(address); err != resetBy {
			t.Fatalf("%v", err)
		}
	}
	var stopped *web.Stopped
	_, _, err := wrapped.Get(address)
	if !errors.As(err, &stopped) || !stopped.Ours {
		t.Errorf("%v", err)
	}
}

// Anti-test: any answer of the host's, a 500 among them, starts the count
// again, so one request used up before it and one after do not open the
// circuit.
func TestAnAnswerOfAnyStatusStartsTheCountAgain(t *testing.T) {
	wrapped, inner, _, _ := within(t, example(t))
	broken := statusError(http.StatusInternalServerError, "")
	gomock.InOrder(
		inner.EXPECT().Get(address).Return(nil, int64(0), resetBy).Times(3),
		inner.EXPECT().Get(address).Return(nil, int64(0), broken),
		inner.EXPECT().Get(address).Return(nil, int64(0), resetBy).Times(3),
		inner.EXPECT().Get(address).Return(answer(), int64(1), nil))
	for _, failure := range []error{resetBy, broken, resetBy} {
		if _, _, err := wrapped.Get(address); err != failure {
			t.Fatalf("%v", err)
		}
	}
	if _, _, err := wrapped.Get(address); err != nil {
		t.Errorf("stopped: %v", err)
	}
}

// A 503 with no Retry-After is asked again, as a failure with no word.
func TestABareServiceUnavailableIsAskedAgain(t *testing.T) {
	wrapped, inner, _, _ := within(t, example(t))
	bare := statusError(http.StatusServiceUnavailable, "")
	gomock.InOrder(
		inner.EXPECT().Get(address).Return(nil, int64(0), bare),
		inner.EXPECT().Get(address).Return(answer(), int64(1), nil))
	body, _, err := wrapped.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	body.Close()
}

// Anti-test: a request that carries a body, and a host that has not said
// it is safe to repeat one, is paced and judged and not asked again, and
// does not count against the host, so two such failures do not shut out
// its GETs.
func TestAPostIsNotAskedAgainUnlessTheHostSaysItIsSafe(t *testing.T) {
	wrapped, inner, _, _ := within(t, defaulted(t))
	inner.EXPECT().Post(address, gomock.Any()).Return("", resetBy).Times(3)
	for range 3 {
		if _, err := wrapped.Post(address, nil); err != resetBy {
			t.Errorf("%v", err)
		}
	}
	inner.EXPECT().Get(address).Return(answer(), int64(1), nil)
	body, _, err := wrapped.Get(address)
	if err != nil {
		t.Fatalf("the POSTs shut the GETs out: %v", err)
	}
	body.Close()
	again, inner, _, _ := within(t, example(t))
	gomock.InOrder(
		inner.EXPECT().Post(address, gomock.Any()).Return("", resetBy),
		inner.EXPECT().Post(address, gomock.Any()).Return("there", nil))
	if got, err := again.Post(address, nil); err != nil || got != "there" {
		t.Errorf("%v %q", err, got)
	}
}

// A failure that comes when the context has ended is the failure, not the
// host's silence: it is not asked again and does not count.
func TestAFailureAfterTheContextEndedIsNotAskedAgain(t *testing.T) {
	clock, _ := clockAt(t, start)
	gentle := gentleOn(t, example(t), clock)
	inner := mocks.NewMockWeb(gomock.NewController(t))
	ctx, cancel := context.WithCancel(t.Context())
	wrapped, _ := web.Within(ctx, inner, gentle)
	inner.EXPECT().Get(address).DoAndReturn(func(string) (io.ReadCloser,
		int64, error) {
		cancel()
		return nil, 0, timedOut
	}).Times(1)
	if _, _, err := wrapped.Get(address); err != timedOut {
		t.Errorf("%v", err)
	}
	if web.Holding(gentle) {
		t.Error("held")
	}
}

// Anti-test: a retry whose wait, or whose pace, is cut off by the context
// gives why, and leaves the gentle.
func TestARetryCutOffByTheContextGivesWhyAndLeavesTheGentle(t *testing.T) {
	for _, from := range []int{1, 2} {
		wrapped, inner, gentle, keeping := within(t, example(t))
		keeping.refuseFrom = from
		inner.EXPECT().Get(address).Return(nil, int64(0), resetBy).Times(1)
		if _, _, err := wrapped.Get(address); !errors.Is(err,
			context.Canceled) {
			t.Errorf("sleep %d: %v", from, err)
		}
		if web.Holding(gentle) {
			t.Errorf("sleep %d: held", from)
		}
	}
}

// insisting is a web through the example host's gentle, which holds a
// record at path when there is one, and insists, writing to the buffer.
func insisting(t *testing.T, policy web.Policy, path string) (web.Web,
	*mocks.MockWeb, *web.Gentle, *moment, *bytes.Buffer) {
	t.Helper()
	wrapped, inner, gentle, keeping := within(t, policy)
	if path != "" {
		if err := gentle.Hold(path); err != nil {
			t.Fatal(err)
		}
	}
	said := &bytes.Buffer{}
	gentle.Insist(said)
	return wrapped, inner, gentle, keeping, said
}

// A fresh stop, with the gentle insisting, says which host and what it
// asked for, waits the pace, and asks the request once more.
func TestAFreshStopIsAskedOnceMoreWhenTheGentleInsists(t *testing.T) {
	wrapped, inner, _, keeping, said := insisting(t, example(t), "")
	refused := statusError(http.StatusTooManyRequests, "120")
	gomock.InOrder(
		inner.EXPECT().Get(address).Return(nil, int64(0), refused),
		inner.EXPECT().Get(address).Return(answer(), int64(1), nil))
	body, _, err := wrapped.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	body.Close()
	want := "Example: GET " + address + ": Too Many Requests; its " +
		"Retry-After was 120; asking once more, as told to\n"
	if said.String() != want {
		t.Errorf("%q", said.String())
	}
	if len(keeping.slept) != 1 || keeping.slept[0] != 3*time.Second {
		t.Errorf("the pace was not waited: %v", keeping.slept)
	}
}

// Anti-test: a stop that is refused again is given, once, after the
// request was asked twice and no more; it says the override was tried and
// does not suggest one.
func TestAStopRefusedAgainIsGivenAfterTwoAsksAndSaysTheOverrideWasTried(
	t *testing.T) {
	wrapped, inner, _, _, _ := insisting(t, example(t), "")
	refused := statusError(http.StatusTooManyRequests, "120")
	inner.EXPECT().Get(address).Return(nil, int64(0), refused).Times(2)
	_, _, err := wrapped.Get(address)
	var stopped *web.Stopped
	if !errors.As(err, &stopped) || !stopped.Tried {
		t.Fatalf("%v", err)
	}
	if !strings.Contains(err.Error(), "the override was tried, and it "+
		"refused again") || strings.Contains(err.Error(), "an override "+
		"for one run asks anyway") {
		t.Errorf("%v", err)
	}
}

// A refusal that names no Retry-After says so.
func TestAFreshStopThatNamesNoRetryAfterSaysSo(t *testing.T) {
	wrapped, inner, _, _, said := insisting(t, example(t), "")
	refused := statusError(http.StatusForbidden, "")
	gomock.InOrder(
		inner.EXPECT().Get(address).Return(nil, int64(0), refused),
		inner.EXPECT().Get(address).Return(answer(), int64(1), nil))
	wrapped.Get(address)
	if !strings.Contains(said.String(), "it named no Retry-After; asking "+
		"once more") {
		t.Errorf("%q", said.String())
	}
}

// Anti-test: a POST to a host that does not say it may be repeated is not
// asked again for a stop; its stop says the override is set and why it
// was not used, and does not suggest one.
func TestAPostThatMayNotBeRepeatedIsNotAskedAgainForAStop(t *testing.T) {
	wrapped, inner, _, _, said := insisting(t, defaulted(t), "")
	refused := web.NewStatus(http.MethodPost, address, "Too Many Requests",
		http.StatusTooManyRequests, "5")
	inner.EXPECT().Post(address, gomock.Any()).Return("", refused).Times(1)
	_, err := wrapped.Post(address, nil)
	var stopped *web.Stopped
	if !errors.As(err, &stopped) || stopped.Tried || !stopped.Insisting ||
		!strings.Contains(err.Error(), "the override is set, but this "+
			"request may not be asked again") ||
		strings.Contains(err.Error(), "an override for one run") {
		t.Errorf("%v", err)
	}
	if said.Len() != 0 {
		t.Errorf("said %q of a request it did not repeat", said.String())
	}
}

// A POST to a host whose posts may be repeated is asked again for a stop,
// as a GET is.
func TestAPostThatMayBeRepeatedIsAskedAgainForAStop(t *testing.T) {
	wrapped, inner, _, _, _ := insisting(t, example(t), "")
	refused := web.NewStatus(http.MethodPost, address, "Too Many Requests",
		http.StatusTooManyRequests, "5")
	gomock.InOrder(
		inner.EXPECT().Post(address, gomock.Any()).Return("", refused),
		inner.EXPECT().Post(address, gomock.Any()).Return("there", nil))
	if got, err := wrapped.Post(address, nil); err != nil ||
		got != "there" {
		t.Errorf("%v %q", err, got)
	}
}

// The override and what the host asked are kept in the record, beside the
// wait that is still written, so the next run without the flag meets it.
func TestTheOverrideOfAFreshStopIsKeptAndTheWaitStillStandsNextRun(
	t *testing.T) {
	path := recordIn(t)
	wrapped, inner, gentle, _, _ := insisting(t, example(t), path)
	refused := statusError(http.StatusTooManyRequests, "120")
	inner.EXPECT().Get(address).Return(nil, int64(0), refused).Times(2)
	wrapped.Get(address)
	released(t, gentle)
	// the second refusal came after the pace, so its wait ends later than
	// the first's, which is the one the override says it overrode
	first := start.Add(2 * time.Minute).Format(time.RFC3339Nano)
	last := start.Add(3*time.Second + 2*time.Minute)
	got := recordOf(t, path)
	for _, says := range []string{"until " + last.Format(time.RFC3339Nano),
		"asked 120\n", " " + first + " 120\n"} {
		if !strings.Contains(got, says) {
			t.Errorf("%q is not in %q", says, got)
		}
	}
	clock, _ := clockAt(t, start.Add(time.Minute))
	next := gentleOn(t, example(t), clock)
	if err := next.Hold(path); err != nil {
		t.Fatal(err)
	}
	if stopped := stoppedBy(t, next); stopped.RetryAfter != "120" {
		t.Errorf("%+v", stopped)
	}
}

// A refusal of a POST carries its status and its Retry-After, with the
// text it always had, never the form. It is the one test that needs the
// client itself, so it asks a local server.
func TestAPostRefusalIsAStatusWithItsRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		answer http.ResponseWriter, _ *http.Request) {
		answer.Header().Set("Retry-After", "7")
		answer.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	_, err := patient(t).Post(server.URL, url.Values{"password": {"x"}})
	var status *web.Status
	if !errors.As(err, &status) || status.Code() != 429 ||
		status.RetryAfter() != "7" {
		t.Fatalf("%v", err)
	}
	want := "POST " + server.URL + ": 429 Too Many Requests"
	if err.Error() != want || strings.Contains(err.Error(), "password") {
		t.Errorf("%q", err)
	}
}
