package arxiv_test

import (
	"context"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/janearc/celestial-data/arxiv"
	"github.com/janearc/celestial-data/mocks"
	"go.uber.org/mock/gomock"
)

// releasing lets a gate go when a test ends, whatever it says then.
type releasing struct{ gate *arxiv.Gate }

// release lets the gate go.
func (holding releasing) release() { holding.gate.Release() }

// heldToTheEnd is a gate holding the record at path until the test
// ends, as a run that is stopped before it lets go does.
func heldToTheEnd(t *testing.T, clock arxiv.Clock,
	path string) *arxiv.Gate {
	t.Helper()
	gate := held(t, clock, path)
	t.Cleanup(releasing{gate}.release)
	return gate
}

// A lock's file left by a run that is gone is no lock: the system let
// the lock go when that run ended, and the next run holds it.
func TestALockLeftByARunThatIsGoneIsNoLock(t *testing.T) {
	path := recordIn(t)
	if err := os.MkdirAll(filepath.Dir(path), openFolder); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".lock", nil, plainFile); err != nil {
		t.Fatal(err)
	}
	gate := held(t, arxiv.Wall(), path)
	released(t, gate)
}

// The record is written as each request begins, so a run stopped
// before it lets go of the gate has kept when its last request began.
func TestTheRecordIsWrittenAsEachRequestBegins(t *testing.T) {
	path := recordIn(t)
	start := begun(t)
	gate := heldToTheEnd(t, readings(t, start, start), path)
	askOnce(t, gate)
	written, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(written),
		"last "+start.Format(time.RFC3339Nano)) {
		t.Errorf("%q, %v", written, err)
	}
}

// Anti-test: a request whose start will not write to the record is not
// sent, and says why.
func TestARequestWhoseRecordWillNotWriteIsNotSent(t *testing.T) {
	path := recordIn(t)
	start := begun(t)
	gate := heldToTheEnd(t, readings(t, start, start), path)
	closedTo(t, filepath.Dir(path))
	quiet := mocks.NewMockWeb(gomock.NewController(t))
	through := pacedOn(context.Background(), t, quiet, gate)
	_, err := through.Head(paper)
	if err == nil || !strings.Contains(err.Error(), "open "+path+".part") ||
		gate.Asked() != noRequest {
		t.Errorf("%v, %d asked", err, gate.Asked())
	}
}

// Anti-test: a record whose last start is ahead of the clock, as a
// clock set wrong leaves one, holds a run no longer than the pace.
func TestARecordFromTheFutureWaitsNoMoreThanThePace(t *testing.T) {
	path := recordIn(t)
	start, least := begun(t), seconds(t, "arxivs-least")
	if err := os.MkdirAll(filepath.Dir(path), openFolder); err != nil {
		t.Fatal(err)
	}
	ahead := "last " + start.Add(time.Hour).Format(time.RFC3339Nano) +
		"\n"
	if err := os.WriteFile(path, []byte(ahead), plainFile); err != nil {
		t.Fatal(err)
	}
	clock := readings(t, start, start.Add(least))
	clock.EXPECT().Sleep(gomock.Any(), least).Return(nil)
	gate := held(t, clock, path)
	askOnce(t, gate)
	released(t, gate)
}

// longest is the longest wait a time.Duration holds, in whole seconds.
// It's defined by time.Duration.
const longest = time.Duration(math.MaxInt64/int64(time.Second)) *
	time.Second

// Anti-test: a Retry-After longer than a time.Duration holds is not
// lost to an overflow; it is kept as the longest one does hold.
func TestAWaitTooLongForATimeIsKeptAsLongAsItCan(t *testing.T) {
	path := recordIn(t)
	start := begun(t)
	refused := halted(t, halting{http.StatusTooManyRequests,
		"99999999999999"})
	stoppedAt(t, path, start, refused)
	later := asksAt(t, path, start.Add(time.Hour))
	until := start.Add(longest).Format(time.RFC3339)
	if !arxiv.AskedToStop(later) || !strings.Contains(later.Error(),
		"left until "+until) {
		t.Errorf("%v", later)
	}
}
