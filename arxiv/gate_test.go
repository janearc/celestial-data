package arxiv_test

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/arxiv"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
)

// The counts of requests these tests see let through.
const (
	noRequest   = 0 // defined: none
	oneRequest  = 1 // defined: one
	twoRequests = 2 // defined: one, then the next
)

// unsaid is the length a web gives when its server says none. It's
// defined by web.Web.
const unsaid = -1

// A gate takes arXiv's least pace, and any slower.
func TestAGateTakesThePaceArXivAllowsOrSlower(t *testing.T) {
	for _, pace := range []time.Duration{seconds(t, "arxivs-least"),
		time.Minute} {
		if _, err := arxiv.NewGate(pace, arxiv.Wall()); err != nil {
			t.Errorf("%v: %v", pace, err)
		}
	}
}

// Anti-test: a pace quicker than arXiv allows is refused, saying the
// pace it allows, and so is a gate with no clock.
func TestAGateRefusesAQuickerPaceAndNoClock(t *testing.T) {
	least := seconds(t, "arxivs-least")
	quicker, quick := arxiv.NewGate(least-time.Nanosecond, arxiv.Wall())
	clockless, unclocked := arxiv.NewGate(least, nil)
	if quicker != nil || clockless != nil ||
		!errors.Is(quick, celestialdata.Refused()) ||
		!errors.Is(unclocked, celestialdata.Refused()) ||
		!strings.Contains(quick.Error(), "every "+least.String()) {
		t.Errorf("%v, %v", quick, unclocked)
	}
}

// The first request waits for nothing, and the next waits out what is
// left of the pace from the start of the one before.
func TestTheNextRequestWaitsOutThePace(t *testing.T) {
	start, least, step := begun(t), seconds(t, "arxivs-least"),
		seconds(t, "step")
	clock := readings(t, start, start, start.Add(step), start.Add(least))
	clock.EXPECT().Sleep(gomock.Any(), least-step).Return(nil)
	gate := gateOn(t, clock)
	through := pacedOn(context.Background(), t, answering(t), gate)
	headed(t, through)
	headed(t, through)
	if gate.Asked() != twoRequests {
		t.Errorf("%d asked", gate.Asked())
	}
}

// A request that comes a nanosecond before the pace has passed waits
// out that nanosecond.
func TestARequestANanosecondEarlyWaitsItOut(t *testing.T) {
	start, least := begun(t), seconds(t, "arxivs-least")
	early := start.Add(least - time.Nanosecond)
	clock := readings(t, start, start, early, start.Add(least))
	clock.EXPECT().Sleep(gomock.Any(), time.Nanosecond).Return(nil)
	through := pacedOn(context.Background(), t, answering(t),
		gateOn(t, clock))
	headed(t, through)
	headed(t, through)
}

// Anti-test: a request that comes just as the pace has passed, or long
// after, waits for nothing.
func TestARequestAfterThePaceWaitsForNothing(t *testing.T) {
	start := begun(t)
	for _, later := range []time.Duration{seconds(t, "arxivs-least"),
		time.Hour} {
		after := start.Add(later)
		gate := gateOn(t, readings(t, start, start, after, after))
		through := pacedOn(context.Background(), t, answering(t), gate)
		headed(t, through)
		headed(t, through)
	}
}

// A wait the context ends is the context's error, and the gate is let
// go: the next request goes through.
func TestAWaitTheContextEndsLetsTheGateGo(t *testing.T) {
	start := begun(t)
	later := start.Add(time.Hour)
	clock := readings(t, start, start, start, later, later)
	clock.EXPECT().Sleep(gomock.Any(), seconds(t, "arxivs-least")).
		Return(context.Canceled)
	gate := gateOn(t, clock)
	through := pacedOn(context.Background(), t, answering(t), gate)
	headed(t, through)
	if _, err := through.Head(paper); err != context.Canceled {
		t.Fatalf("%v", err)
	}
	headed(t, through)
	if gate.Asked() != twoRequests {
		t.Errorf("%d asked", gate.Asked())
	}
}

// Anti-test: with its context ended, a paced web asks nothing, for any
// of its requests, and says the context's error.
func TestAnEndedPacedWebAsksNothing(t *testing.T) {
	gate := gateOn(t, arxiv.Wall())
	quiet := mocks.NewMockWeb(gomock.NewController(t))
	through := pacedOn(ended(), t, quiet, gate)
	body, length, got := through.Get(paper)
	headedLength, head := through.Head(paper)
	where, post := through.Post(arxiv.API, nil)
	if body != nil || length != noRequest || headedLength != noRequest ||
		where != "" || gate.Asked() != noRequest {
		t.Errorf("%v %d %d %q", body, length, headedLength, where)
	}
	for _, err := range []error{got, head, post} {
		if err != context.Canceled {
			t.Errorf("%v", err)
		}
	}
}

// waiter is a request waiting at the gate, which says when it has gone
// through, and how.
type waiter struct {
	through web.Web
	done    chan error
}

// waiting is a waiter on a web.
func waiting(through web.Web) *waiter {
	return &waiter{through: through, done: make(chan error, oneRequest)}
}

// head asks the paper's Head, and says how it went.
func (request *waiter) head() {
	_, err := request.through.Head(paper)
	request.done <- err
}

// One request at a time: while a Get's body is open, the next waits, and
// when the body closes, the next goes through.
func TestOneRequestAtATime(t *testing.T) {
	gate := gateOn(t, hourly(t))
	get := answering(t)
	get.EXPECT().Get(paper).DoAndReturn(served)
	through := pacedOn(context.Background(), t, get, gate)
	body, _, err := through.Get(paper)
	if err != nil {
		t.Fatal(err)
	}
	next := waiting(through)
	go next.head()
	select {
	case err := <-next.done:
		t.Fatalf("went through while a body was open: %v", err)
	case <-time.After(seconds(t, "watch")):
	}
	if read, err := io.ReadAll(body); string(read) != pdf || err != nil {
		t.Errorf("read %q, %v", read, err)
	}
	body.Close()
	if err := <-next.done; err != nil {
		t.Fatal(err)
	}
}

// watched is a body that, as it first closes, watches for a waiting
// request going through, which it must not until the close is done.
type watched struct {
	content io.Reader
	next    *waiter
	watch   time.Duration
	closes  int
	passed  bool
}

// firstClose is a body's first close. It's defined: the one that holds
// the gate.
const firstClose = 1

// Read reads what the body holds.
func (body *watched) Read(into []byte) (int, error) {
	return body.content.Read(into)
}

// Close watches, on the first close, for the waiting request going
// through while it closes.
func (body *watched) Close() error {
	body.closes++
	if body.closes != firstClose {
		return nil
	}
	select {
	case err := <-body.next.done:
		body.passed = true
		body.next.done <- err
	case <-time.After(body.watch):
	}
	return nil
}

// A Get's body lets the gate go after it has closed, not before, and a
// body closed again lets go of nothing.
func TestABodyLetsTheGateGoOnceAfterItCloses(t *testing.T) {
	gate := gateOn(t, hourly(t))
	get := answering(t)
	through := pacedOn(context.Background(), t, get, gate)
	next := waiting(through)
	content := strings.NewReader(pdf)
	body := &watched{content: content, next: next,
		watch: seconds(t, "watch")}
	get.EXPECT().Get(paper).Return(body, content.Size(), nil)
	opened, _, err := through.Get(paper)
	if err != nil {
		t.Fatal(err)
	}
	go next.head()
	opened.Close()
	if err := <-next.done; err != nil || body.passed {
		t.Fatalf("went through as the body closed %v: %v", body.passed,
			err)
	}
	again := waiting(through)
	go again.closing(opened)
	select {
	case <-again.done:
	case <-time.After(body.watch):
		t.Error("a second close waited for a gate it no longer held")
	}
}

// closing closes a body, and says it has.
func (request *waiter) closing(body io.Closer) {
	request.done <- body.Close()
}

// Anti-test: a Get that fails lets the gate go at once, with the web's
// error and length.
func TestAFailedGetLetsTheGateGo(t *testing.T) {
	gate := gateOn(t, hourly(t))
	get := answering(t)
	gone := errors.New("gone")
	get.EXPECT().Get(paper).Return(nil, int64(unsaid), gone)
	through := pacedOn(context.Background(), t, get, gate)
	body, length, err := through.Get(paper)
	if body != nil || length != unsaid || err != gone {
		t.Errorf("%v %d %v", body, length, err)
	}
	next := waiting(through)
	go next.head()
	select {
	case err := <-next.done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(seconds(t, "patient-wait")):
		t.Error("the gate was held after a failed Get")
	}
}

// A Head and a Post go through the gate, and are paced like a Get.
func TestAHeadAndAPostArePaced(t *testing.T) {
	start, least, step := begun(t), seconds(t, "arxivs-least"),
		seconds(t, "step")
	clock := readings(t, start, start, start.Add(step), start.Add(least))
	clock.EXPECT().Sleep(gomock.Any(), least-step).Return(nil)
	gate := gateOn(t, clock)
	get := answering(t)
	form := url.Values{"id_list": {"1809.03507"}}
	get.EXPECT().Post(arxiv.API, form).Return("", nil)
	through := pacedOn(context.Background(), t, get, gate)
	headed(t, through)
	if where, err := through.Post(arxiv.API, form); where != "" ||
		err != nil {
		t.Errorf("%q, %v", where, err)
	}
	if gate.Asked() != twoRequests {
		t.Errorf("%d asked", gate.Asked())
	}
}

// Anti-test: a context that ends while the gate is held ends the wait
// for it, and nothing is asked.
func TestAContextEndingAtTheGateEndsTheWait(t *testing.T) {
	gate := gateOn(t, hourly(t))
	get := answering(t)
	get.EXPECT().Get(paper).DoAndReturn(served)
	body, _, err := pacedOn(context.Background(), t, get, gate).Get(paper)
	if err != nil {
		t.Fatal(err)
	}
	patience, end := context.WithCancel(context.Background())
	impatient := pacedOn(patience, t, get, gate)
	go end()
	if _, err := impatient.Head(paper); err != context.Canceled {
		t.Errorf("%v", err)
	}
	body.Close()
	if gate.Asked() != oneRequest {
		t.Errorf("%d asked", gate.Asked())
	}
}

// pacing is what Paced is given.
type pacing struct {
	ctx  context.Context
	get  web.Web
	gate *arxiv.Gate
}

// Anti-test: a paced web refuses a missing context, web or gate.
func TestAPacedWebRefusesWhatItNeeds(t *testing.T) {
	gate := gateOn(t, arxiv.Wall())
	get := mocks.NewMockWeb(gomock.NewController(t))
	var none context.Context
	for _, given := range []pacing{{none, get, gate},
		{context.Background(), nil, gate},
		{context.Background(), get, nil}} {
		made, err := arxiv.Paced(given.ctx, given.get, given.gate)
		if made != nil || !errors.Is(err, celestialdata.Refused()) {
			t.Errorf("%v, %v", made, err)
		}
	}
}

// The wall's clock is now, and its sleep waits so long.
func TestTheWallsClockIsNowAndWaits(t *testing.T) {
	before := time.Now()
	if err := arxiv.Wall().Sleep(context.Background(),
		time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if slept := arxiv.Wall().Now().Sub(before); slept < time.Millisecond {
		t.Errorf("slept %v", slept)
	}
}

// Anti-test: the wall's sleep ends with its context, with the context's
// error.
func TestTheWallsSleepEndsWithItsContext(t *testing.T) {
	if err := arxiv.Wall().Sleep(ended(), time.Hour); err !=
		context.Canceled {
		t.Errorf("%v", err)
	}
}

// slowBody is a body whose reading takes the clock past the pace, as a
// long download does.
type slowBody struct {
	content io.Reader
}

// Read reads what the body holds.
func (body slowBody) Read(into []byte) (int, error) {
	return body.content.Read(into)
}

// Close closes nothing.
func (slowBody) Close() error { return nil }

// Anti-test: the pace is held from the start of the request before, not
// its end: a request after one that took longer than the pace waits
// for nothing.
func TestThePaceIsHeldFromTheStartNotTheEnd(t *testing.T) {
	start, least := begun(t), seconds(t, "arxivs-least")
	finished := start.Add(least + least)
	gate := gateOn(t, readings(t, start, start, finished, finished))
	get := answering(t)
	content := strings.NewReader(pdf)
	get.EXPECT().Get(paper).Return(slowBody{content}, content.Size(),
		nil)
	through := pacedOn(context.Background(), t, get, gate)
	body, _, err := through.Get(paper)
	if err != nil {
		t.Fatal(err)
	}
	body.Close()
	headed(t, through)
}
