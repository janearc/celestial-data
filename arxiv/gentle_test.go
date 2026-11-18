package arxiv_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/arxiv"
	"github.com/janearc/celestial-data/mocks"
	"go.uber.org/mock/gomock"
)

// timedOut is a network error that is a timeout, which is a failure with
// no word from the host.
type timedOut struct{}

// Error says what happened.
func (timedOut) Error() string { return "timed out" }

// Timeout is true: it was a timeout.
func (timedOut) Timeout() bool { return true }

// Temporary is false, as it is for most errors.
func (timedOut) Temporary() bool { return false }

// arXiv retries nothing: a request that times out is asked once, and the
// error is given as it came. That is how this adapter has always asked,
// kept on purpose in arXiv's numbers as tries.
func TestAFailureWithNoWordIsNotAskedAgain(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Get(paper).Return(nil, int64(unsaid), timedOut{})
	through := pacedOn(context.Background(), t, get, gateOn(t, hourly(t)))
	_, _, err := through.Get(paper)
	if !errors.Is(err, timedOut{}) || arxiv.AskedToStop(err) {
		t.Errorf("%v", err)
	}
}

// arXiv is never left alone for the run because of failures with no
// word: however many in a row, the next is asked. That is how this
// adapter has always asked, kept on purpose in arXiv's numbers as opens.
func TestFailuresWithNoWordNeverOpenACircuit(t *testing.T) {
	const failures = 5
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Get(paper).Return(nil, int64(unsaid), timedOut{}).
		Times(failures)
	through := pacedOn(context.Background(), t, get, gateOn(t, hourly(t)))
	for range failures {
		_, _, err := through.Get(paper)
		if !errors.Is(err, timedOut{}) {
			t.Errorf("%v", err)
		}
	}
}

// Anti-test: a folder that will not open is no refusal, and a record
// that is not the gate's is.
func TestHoldRefusesOnlyWhatItAlwaysDid(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	err := gateOn(t, arxiv.Wall()).Hold(filepath.Join(blocker, "record"))
	if err == nil || errors.Is(err, celestialdata.Refused()) {
		t.Errorf("a folder that will not open was a refusal: %v", err)
	}
	notOurs := filepath.Join(t.TempDir(), "record")
	if err := os.WriteFile(notOurs, []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = gateOn(t, arxiv.Wall()).Hold(notOurs)
	if !errors.Is(err, celestialdata.Refused()) {
		t.Errorf("%v", err)
	}
}
