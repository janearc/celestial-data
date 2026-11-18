package web_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
)

// askOnce is a request that enters and leaves the gentle.
func askOnce(t *testing.T, gentle *web.Gentle) {
	t.Helper()
	if err := gentle.Enter(context.Background()); err != nil {
		t.Fatal(err)
	}
	gentle.Leave()
}

// stoppedBy is what the gentle gives when it is asked to enter.
func stoppedBy(t *testing.T, gentle *web.Gentle) *web.Stopped {
	t.Helper()
	err := gentle.Enter(context.Background())
	var stopped *web.Stopped
	if !errors.As(err, &stopped) {
		t.Fatalf("not stopped: %v", err)
	}
	if web.Holding(gentle) {
		t.Fatal("a refused request holds the gentle")
	}
	return stopped
}

// refuse has a gentle judge a refusal of a status and a Retry-After, and
// gives what it said.
func refuse(t *testing.T, gentle *web.Gentle, code int,
	retryAfter string) *web.Stopped {
	t.Helper()
	if err := gentle.Enter(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer gentle.Leave()
	silent, err := gentle.Judge(code, retryAfter,
		errors.New(http.StatusText(code)))
	var stopped *web.Stopped
	if silent || !errors.As(err, &stopped) {
		t.Fatalf("%d: silent %v, %v", code, silent, err)
	}
	return stopped
}

// Each request starts at least its pace after the one before started.
func TestEachRequestWaitsOutThePaceSinceTheLastStarted(t *testing.T) {
	gentle, keeping := exampleAt(t)
	askOnce(t, gentle)
	askOnce(t, gentle)
	keeping.now = keeping.now.Add(time.Second)
	askOnce(t, gentle)
	want := []time.Duration{3 * time.Second, 2 * time.Second}
	if len(keeping.slept) != 2 || keeping.slept[0] != want[0] ||
		keeping.slept[1] != want[1] || gentle.Asked() != 3 {
		t.Errorf("%v, asked %d", keeping.slept, gentle.Asked())
	}
}

// Anti-test: a request after the pace has passed does not wait, and a
// clock set back waits no more than the pace.
func TestAPacePassedIsNotWaitedAndAClockSetBackWaitsOnlyThePace(t *testing.T) {
	gentle, keeping := exampleAt(t)
	askOnce(t, gentle)
	keeping.now = keeping.now.Add(time.Hour)
	askOnce(t, gentle)
	if len(keeping.slept) != 0 {
		t.Errorf("waited after the pace: %v", keeping.slept)
	}
	keeping.now = keeping.now.Add(-24 * time.Hour)
	askOnce(t, gentle)
	if len(keeping.slept) != 1 || keeping.slept[0] != 3*time.Second {
		t.Errorf("a clock set back: %v", keeping.slept)
	}
}

// A request with a context that has ended sends nothing, though the
// gentle is free; and one that ends while it waits its turn stops waiting
// and does not hold the gentle.
func TestAnEndedContextSendsNothingAndStopsWaitingForItsTurn(t *testing.T) {
	gentle, _ := exampleAt(t)
	ended, cancel := context.WithCancel(context.Background())
	cancel()
	if err := gentle.Enter(ended); !errors.Is(err, context.Canceled) {
		t.Errorf("%v", err)
	}
	if err := gentle.Enter(context.Background()); err != nil {
		t.Fatal(err)
	}
	// the context is alive when it is looked at first and has ended by the
	// time the request waits its turn
	done := make(chan struct{})
	close(done)
	late := mocks.NewMockContext(gomock.NewController(t))
	gomock.InOrder(late.EXPECT().Err().Return(nil),
		late.EXPECT().Err().Return(context.Canceled))
	late.EXPECT().Done().Return(done)
	if err := gentle.Enter(late); !errors.Is(err, context.Canceled) {
		t.Errorf("%v", err)
	}
	if !web.Holding(gentle) {
		t.Error("the first request lost the gentle")
	}
}

// Anti-test: a wait that is cut off by its context is an error, and does
// not hold the gentle, nor count as a request.
func TestAWaitCutOffByItsContextDoesNotHoldTheGentle(t *testing.T) {
	gentle, keeping := exampleAt(t)
	askOnce(t, gentle)
	keeping.refuseFrom = 1
	if err := gentle.Enter(context.Background()); !errors.Is(err,
		context.Canceled) {
		t.Errorf("%v", err)
	}
	if web.Holding(gentle) || gentle.Asked() != 1 {
		t.Errorf("held %v, asked %d", web.Holding(gentle), gentle.Asked())
	}
}

// A 403, a 429 and a 503 with a Retry-After are the host asking us to
// stop: the wait is kept, and nothing is sent until it has passed.
func TestAStatusWithARetryAfterStopsTheHostUntilItPasses(t *testing.T) {
	for _, code := range []int{http.StatusForbidden,
		http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		gentle, keeping := exampleAt(t)
		got := refuse(t, gentle, code, "120")
		until := start.Add(2 * time.Minute)
		if !got.Until.Equal(until) || got.RetryAfter != "120" ||
			got.Host != "Example" || got.Ours {
			t.Errorf("%d: %+v", code, got)
		}
		later := stoppedBy(t, gentle)
		says := "it asked to be left until " + until.Format(time.RFC3339)
		if !strings.Contains(later.Error(), says) {
			t.Errorf("%d: %v", code, later)
		}
		keeping.now = until
		askOnce(t, gentle)
	}
}

// Anti-test: a 404, a 500 and a success are not the host asking us to
// stop, and nothing is kept.
func TestOtherStatusesDoNotStopTheHost(t *testing.T) {
	gentle, _ := exampleAt(t)
	if err := gentle.Enter(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer gentle.Leave()
	for _, code := range []int{http.StatusNotFound,
		http.StatusInternalServerError} {
		failure := errors.New(http.StatusText(code))
		silent, err := gentle.Judge(code, "120", failure)
		if silent || err != failure {
			t.Errorf("%d: silent %v, %v", code, silent, err)
		}
	}
	if silent, err := gentle.Judge(http.StatusOK, "", nil); silent ||
		err != nil {
		t.Errorf("a success: %v %v", silent, err)
	}
}

// A Retry-After is read as seconds or as a date.
func TestARetryAfterIsReadAsSecondsOrADate(t *testing.T) {
	date := start.Add(90 * time.Minute)
	gone := start.Add(-time.Hour)
	for asked, want := range map[string]time.Time{
		"0":                          start,
		"1":                          start.Add(time.Second),
		date.Format(http.TimeFormat): date,
		gone.Format(http.TimeFormat): gone,
	} {
		gentle, _ := exampleAt(t)
		got := refuse(t, gentle, http.StatusTooManyRequests, asked)
		if !got.Until.Equal(want) {
			t.Errorf("%q: %v, not %v", asked, got.Until, want)
		}
	}
}

// A Retry-After date that has already passed is the server saying the
// time has gone: the next request is asked at once, without the rest the
// gentle takes when a host names no time.
func TestAPastRetryAfterDateStopsNothing(t *testing.T) {
	gentle, keeping := exampleAt(t)
	gone := start.Add(-time.Hour).Format(http.TimeFormat)
	refuse(t, gentle, http.StatusTooManyRequests, gone)
	if err := gentle.Enter(context.Background()); err != nil {
		t.Fatalf("a date that has passed still stopped it: %v", err)
	}
	gentle.Leave()
	if keeping.total() >= restUnsaid {
		t.Errorf("slept %v for a date that had passed", keeping.total())
	}
}

// Anti-test: a date still ahead does stop the next request, and an
// unreadable one rests the unsaid default.
func TestAFutureRetryAfterDateAndAnUnreadableOneStillStop(t *testing.T) {
	for _, asked := range []string{
		start.Add(time.Hour).Format(http.TimeFormat), "soon"} {
		gentle, _ := exampleAt(t)
		refuse(t, gentle, http.StatusTooManyRequests, asked)
		var stopped *web.Stopped
		err := gentle.Enter(context.Background())
		if !errors.As(err, &stopped) {
			t.Errorf("%q: asked again too soon: %v", asked, err)
		}
	}
}

// Anti-test: a refusal with no Retry-After, one that is not a number or a
// date, a negative one, and a number beyond what a whole number holds,
// rest as long as the policy says; and a number that does fit but is too
// long for a duration is kept as the longest one that does fit.
func TestARetryAfterThatSaysNothingRestsAndOneTooLongIsKeptShort(t *testing.T) {
	rest := start.Add(300 * time.Second)
	longest := start.Add(time.Duration(1<<63-1) / time.Second * time.Second)
	for asked, want := range map[string]time.Time{
		"":                     rest,
		"soon":                 rest,
		"-5":                   rest,
		"99999999999999999999": rest,
		"99999999999999":       longest,
	} {
		gentle, _ := exampleAt(t)
		got := refuse(t, gentle, http.StatusTooManyRequests, asked)
		if !got.Until.Equal(want) {
			t.Errorf("%q: %v, not %v", asked, got.Until, want)
		}
	}
}

// A 503 with no Retry-After is a failure with no word, which may be
// asked again; for a host that may not be asked again, it is a stop.
func TestABareServiceUnavailableIsSilentUnlessNoRetryIsAllowed(t *testing.T) {
	gentle, _ := exampleAt(t)
	if err := gentle.Enter(context.Background()); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("503")
	silent, err := gentle.Judge(http.StatusServiceUnavailable, "", failure)
	if !silent || err != failure {
		t.Errorf("silent %v, %v", silent, err)
	}
	gentle.Leave()
	clock, _ := clockAt(t, start)
	quiet := gentleOn(t, policyOf(t, "Quiet", "quiet"), clock)
	got := refuse(t, quiet, http.StatusServiceUnavailable, "")
	if got.Until.Sub(start) != 300*time.Second {
		t.Errorf("%+v", got)
	}
}

// A 429 or a 403 with no Retry-After is a stop that rests, not a retry.
func TestARefusalWithNoRetryAfterIsAStopNotARetry(t *testing.T) {
	for _, code := range []int{http.StatusTooManyRequests,
		http.StatusForbidden} {
		gentle, _ := exampleAt(t)
		got := refuse(t, gentle, code, "")
		if got.RetryAfter != "" || got.Until.Sub(start) != 300*time.Second {
			t.Errorf("%d: %+v", code, got)
		}
	}
}

// A failure with no answer at all, that is a timeout, a reset or a
// connection that ended early, is silent.
func TestAFailureWithNoAnswerIsSilentWhenTheNetworkMayPass(t *testing.T) {
	gentle, _ := exampleAt(t)
	if err := gentle.Enter(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer gentle.Leave()
	for name, failure := range silentKinds() {
		silent, err := gentle.Judge(0, "", failure)
		if !silent || err != failure {
			t.Errorf("%s: silent %v, %v", name, silent, err)
		}
	}
}

// Anti-test: a malformed address, a refused connection, a name not found
// and too many redirects are given as they were, not silent, and they
// do not move the breaker however often they come.
func TestAFailureThatIsOursOrLastingIsNotSilentAndNotCounted(t *testing.T) {
	gentle, _ := exampleAt(t)
	if err := gentle.Enter(context.Background()); err != nil {
		t.Fatal(err)
	}
	gentle.Leave()
	for name, failure := range lastingKinds() {
		for range 3 {
			silent, err := gentle.Judge(0, "", failure)
			if silent || err != failure {
				t.Errorf("%s: silent %v, %v", name, silent, err)
			}
		}
	}
	askOnce(t, gentle)
}

// Any answer but a silent one shows the host is talking, and starts the
// count again, whatever its status; a silent one does not.
func TestAnyAnswerStartsTheCountAgainButASilentOneDoesNot(t *testing.T) {
	for _, code := range []int{http.StatusInternalServerError,
		http.StatusNotFound} {
		gentle, _ := exampleAt(t)
		gentle.GiveUp()
		gentle.Judge(code, "", errors.New(http.StatusText(code)))
		gentle.GiveUp()
		askOnce(t, gentle)
	}
	gentle, _ := exampleAt(t)
	gentle.GiveUp()
	gentle.Judge(0, "", timedOut)
	gentle.GiveUp()
	stoppedBy(t, gentle)
}

// A host that is asked through an override and answers shows it is
// talking, and its circuit closes, whatever the status.
func TestAnAnswerClosesAnOpenCircuit(t *testing.T) {
	gentle, _ := exampleAt(t)
	gentle.GiveUp()
	gentle.GiveUp()
	gentle.Judge(http.StatusNotFound, "", errors.New("404"))
	askOnce(t, gentle)
}

// A request that has used up its tries counts against the host, and when
// two have, in a row, the host is left alone for the run, and says so.
func TestTheCircuitOpensAfterTheOpensAndAsksNothing(t *testing.T) {
	gentle, _ := exampleAt(t)
	gentle.GiveUp()
	askOnce(t, gentle)
	gentle.GiveUp()
	stopped := stoppedBy(t, gentle)
	says := "we stopped asking Example: it failed too often with no " +
		"word from it"
	if !stopped.Ours || !stopped.Until.IsZero() ||
		!strings.Contains(stopped.Error(), says) {
		t.Errorf("%v", stopped)
	}
}

// A policy with no opens has no circuit: however many requests use up
// their tries, the host is still asked, and nothing is said stopped.
func TestWithNoOpensTheCircuitNeverOpens(t *testing.T) {
	clock, _ := clockAt(t, start)
	policy := example(t)
	policy.Opens = 0
	gentle := gentleOn(t, policy, clock)
	for range 10 {
		gentle.GiveUp()
		askOnce(t, gentle)
	}
}

// Anti-test: the same requests with two opens do open it.
func TestWithTwoOpensTheSameFailuresOpenTheCircuit(t *testing.T) {
	gentle, _ := exampleAt(t)
	for range 2 {
		gentle.GiveUp()
	}
	if stopped := stoppedBy(t, gentle); !stopped.Ours {
		t.Errorf("%v", stopped)
	}
}

// Anti-test: a success between two failures starts the count again, and
// a success closes a circuit that an override had asked through.
func TestASuccessStartsTheCountAgainAndClosesTheCircuit(t *testing.T) {
	gentle, _ := exampleAt(t)
	gentle.GiveUp()
	if _, err := gentle.Judge(http.StatusOK, "", nil); err != nil {
		t.Fatal(err)
	}
	gentle.GiveUp()
	askOnce(t, gentle)
	gentle.GiveUp()
	gentle.Insist(&bytes.Buffer{})
	askOnce(t, gentle)
	if _, err := gentle.Judge(http.StatusOK, "", nil); err != nil {
		t.Fatal(err)
	}
	gentle.Insist(nil)
	askOnce(t, gentle)
}

// The circuit does not outlast the run: a new gentle asks again.
func TestTheCircuitDoesNotSurviveTheRun(t *testing.T) {
	gentle, _ := exampleAt(t)
	gentle.GiveUp()
	gentle.GiveUp()
	stoppedBy(t, gentle)
	again, _ := exampleAt(t)
	askOnce(t, again)
}

// A stop for a host that has no pace of its own says the pace was the
// default.
func TestAStopOfAHostWithoutAPaceSaysThePaceWasTheDefault(t *testing.T) {
	policy, err := web.DefaultPolicy("example.org")
	if err != nil {
		t.Fatal(err)
	}
	clock, _ := clockAt(t, start)
	gentle := gentleOn(t, policy, clock)
	got := refuse(t, gentle, http.StatusTooManyRequests, "30")
	says := "no pace was picked for this host, so the default pace was used"
	if !strings.Contains(got.Error(), says) ||
		!strings.Contains(got.Error(), "example.org asked us to stop") {
		t.Errorf("%v", got)
	}
}

// Anti-test: a stop for a host with a pace of its own does not say so,
// and names no override when none is set.
func TestAStopDoesNotNameAnUnsetOverride(t *testing.T) {
	gentle, _ := exampleAt(t)
	text := refuse(t, gentle, http.StatusTooManyRequests, "30").Error()
	for _, says := range []string{"Example asked us to stop: Too Many " +
		"Requests", "its Retry-After is 30", "asked again at " +
		start.Add(30*time.Second).Format(time.RFC3339)} {
		if !strings.Contains(text, says) {
			t.Errorf("%q is not in %q", says, text)
		}
	}
	if strings.Contains(text, "default pace") {
		t.Errorf("%v", text)
	}
	if strings.Contains(text, "an override for one run") {
		t.Errorf("an unset override was named: %v", text)
	}
}

// A gentle that insists asks through a wait the host asked for, after
// saying what it asked, once, however many requests go.
func TestAGentleThatInsistsAsksThroughAWaitAndSaysSoOnce(t *testing.T) {
	gentle, keeping := exampleAt(t)
	refuse(t, gentle, http.StatusTooManyRequests, "120")
	var said bytes.Buffer
	gentle.Insist(&said)
	askOnce(t, gentle)
	keeping.now = keeping.now.Add(time.Minute)
	askOnce(t, gentle)
	until := start.Add(2 * time.Minute).Format(time.RFC3339)
	want := "Example: it asked to be left until " + until
	if strings.Count(said.String(), want) != 1 ||
		!strings.Contains(said.String(), "its Retry-After was 120; "+
			"asking anyway") {
		t.Errorf("%q", said.String())
	}
}

// Anti-test: the same wait without an override is not asked through, and
// nothing is said.
func TestWithoutInsistingAWaitIsNotAskedThrough(t *testing.T) {
	gentle, _ := exampleAt(t)
	refuse(t, gentle, http.StatusTooManyRequests, "120")
	stoppedBy(t, gentle)
}

// A gentle that insists asks through an open circuit and says so, once.
func TestAGentleThatInsistsAsksThroughAnOpenCircuitAndSaysSoOnce(t *testing.T) {
	gentle, _ := exampleAt(t)
	gentle.GiveUp()
	gentle.GiveUp()
	var said bytes.Buffer
	gentle.Insist(&said)
	askOnce(t, gentle)
	askOnce(t, gentle)
	want := "Example: it failed too often with no word from it; this is " +
		"our own circuit, not its word; asking anyway"
	if strings.Count(said.String(), want) != 1 {
		t.Errorf("%q", said.String())
	}
}

// Anti-test: a gentle that insists and is not stopped says nothing.
func TestAGentleThatInsistsAndIsNotStoppedSaysNothing(t *testing.T) {
	gentle, _ := exampleAt(t)
	var said bytes.Buffer
	gentle.Insist(&said)
	askOnce(t, gentle)
	if said.Len() != 0 {
		t.Errorf("%q", said.String())
	}
}

// A gentle is its own chooser, for any address.
func TestAGentleIsItsOwnChooser(t *testing.T) {
	gentle, _ := exampleAt(t)
	got, err := gentle.For("https://example.org/any")
	if err != nil || got != gentle {
		t.Errorf("%v %v", got, err)
	}
}
