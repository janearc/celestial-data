//ninja:vow dojo the asker's pace and its stop, on a clock the test keeps
package dasch

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/janearc/celestial-data/mocks"
)

// These tests hold the asker to review 018's findings: it paces its
// questions, stops for good when the API says stop, and refuses an
// answer past its size. Each runs on a clock the test keeps; none asks
// the network.

// stood is a clock that stands at one time, and notes every sleep.
type stood struct {
	at    time.Time
	slept []time.Duration
}

// on is an asker over a doer on the stood clock.
func (clock *stood) on(doer Doer) *asker {
	asking := newAsker(doer, anyAgent)
	asking.now = func() time.Time { return clock.at }
	asking.sleep = func(wait time.Duration) {
		clock.slept = append(clock.slept, wait)
		clock.at = clock.at.Add(wait)
	}
	return asking
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

// refusing is a 429 with a Retry-After.
func refusing(after string) *http.Response {
	header := http.Header{}
	if after != "" {
		header.Set("Retry-After", after)
	}
	return &http.Response{StatusCode: http.StatusTooManyRequests,
		Header: header, Body: io.NopCloser(strings.NewReader(""))}
}

// noon is any time to stand at.
var noon = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

// the test: the first question goes at once and the second waits out the
// pace since the first began.
func TestASecondQuestionWaitsOutThePace(t *testing.T) {
	doer := mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).DoAndReturn(listings).Times(2)
	clock := &stood{at: noon}
	asking := clock.on(doer)
	for range 2 {
		if _, err := asking.post(curvePath, curve{}); err != nil {
			t.Fatal(err)
		}
	}
	if len(clock.slept) != 1 || clock.slept[0] != pace {
		t.Errorf("slept %v, not once for %v", clock.slept, pace)
	}
}

// the anti-test: a question asked a pace or more after the last began
// does not wait.
func TestAQuestionAfterThePaceDoesNotWait(t *testing.T) {
	doer := mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).DoAndReturn(listings).Times(2)
	clock := &stood{at: noon}
	asking := clock.on(doer)
	if _, err := asking.post(curvePath, curve{}); err != nil {
		t.Fatal(err)
	}
	clock.at = clock.at.Add(pace)
	if _, err := asking.post(curvePath, curve{}); err != nil {
		t.Fatal(err)
	}
	if len(clock.slept) != 0 {
		t.Errorf("slept %v after the pace had passed", clock.slept)
	}
}

// the test: after a 429, every later question is refused as ErrStopped
// and nothing more is sent, until the time the API gave; then it goes.
func TestA429StopsEveryLaterQuestionUntilItsTime(t *testing.T) {
	doer := mocks.NewMockDoer(gomock.NewController(t))
	first := doer.EXPECT().Do(gomock.Any()).Return(refusing("120"), nil)
	doer.EXPECT().Do(gomock.Any()).Return(listing(), nil).After(first)
	clock := &stood{at: noon}
	asking := clock.on(doer)
	for range 3 {
		if _, err := asking.post(curvePath, curve{}); !errors.Is(err,
			ErrStopped) {
			t.Fatalf("not stopped: %v", err)
		}
	}
	clock.at = clock.at.Add(120 * time.Second)
	if _, err := asking.post(curvePath, curve{}); err != nil {
		t.Errorf("not asked again after the wait: %v", err)
	}
}

// the anti-test: a status that is not 429 does not stop the asker.
func TestAnotherStatusDoesNotStopTheAsker(t *testing.T) {
	doer := mocks.NewMockDoer(gomock.NewController(t))
	first := doer.EXPECT().Do(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusInternalServerError, Header: http.Header{},
		Body: io.NopCloser(strings.NewReader(""))}, nil)
	doer.EXPECT().Do(gomock.Any()).Return(listing(), nil).After(first)
	asking := (&stood{at: noon}).on(doer)
	if _, err := asking.post(curvePath, curve{}); err == nil ||
		errors.Is(err, ErrStopped) {
		t.Fatalf("a 500: %v", err)
	}
	if _, err := asking.post(curvePath, curve{}); err != nil {
		t.Errorf("stopped by a 500: %v", err)
	}
}

// What the asker's comments promise, held here as numbers so a change
// to the code's own is seen. Each is defined by those comments.
const (
	promisedRest  = 5 * time.Minute // defined: asker.go's five minutes
	promisedBytes = 32 << 20        // defined: asker.go's 32 MiB
	// longSeconds is a Retry-After of 285 years: past half the longest
	// a Duration holds, and short of the longest. Defined.
	longSeconds = 9_000_000_000
)

// the test: a 429's Retry-After is read as seconds, as a date, or, when
// it is neither or none, as the rest of five minutes. A wait of none
// stops nothing, and a wait of centuries is kept whole.
func TestA429KeepsTheTimeItGave(t *testing.T) {
	for after, want := range map[string]time.Time{
		"30":                            noon.Add(30 * time.Second),
		"Mon, 09 Nov 2026 13:00:00 GMT": noon.Add(time.Hour),
		"soon":                          noon.Add(promisedRest),
		"":                              noon.Add(promisedRest),
		"0":                             noon,
		"9000000000":                    noon.Add(longSeconds * time.Second),
	} {
		doer := mocks.NewMockDoer(gomock.NewController(t))
		doer.EXPECT().Do(gomock.Any()).Return(refusing(after), nil)
		asking := (&stood{at: noon}).on(doer)
		if _, err := asking.post(curvePath, curve{}); !errors.Is(err,
			ErrStopped) {
			t.Errorf("%q: not stopped: %v", after, err)
		}
		if !asking.until.Equal(want) {
			t.Errorf("%q: until %v, not %v", after, asking.until, want)
		}
	}
}

// the test: a question that would go a nanosecond before the pace is
// up waits that nanosecond.
func TestTheLastNanosecondOfThePaceIsWaited(t *testing.T) {
	doer := mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).DoAndReturn(listings).Times(2)
	clock := &stood{at: noon}
	asking := clock.on(doer)
	if _, err := asking.post(curvePath, curve{}); err != nil {
		t.Fatal(err)
	}
	clock.at = noon.Add(pace - time.Nanosecond)
	if _, err := asking.post(curvePath, curve{}); err != nil {
		t.Fatal(err)
	}
	if len(clock.slept) != 1 || clock.slept[0] != time.Nanosecond {
		t.Errorf("slept %v, not a nanosecond", clock.slept)
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

// the anti-test: an answer one byte past the 32 MiB promised is refused
// unread.
func TestAnAnswerPastTheMostBytesIsRefused(t *testing.T) {
	line := strings.Repeat("a", promisedBytes+onePast-len(`[""]`))
	body := bytes.NewReader([]byte(`["` + line + `"]`))
	listed, err := lines(curvePath, body)
	if listed != nil || err == nil ||
		!strings.Contains(err.Error(), "more than") {
		t.Errorf("%d lines, %v", len(listed), err)
	}
}
