package arxiv_test

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/janearc/celestial-data/arxiv"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"github.com/janearc/puzzlemouth/numbers"
	"go.uber.org/mock/gomock"
)

// numbersFile is where the numbers these tests are held to are kept.
const numbersFile = "testdata/numbers.yaml"

// seconds is a number of seconds from the numbers file, as a time.
func seconds(t *testing.T, name string) time.Duration {
	t.Helper()
	raw, err := os.ReadFile(numbersFile)
	if err != nil {
		t.Fatal(err)
	}
	held, err := numbers.Read(numbersFile, raw)
	if err != nil {
		t.Fatal(err)
	}
	number, err := held.Get(name)
	if err != nil {
		t.Fatal(err)
	}
	return web.Seconds(number.Value)
}

// morning is when the clocks in these tests begin. It's defined: any
// time will do.
const morning = "2026-11-06T09:00:00Z"

// begun is the morning, as a time.
func begun(t *testing.T) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339, morning)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

// ticking is a time that moves on by a step each time it is read.
type ticking struct {
	now  time.Time
	step time.Duration
}

// read is the time, a step on from the last reading.
func (clock *ticking) read() time.Time {
	clock.now = clock.now.Add(clock.step)
	return clock.now
}

// hourly is a mock clock an hour later each time it is read, so a gate
// on it never waits.
func hourly(t *testing.T) *mocks.MockClock {
	clock := mocks.NewMockClock(gomock.NewController(t))
	hours := &ticking{now: begun(t), step: time.Hour}
	clock.EXPECT().Now().DoAndReturn(hours.read).AnyTimes()
	return clock
}

// readings is a mock clock whose Now gives each time in turn.
func readings(t *testing.T, times ...time.Time) *mocks.MockClock {
	clock := mocks.NewMockClock(gomock.NewController(t))
	var calls []any
	for _, at := range times {
		calls = append(calls, clock.EXPECT().Now().Return(at))
	}
	gomock.InOrder(calls...)
	return clock
}

// gateOn is a gate at arXiv's least pace, as these tests read it, on a
// clock. The test is watched: a request that hangs at the gate stops
// it, rather than the suite waiting until it is killed.
func gateOn(t *testing.T, clock arxiv.Clock) *arxiv.Gate {
	t.Helper()
	gate, err := arxiv.NewGate(seconds(t, "arxivs-least"), clock)
	if err != nil {
		t.Fatal(err)
	}
	watch := &watchdog{name: t.Name()}
	watch.timer = time.AfterFunc(seconds(t, "patient-wait"), watch.bark)
	t.Cleanup(watch.stop)
	return gate
}

// watchdog watches a test for a request that hangs at the gate.
type watchdog struct {
	name  string
	timer *time.Timer
}

// bark stops every test, saying which one hung. A test that hangs can
// be stopped no other way from outside it.
func (watch *watchdog) bark() {
	panic(watch.name + " waited past its patience: a request hung at " +
		"the gate")
}

// stop stops the watch when the test is done.
func (watch *watchdog) stop() { watch.timer.Stop() }

// pacedOn is a paced web over a web and a gate, waiting while ctx lasts.
func pacedOn(ctx context.Context, t *testing.T, get web.Web,
	gate *arxiv.Gate) web.Web {
	t.Helper()
	through, err := arxiv.Paced(ctx, get, gate)
	if err != nil {
		t.Fatal(err)
	}
	return through
}

// paper is the address of a paper's PDF these tests ask for.
const paper = "https://export.arxiv.org/pdf/1809.03507v2"

// pdf is what a paper holds in these tests.
const pdf = "%PDF-1.5 a paper"

// served is the paper, as a web serves it.
func served(string) (io.ReadCloser, int64, error) {
	content := strings.NewReader(pdf)
	return io.NopCloser(content), content.Size(), nil
}

// answering is a mock web that answers a Head of the paper with its
// length, as often as it is asked.
func answering(t *testing.T) *mocks.MockWeb {
	get := mocks.NewMockWeb(gomock.NewController(t))
	length := strings.NewReader(pdf).Size()
	get.EXPECT().Head(paper).Return(length, nil).AnyTimes()
	return get
}

// headed is a Head of the paper through a web, failing the test when it
// fails.
func headed(t *testing.T, through web.Web) {
	t.Helper()
	if _, err := through.Head(paper); err != nil {
		t.Fatal(err)
	}
}

// ended is a context that has ended.
func ended() context.Context {
	ctx, end := context.WithCancel(context.Background())
	end()
	return ctx
}
