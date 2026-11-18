package dasch

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
)

// These tests hold the asker to review 018's findings, now that the web
// package's gentle does the work: its questions are paced, it stops for
// good when the API says stop, and an answer is refused past its size.
// Each asks through a web.Through over a mock doer, on a clock the test
// keeps and with the jitter a test picks; none asks the network or
// sleeps.

// stood is a clock that stands at one time, and notes every sleep.
type stood struct {
	at    time.Time
	slept []time.Duration
}

// mock is a mock Clock that reads and sleeps on the stood clock.
func (clock *stood) mock(t *testing.T) *mocks.MockClock {
	t.Helper()
	made := mocks.NewMockClock(gomock.NewController(t))
	made.EXPECT().Now().DoAndReturn(func() time.Time {
		return clock.at
	}).AnyTimes()
	made.EXPECT().Sleep(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, wait time.Duration) error {
			clock.slept = append(clock.slept, wait)
			clock.at = clock.at.Add(wait)
			return nil
		}).AnyTimes()
	return made
}

// middle is a source of jitter that always picks the middle of the wait.
func middle(t *testing.T) *mocks.MockRandom {
	t.Helper()
	made := mocks.NewMockRandom(gomock.NewController(t))
	made.EXPECT().Float64().Return(0.5).AnyTimes()
	return made
}

// on is an asker over a doer, asking through this package's own policy.
func (clock *stood) on(t *testing.T, doer Doer) *asker {
	t.Helper()
	policy, err := Policy()
	if err != nil {
		t.Fatal(err)
	}
	return clock.under(t, policy, doer)
}

// under is an asker over a doer, asking through a policy.
func (clock *stood) under(t *testing.T, policy web.Policy,
	doer Doer) *asker {
	t.Helper()
	gentle, err := web.NewGentle(policy, clock.mock(t), middle(t))
	if err != nil {
		t.Fatal(err)
	}
	through, err := web.Through(doer, gentle)
	if err != nil {
		t.Fatal(err)
	}
	return newAsker(through, anyAgent)
}

// listing is a 200 whose body is a list of one line.
func listing() *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{},
		Body: io.NopCloser(strings.NewReader(`["a,b"]`))}
}

// listings answers each question with a listing of its own: a body is
// read once, so two answers cannot share one.
func listings(*http.Request) (*http.Response, error) {
	return listing(), nil
}

// refusing is an answer of a status with a Retry-After, when it has one.
func refusing(code int, after string) *http.Response {
	header := http.Header{}
	if after != "" {
		header.Set("Retry-After", after)
	}
	return &http.Response{StatusCode: code, Header: header,
		Body: io.NopCloser(strings.NewReader(""))}
}

// timedOut is a request that timed out, as the client gives it.
func timedOut(*http.Request) (*http.Response, error) {
	return nil, &url.Error{Op: "Post", URL: root,
		Err: context.DeadlineExceeded}
}

// noon is any time to stand at.
var noon = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

// the test: the first question goes at once and the second waits out the
// pace since the first began.
func TestASecondQuestionWaitsOutThePace(t *testing.T) {
	doer := mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).DoAndReturn(listings).Times(2)
	clock := &stood{at: noon}
	asking := clock.on(t, doer)
	for range 2 {
		if _, err := asking.post(curvePath, curve{}); err != nil {
			t.Fatal(err)
		}
	}
	if len(clock.slept) != 1 || clock.slept[0] != time.Second {
		t.Errorf("slept %v, not once for a second", clock.slept)
	}
}

// the anti-test: a question asked a pace or more after the last began
// does not wait.
func TestAQuestionAfterThePaceDoesNotWait(t *testing.T) {
	doer := mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).DoAndReturn(listings).Times(2)
	clock := &stood{at: noon}
	asking := clock.on(t, doer)
	if _, err := asking.post(curvePath, curve{}); err != nil {
		t.Fatal(err)
	}
	clock.at = clock.at.Add(time.Second)
	if _, err := asking.post(curvePath, curve{}); err != nil {
		t.Fatal(err)
	}
	if len(clock.slept) != 0 {
		t.Errorf("slept %v after the pace had passed", clock.slept)
	}
}

// the test: after a 429, every later question is refused as ErrStopped
// and nothing more is sent, until the time the API gave; then it goes.
// The stop is the web package's too, naming the host.
func TestA429StopsEveryLaterQuestionUntilItsTime(t *testing.T) {
	doer := mocks.NewMockDoer(gomock.NewController(t))
	first := doer.EXPECT().Do(gomock.Any()).Return(
		refusing(http.StatusTooManyRequests, "120"), nil)
	doer.EXPECT().Do(gomock.Any()).Return(listing(), nil).After(first)
	clock := &stood{at: noon}
	asking := clock.on(t, doer)
	for range 3 {
		_, err := asking.post(curvePath, curve{})
		var stop *web.Stopped
		if !errors.Is(err, ErrStopped) || !errors.As(err, &stop) ||
			stop.Host != hostName {
			t.Fatalf("not stopped: %v", err)
		}
	}
	clock.at = clock.at.Add(120 * time.Second)
	if _, err := asking.post(curvePath, curve{}); err != nil {
		t.Errorf("not asked again after the wait: %v", err)
	}
}

// the anti-test: a status that is not a stop does not stop the asker.
func TestAnotherStatusDoesNotStopTheAsker(t *testing.T) {
	doer := mocks.NewMockDoer(gomock.NewController(t))
	first := doer.EXPECT().Do(gomock.Any()).Return(
		refusing(http.StatusInternalServerError, ""), nil)
	doer.EXPECT().Do(gomock.Any()).Return(listing(), nil).After(first)
	asking := (&stood{at: noon}).on(t, doer)
	if _, err := asking.post(curvePath, curve{}); err == nil ||
		errors.Is(err, ErrStopped) {
		t.Fatalf("a 500: %v", err)
	}
	if _, err := asking.post(curvePath, curve{}); err != nil {
		t.Errorf("stopped by a 500: %v", err)
	}
}

// the test: a stop's Retry-After is read as seconds, as a date, or, when
// it is neither or none, as the rest of five minutes.
func TestAStopKeepsTheTimeItGave(t *testing.T) {
	for after, want := range map[string]time.Time{
		"30":                            noon.Add(30 * time.Second),
		"Mon, 09 Nov 2026 13:00:00 GMT": noon.Add(time.Hour),
		"soon":                          noon.Add(5 * time.Minute),
		"":                              noon.Add(5 * time.Minute),
	} {
		doer := mocks.NewMockDoer(gomock.NewController(t))
		doer.EXPECT().Do(gomock.Any()).Return(
			refusing(http.StatusTooManyRequests, after), nil)
		asking := (&stood{at: noon}).on(t, doer)
		_, err := asking.post(curvePath, curve{})
		var stop *web.Stopped
		if !errors.Is(err, ErrStopped) || !errors.As(err, &stop) {
			t.Fatalf("%q: not stopped: %v", after, err)
		}
		if !stop.Until.Equal(want) || stop.RetryAfter != after {
			t.Errorf("%q: until %v, not %v", after, stop.Until, want)
		}
	}
}

// the test: a 403, and a 503 that names a Retry-After, are the API
// asking us to stop too, which a 429 alone was before.
func TestA403OrA503WithARetryAfterStopsToo(t *testing.T) {
	for _, code := range []int{http.StatusForbidden,
		http.StatusServiceUnavailable} {
		doer := mocks.NewMockDoer(gomock.NewController(t))
		doer.EXPECT().Do(gomock.Any()).Return(refusing(code, "60"), nil)
		asking := (&stood{at: noon}).on(t, doer)
		_, err := asking.post(curvePath, curve{})
		if !errors.Is(err, ErrStopped) {
			t.Errorf("%d: not stopped: %v", code, err)
		}
	}
}

// the anti-test: a 503 with no Retry-After is the API saying nothing. It
// is asked again, as many times as the policy allows, and then given as
// the status it was, not as a stop.
func TestA503WithNoRetryAfterIsAskedAgainAndIsNoStop(t *testing.T) {
	policy, err := Policy()
	if err != nil {
		t.Fatal(err)
	}
	doer := mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).DoAndReturn(
		func(*http.Request) (*http.Response, error) {
			return refusing(http.StatusServiceUnavailable, ""), nil
		}).Times(policy.Tries + 1)
	asking := (&stood{at: noon}).on(t, doer)
	if _, err := asking.post(curvePath, curve{}); err == nil ||
		errors.Is(err, ErrStopped) {
		t.Errorf("a 503 with no Retry-After: %v", err)
	}
}

// the test: a timeout is asked again after a jittered backoff, and the
// answer of the second asking is the answer.
func TestATimeoutIsAskedAgainAfterABackoff(t *testing.T) {
	doer := mocks.NewMockDoer(gomock.NewController(t))
	first := doer.EXPECT().Do(gomock.Any()).DoAndReturn(timedOut)
	doer.EXPECT().Do(gomock.Any()).DoAndReturn(listings).After(first)
	clock := &stood{at: noon}
	asking := clock.on(t, doer)
	if _, err := asking.post(curvePath, curve{}); err != nil {
		t.Fatalf("not asked again: %v", err)
	}
	// the first retry's ceiling is a second, and the jitter picks half
	if len(clock.slept) == 0 || clock.slept[0] != time.Second/2 {
		t.Errorf("slept %v, not a half second first", clock.slept)
	}
}

// the test: a question that is asked again carries the same body, which
// is what makes a POST safe to send again here.
func TestAQuestionAskedAgainCarriesItsBodyAgain(t *testing.T) {
	var bodies []string
	doer := mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).DoAndReturn(
		func(request *http.Request) (*http.Response, error) {
			raw, _ := io.ReadAll(request.Body)
			bodies = append(bodies, string(raw))
			if len(bodies) == 1 {
				return timedOut(request)
			}
			return listing(), nil
		}).Times(2)
	asking := (&stood{at: noon}).on(t, doer)
	if _, err := asking.post(curvePath, curve{Refcat: "apass",
		Number: 7}); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || bodies[0] == "" || bodies[0] != bodies[1] {
		t.Errorf("bodies %q", bodies)
	}
}

// the anti-test: when the numbers say a POST is not safe to repeat, a
// timeout is given at once and nothing is asked again.
func TestATimeoutIsNotAskedAgainWhenAPostIsNotSafe(t *testing.T) {
	policy, err := policyFrom(numbersOf(t, "0"))
	if err != nil || policy.RepeatsPosts {
		t.Fatalf("%v, repeats posts: %v", err, policy.RepeatsPosts)
	}
	doer := mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).DoAndReturn(timedOut)
	asking := (&stood{at: noon}).under(t, policy, doer)
	if _, err := asking.post(curvePath, curve{}); err == nil ||
		errors.Is(err, ErrStopped) {
		t.Errorf("a timeout: %v", err)
	}
}

// numbersOf is numbers for the policy, with the repeats-posts one given.
func numbersOf(t *testing.T, repeats string) web.Numbers {
	t.Helper()
	return celestialdata.NewMeasured("test.yaml", []byte("numbers:\n"+
		"  - name: dasch-pace\n    value: 1\n    says: a test\n"+
		"    kind: picked\n    by: a test\n"+
		"  - name: dasch-repeats-posts\n    value: "+repeats+"\n"+
		"    says: a test\n    kind: picked\n    by: a test\n"))
}

// the test: a question that fails with no word in all its tries is given
// as the failure.
func TestAQuestionThatFailsInAllItsTriesIsGiven(t *testing.T) {
	policy, err := Policy()
	if err != nil {
		t.Fatal(err)
	}
	doer := mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).DoAndReturn(timedOut).
		Times(policy.Tries + 1)
	asking := (&stood{at: noon}).on(t, doer)
	_, err = asking.post(curvePath, curve{})
	if err == nil || errors.Is(err, ErrStopped) ||
		!strings.Contains(err.Error(), "POST "+curvePath+" failed") {
		t.Errorf("%v", err)
	}
}

// the test: after as many questions as the policy opens on have failed
// in all their tries, Starglass is not asked again for the run, and the
// stop says it is our own circuit.
func TestTheCircuitOpensAfterEnoughFailures(t *testing.T) {
	policy, err := Policy()
	if err != nil {
		t.Fatal(err)
	}
	doer := mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).DoAndReturn(timedOut).
		Times(policy.Opens * (policy.Tries + 1))
	asking := (&stood{at: noon}).on(t, doer)
	for range policy.Opens {
		asking.post(curvePath, curve{})
	}
	_, err = asking.post(curvePath, curve{})
	var stop *web.Stopped
	if !errors.Is(err, ErrStopped) || !errors.As(err, &stop) ||
		!stop.Ours {
		t.Errorf("circuit not open: %v", err)
	}
}

// the anti-test: one question that fails in all its tries does not open
// the circuit while the policy opens on more, and the next is asked.
func TestOneFailedQuestionDoesNotOpenTheCircuit(t *testing.T) {
	policy, err := Policy()
	if err != nil || policy.Opens < 2 {
		t.Fatalf("%v: the circuit opens on %d", err, policy.Opens)
	}
	doer := mocks.NewMockDoer(gomock.NewController(t))
	failing := doer.EXPECT().Do(gomock.Any()).DoAndReturn(timedOut).
		Times(policy.Tries + 1)
	doer.EXPECT().Do(gomock.Any()).DoAndReturn(listings).After(failing)
	asking := (&stood{at: noon}).on(t, doer)
	asking.post(curvePath, curve{})
	if _, err := asking.post(curvePath, curve{}); err != nil {
		t.Errorf("the circuit opened too soon: %v", err)
	}
}

// the anti-test: an error that is neither a stop nor a failure with no
// word is given as it came, named with the question, and is no stop.
func TestAnotherErrorIsNoStop(t *testing.T) {
	doer := mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).Return(nil, errors.New("no route"))
	asking := (&stood{at: noon}).on(t, doer)
	_, err := asking.post(curvePath, curve{})
	if err == nil || errors.Is(err, ErrStopped) ||
		!strings.Contains(err.Error(), "POST "+curvePath+" failed: "+
			"no route") {
		t.Errorf("%v", err)
	}
}

// the test: the policy is this package's own numbers, and the web
// package's defaults for the tries and opens, which it does not name.
func TestThePolicyIsTheOwnNumbersOfThePackage(t *testing.T) {
	policy, err := Policy()
	defaults, defaultsErr := web.DefaultPolicy(hostName)
	if err != nil || defaultsErr != nil {
		t.Fatalf("%v, %v", err, defaultsErr)
	}
	if policy.Host != hostName || policy.Pace != time.Second ||
		policy.Rest != 5*time.Minute || !policy.RepeatsPosts ||
		policy.Defaulted || policy.Tries != defaults.Tries ||
		policy.Opens != defaults.Opens {
		t.Errorf("%+v", policy)
	}
}

// the anti-test: numbers with no pace give no policy.
func TestNumbersWithNoPaceGiveNoPolicy(t *testing.T) {
	none := celestialdata.NewMeasured("test.yaml", []byte("numbers:\n"+
		"  - name: other\n    value: 1\n    says: a test\n"+
		"    kind: picked\n    by: a test\n"))
	if _, err := policyFrom(none); err == nil {
		t.Error("a policy with no pace")
	}
}

// the test: the host the policy is for is the host the questions go to.
func TestTheHostIsTheRootsHost(t *testing.T) {
	where, err := url.Parse(root)
	if err != nil || where.Hostname() != Host {
		t.Errorf("%q, %v", where.Hostname(), err)
	}
}

// the anti-test: an answer that breaks off as it is read is an error that
// names the question.
func TestAnAnswerThatBreaksOffIsAnError(t *testing.T) {
	listed, err := lines(curvePath, iotest.ErrReader(errors.New("cut")))
	if listed != nil || err == nil || !strings.Contains(err.Error(),
		"reading the answer: cut") {
		t.Errorf("%v, %v", listed, err)
	}
}

// the test: an answer of exactly mostBytes is read.
func TestAnAnswerOfTheMostBytesIsRead(t *testing.T) {
	line := strings.Repeat("a", mostBytes-len(`[""]`))
	listed, err := lines(curvePath, strings.NewReader(`["`+line+`"]`))
	if err != nil || len(listed) != 1 {
		t.Errorf("%d lines, %v", len(listed), err)
	}
}

// the anti-test: an answer one byte past mostBytes is refused unread.
func TestAnAnswerPastTheMostBytesIsRefused(t *testing.T) {
	line := strings.Repeat("a", mostBytes+1-len(`[""]`))
	body := bytes.NewReader([]byte(`["` + line + `"]`))
	listed, err := lines(curvePath, body)
	if listed != nil || err == nil ||
		!strings.Contains(err.Error(), "more than") {
		t.Errorf("%d lines, %v", len(listed), err)
	}
}
