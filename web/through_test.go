package web_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
)

// through is a doer through the gentle of a policy, over a mock doer.
func through(t *testing.T, policy web.Policy) (web.Doer, *mocks.MockDoer,
	*web.Gentle) {
	t.Helper()
	clock, _ := clockAt(t, start)
	gentle := gentleOn(t, policy, clock)
	inner := mocks.NewMockDoer(gomock.NewController(t))
	wrapped, err := web.Through(inner, gentle)
	if err != nil {
		t.Fatal(err)
	}
	return wrapped, inner, gentle
}

// requestOf is a request to the test address, with a body when one is
// given, as a bytes reader gives it, so it can be given again.
func requestOf(t *testing.T, method, body string) *http.Request {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	}
	request, err := http.NewRequestWithContext(t.Context(), method, address,
		reader)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

// okay is an answer that succeeded.
func okay() *http.Response { return refusal(http.StatusOK, "") }

// A doer through a gentle gives the answer as it came, and holds the
// gentle until the answer's body is closed.
func TestThroughGivesTheAnswerAsItCameAndHoldsTheGentleToItsClose(
	t *testing.T) {
	wrapped, inner, gentle := through(t, holds(t))
	inner.EXPECT().Do(gomock.Any()).Return(okay(), nil)
	answer, err := wrapped.Do(requestOf(t, http.MethodGet, ""))
	if err != nil || answer.StatusCode != http.StatusOK {
		t.Fatalf("%v", err)
	}
	if !web.Holding(gentle) {
		t.Error("let go before the body closed")
	}
	answer.Body.Close()
	if web.Holding(gentle) {
		t.Error("held after the body closed")
	}
}

// Anti-test: with a policy that does not hold, the gentle is let go as
// the answer arrives.
func TestThroughLetsGoAtOnceWhenThePolicyDoesNotHold(t *testing.T) {
	wrapped, inner, gentle := through(t, example(t))
	inner.EXPECT().Do(gomock.Any()).Return(okay(), nil)
	answer, err := wrapped.Do(requestOf(t, http.MethodGet, ""))
	if err != nil || web.Holding(gentle) {
		t.Errorf("held before the body closed: %v", err)
	}
	answer.Body.Close()
}

// Anti-test: a doer with no doer inside or no chooser is refused.
func TestThroughRefusesWhatItNeeds(t *testing.T) {
	gentle, _ := exampleAt(t)
	inner := mocks.NewMockDoer(gomock.NewController(t))
	if got, err := web.Through(nil, gentle); err == nil || got != nil {
		t.Error("no doer")
	}
	if got, err := web.Through(inner, nil); err == nil || got != nil {
		t.Error("no chooser")
	}
}

// A 429, a 403 and a 503 with a Retry-After are the host asking us to
// stop: the answer is a Stopped, and its body is closed.
func TestThroughReadsAStopFromTheStatusAndItsRetryAfter(t *testing.T) {
	for _, code := range []int{http.StatusTooManyRequests,
		http.StatusForbidden, http.StatusServiceUnavailable} {
		wrapped, inner, gentle := through(t, example(t))
		inner.EXPECT().Do(gomock.Any()).Return(refusal(code, "60"), nil).
			Times(1)
		answer, err := wrapped.Do(requestOf(t, http.MethodGet, ""))
		var stopped *web.Stopped
		if answer != nil || !errors.As(err, &stopped) ||
			stopped.RetryAfter != "60" {
			t.Fatalf("%d: %v", code, err)
		}
		if web.Holding(gentle) {
			t.Errorf("%d: held", code)
		}
		if _, err := wrapped.Do(requestOf(t, http.MethodGet, "")); !errors.As(
			err, &stopped) {
			t.Errorf("%d: the next went: %v", code, err)
		}
	}
}

// Anti-test: any other status, a 404 or a 500, is given as it came for
// the adapter to read, and is not retried.
func TestThroughGivesOtherStatusesAsTheyCame(t *testing.T) {
	for _, code := range []int{http.StatusNotFound,
		http.StatusInternalServerError, http.StatusUnauthorized} {
		wrapped, inner, _ := through(t, example(t))
		inner.EXPECT().Do(gomock.Any()).Return(refusal(code, "5"), nil).
			Times(1)
		answer, err := wrapped.Do(requestOf(t, http.MethodGet, ""))
		if err != nil || answer.StatusCode != code {
			t.Errorf("%d: %v", code, err)
		}
		answer.Body.Close()
	}
}

// A request that fails with no word, or is answered by a bare 503, is
// asked again, as many times as the policy says.
func TestThroughAsksAgainAfterAFailureWithNoWord(t *testing.T) {
	wrapped, inner, _ := through(t, example(t))
	gomock.InOrder(
		inner.EXPECT().Do(gomock.Any()).Return(nil, resetBy),
		inner.EXPECT().Do(gomock.Any()).Return(refusal(
			http.StatusServiceUnavailable, ""), nil),
		inner.EXPECT().Do(gomock.Any()).Return(okay(), nil))
	answer, err := wrapped.Do(requestOf(t, http.MethodGet, ""))
	if err != nil || answer.StatusCode != http.StatusOK {
		t.Fatalf("%v", err)
	}
	answer.Body.Close()
}

// Anti-test: when every try fails the last failure is given, and a bare
// 503 that is last is a failure that says its status.
func TestThroughGivesTheLastFailureWhenTheTriesAreUsedUp(t *testing.T) {
	wrapped, inner, _ := through(t, example(t))
	gomock.InOrder(
		inner.EXPECT().Do(gomock.Any()).Return(nil, resetBy),
		inner.EXPECT().Do(gomock.Any()).Return(nil, timedOut),
		inner.EXPECT().Do(gomock.Any()).Return(refusal(
			http.StatusServiceUnavailable, ""), nil))
	answer, err := wrapped.Do(requestOf(t, http.MethodGet, ""))
	if answer != nil || err == nil || !strings.Contains(err.Error(),
		"Service Unavailable") {
		t.Errorf("%v", err)
	}
}

// A POST is sent again only when the policy says it is safe and the
// request can give its body again; and each try is sent with its body.
func TestThroughRepeatsAPostOnlyWhenItIsSafeAndItsBodyCanBeGivenAgain(
	t *testing.T) {
	wrapped, inner, _ := through(t, example(t))
	var bodies []string
	inner.EXPECT().Do(gomock.Any()).DoAndReturn(func(
		request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		bodies = append(bodies, string(body))
		if len(bodies) == 1 {
			return nil, resetBy
		}
		return okay(), nil
	}).Times(2)
	answer, err := wrapped.Do(requestOf(t, http.MethodPost, "a=b"))
	if err != nil {
		t.Fatal(err)
	}
	answer.Body.Close()
	if len(bodies) != 2 || bodies[0] != "a=b" || bodies[1] != "a=b" {
		t.Errorf("%q", bodies)
	}
}

// Anti-test: a POST to a host that has not said it is safe to repeat is
// asked once; so is one whose body cannot be given again; and a POST
// with no body is a request like another.
func TestThroughNeverRepeatsABodyWithoutLeaveToDoSo(t *testing.T) {
	reset := resetBy
	wrapped, inner, _ := through(t, defaulted(t))
	inner.EXPECT().Do(gomock.Any()).Return(nil, reset).Times(1)
	if _, err := wrapped.Do(requestOf(t, http.MethodPost, "a=b")); err !=
		reset {
		t.Errorf("a host that did not say: %v", err)
	}
	once, inner, _ := through(t, example(t))
	inner.EXPECT().Do(gomock.Any()).Return(nil, reset).Times(1)
	request := requestOf(t, http.MethodPost, "a=b")
	request.GetBody = nil
	if _, err := once.Do(request); err != reset {
		t.Errorf("a body that cannot be given again: %v", err)
	}
	empty, inner, _ := through(t, example(t))
	gomock.InOrder(inner.EXPECT().Do(gomock.Any()).Return(nil, reset),
		inner.EXPECT().Do(gomock.Any()).Return(okay(), nil))
	if _, err := empty.Do(requestOf(t, http.MethodPost, "")); err != nil {
		t.Errorf("a post with no body: %v", err)
	}
}

// Anti-test: a body that cannot be given again is a failure of ours, so
// the request is not sent, not asked again, and not counted against the
// host: after many, the host is still asked.
func TestThroughGivesAFailureToGiveTheBodyAgainAndDoesNotCountIt(
	t *testing.T) {
	wrapped, inner, _ := through(t, example(t))
	broken := errors.New("no body")
	given := 0
	for range 3 {
		request := requestOf(t, http.MethodPost, "a=b")
		request.GetBody = func() (io.ReadCloser, error) {
			given++
			return nil, broken
		}
		if _, err := wrapped.Do(request); err != broken {
			t.Errorf("%v", err)
		}
	}
	if given != 3 {
		t.Errorf("the body was asked for %d times in three requests", given)
	}
	inner.EXPECT().Do(gomock.Any()).Return(okay(), nil)
	if _, err := wrapped.Do(requestOf(t, http.MethodGet, "")); err != nil {
		t.Errorf("the host was shut out: %v", err)
	}
}

// Anti-test: any answer of the host's, a 500 among them, starts the count
// again, as it does through a web: one request used up before it and one
// after do not open the circuit.
func TestThroughCountsAnAnswerOfAnyStatusAsTheHostTalking(t *testing.T) {
	wrapped, inner, _ := through(t, example(t))
	gomock.InOrder(
		inner.EXPECT().Do(gomock.Any()).Return(nil, resetBy).Times(3),
		inner.EXPECT().Do(gomock.Any()).Return(refusal(
			http.StatusInternalServerError, ""), nil),
		inner.EXPECT().Do(gomock.Any()).Return(nil, resetBy).Times(3),
		inner.EXPECT().Do(gomock.Any()).Return(okay(), nil))
	get := func() (*http.Response, error) {
		return wrapped.Do(requestOf(t, http.MethodGet, ""))
	}
	if _, err := get(); err != resetBy {
		t.Fatalf("%v", err)
	}
	if answered, err := get(); err != nil ||
		answered.StatusCode != http.StatusInternalServerError {
		t.Fatalf("%v", err)
	} else {
		answered.Body.Close()
	}
	if _, err := get(); err != resetBy {
		t.Fatalf("%v", err)
	}
	if answered, err := get(); err != nil {
		t.Errorf("stopped: %v", err)
	} else {
		answered.Body.Close()
	}
}

// A chooser that cannot choose is an error, and nothing is sent.
func TestThroughRefusesAnAddressThatNamesNoHost(t *testing.T) {
	clock, _ := clockAt(t, start)
	inner := mocks.NewMockDoer(gomock.NewController(t))
	wrapped, _ := web.Through(inner, web.NewHosts("", clock,
		randomOf(t, half), nil))
	request, err := http.NewRequest(http.MethodGet, "/just/a/path", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.Do(request); err == nil {
		t.Error("sent")
	}
}
