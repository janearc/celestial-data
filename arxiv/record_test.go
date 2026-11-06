package arxiv_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/arxiv"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
)

// The modes these tests give folders and files.
const (
	openFolder   = 0o755 // defined: written by its owner
	closedFolder = 0o500 // defined: read but not written, by its owner
	sealedFile   = 0o000 // defined: read by no one
	plainFile    = 0o644 // defined: written by its owner
)

// recordIn is where a gate's record is kept in a test's own folder.
func recordIn(t *testing.T) string {
	return filepath.Join(t.TempDir(), "celestial-data", "arxiv-gate")
}

// held is a gate on a clock, at arXiv's least pace, holding the record
// at path, failing the test when it will not.
func held(t *testing.T, clock arxiv.Clock, path string) *arxiv.Gate {
	t.Helper()
	gate := gateOn(t, clock)
	if err := gate.Hold(path); err != nil {
		t.Fatal(err)
	}
	return gate
}

// askOnce has a gate let one Head of the paper through, failing the
// test when it fails.
func askOnce(t *testing.T, gate *arxiv.Gate) {
	t.Helper()
	through := pacedOn(context.Background(), t, answering(t), gate)
	headed(t, through)
}

// released lets a gate go, failing the test when it will not.
func released(t *testing.T, gate *arxiv.Gate) {
	t.Helper()
	if err := gate.Release(); err != nil {
		t.Fatal(err)
	}
}

// A run keeps the pace from the start of the last request of the run
// before: its first request waits out what is left of it.
func TestARunKeepsThePaceOfTheRunBefore(t *testing.T) {
	path := recordIn(t)
	start, least, step := begun(t), seconds(t, "arxivs-least"),
		seconds(t, "step")
	before := held(t, readings(t, start, start), path)
	askOnce(t, before)
	released(t, before)
	clock := readings(t, start.Add(step), start.Add(least))
	clock.EXPECT().Sleep(gomock.Any(), least-step).Return(nil)
	after := held(t, clock, path)
	askOnce(t, after)
	released(t, after)
}

// Anti-test: a run with no record before it waits for nothing, and a
// gate that holds no record has nothing to release.
func TestAFirstRunWaitsForNothing(t *testing.T) {
	path := recordIn(t)
	start := begun(t)
	first := held(t, readings(t, start, start), path)
	askOnce(t, first)
	released(t, first)
	if err := gateOn(t, arxiv.Wall()).Release(); err != nil {
		t.Error(err)
	}
}

// halting is a server that refuses with a status and a Retry-After as
// given, or none when it is empty.
type halting struct {
	code  int
	retry string
}

// ServeHTTP refuses with the status, and the Retry-After when there is
// one.
func (refuse halting) ServeHTTP(answer http.ResponseWriter,
	_ *http.Request) {
	if refuse.retry != "" {
		answer.Header().Set("Retry-After", refuse.retry)
	}
	answer.WriteHeader(refuse.code)
}

// halted is a web's refusal from a server that refuses so.
func halted(t *testing.T, refuse halting) error {
	t.Helper()
	server := httptest.NewServer(refuse)
	t.Cleanup(server.Close)
	patience := seconds(t, "patient-wait")
	_, _, err := web.New(patience, patience, "celestial-data-test").
		Get(server.URL)
	return err
}

// stoppedAt has a gate on a run's clock, reading start, be refused so
// for a PDF, and lets it go, keeping the wait in the record at path.
func stoppedAt(t *testing.T, path string, start time.Time,
	refused error) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Get(paper).Return(nil, int64(noPaper), refused)
	gate := held(t, readings(t, start, start, start), path)
	pacedOn(context.Background(), t, get, gate).Get(paper)
	released(t, gate)
}

// asksAt is what a run says of a Head, its clock reading the times
// given: arXiv's refusal of it, or none when it went through.
func asksAt(t *testing.T, path string, times ...time.Time) error {
	gate := held(t, readings(t, times...), path)
	_, err := pacedOn(context.Background(), t, answering(t), gate).
		Head(paper)
	released(t, gate)
	return err
}

// waitAsked is a wait arXiv asked for, and how long after it was asked
// it ends.
type waitAsked struct {
	retry string
	lasts time.Duration
}

// A wait arXiv asked for carries to the next run: a run before it has
// passed sends nothing and says when it ends, and one just as it ends
// asks. The wait is so many seconds, a date, or, when none is said or
// it will not read, the rest the gate picks.
func TestAWaitAskedCarriesToTheNextRun(t *testing.T) {
	start, least := begun(t), seconds(t, "arxivs-least")
	asked, rest := seconds(t, "asked-wait"), seconds(t, "rest-unsaid")
	date := start.Add(time.Hour).Format(http.TimeFormat)
	for _, each := range []waitAsked{{wait, asked}, {date, time.Hour},
		{"", rest}, {"-5", rest}, {"soon", rest}, {"0", noWait}} {
		path := recordIn(t)
		refused := halted(t, halting{http.StatusTooManyRequests,
			each.retry})
		stoppedAt(t, path, start, refused)
		ends := start.Add(each.lasts)
		early := asksAt(t, path, ends.Add(-time.Nanosecond))
		if !arxiv.AskedToStop(early) || !strings.Contains(early.Error(),
			"left until "+ends.Format(time.RFC3339)) {
			t.Errorf("%q: %v", each.retry, early)
		}
		// a wait shorter than the pace still has the pace to keep
		allowed := ends
		if allowed.Before(start.Add(least)) {
			allowed = start.Add(least)
		}
		if err := asksAt(t, path, allowed, allowed); err != nil {
			t.Errorf("%q: %v", each.retry, err)
		}
	}
}

// noWait is a Retry-After of none. It's defined: arXiv may be asked
// again at once.
const noWait time.Duration = 0

// Anti-test: while one run holds the record, another is refused, saying
// the lock; when the first lets go, the next holds it.
func TestASecondRunWhileOneHoldsIsRefused(t *testing.T) {
	path := recordIn(t)
	first := held(t, arxiv.Wall(), path)
	second := gateOn(t, arxiv.Wall())
	err := second.Hold(path)
	if !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "another run is asking arXiv: "+
			"it holds "+path+".lock") {
		t.Errorf("%v", err)
	}
	released(t, first)
	if err := second.Hold(path); err != nil {
		t.Error(err)
	}
	released(t, second)
}

// Anti-test: a record that is not the gate's is refused, saying how to
// clear it, and the lock is let go; a line the gate does not know is
// passed over.
func TestARecordNotTheGatesIsRefused(t *testing.T) {
	path := recordIn(t)
	if err := os.MkdirAll(filepath.Dir(path), openFolder); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("last yesterday\n"),
		plainFile); err != nil {
		t.Fatal(err)
	}
	err := gateOn(t, arxiv.Wall()).Hold(path)
	if !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "is not the gate's record") {
		t.Errorf("%v", err)
	}
	known := "other " + begun(t).Format(time.RFC3339Nano) + "\n"
	if err := os.WriteFile(path, []byte(known), plainFile); err != nil {
		t.Fatal(err)
	}
	again := held(t, arxiv.Wall(), path)
	released(t, again)
}

// closedTo makes a folder one its owner cannot write in, until the test
// ends.
func closedTo(t *testing.T, folder string) {
	t.Helper()
	if err := os.Chmod(folder, closedFolder); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(opener(folder).open)
}

// opener opens a folder again.
type opener string

// open lets the folder's owner write in it again.
func (folder opener) open() { os.Chmod(string(folder), openFolder) }

// Anti-test: a gate that cannot make its record's folder, take its
// lock, or read its record says why, and holds nothing.
func TestAHoldThatCannotBeTakenSaysWhy(t *testing.T) {
	plain := filepath.Join(t.TempDir(), "plain")
	if err := os.WriteFile(plain, nil, plainFile); err != nil {
		t.Fatal(err)
	}
	locked := recordIn(t)
	if err := os.MkdirAll(filepath.Dir(locked), openFolder); err != nil {
		t.Fatal(err)
	}
	closedTo(t, filepath.Dir(locked))
	sealed := recordIn(t)
	if err := os.MkdirAll(filepath.Dir(sealed), openFolder); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sealed, nil, sealedFile); err != nil {
		t.Fatal(err)
	}
	folder := recordIn(t)
	if err := os.MkdirAll(folder, openFolder); err != nil {
		t.Fatal(err)
	}
	for path, says := range map[string]string{
		filepath.Join(plain, "record"): "mkdir " + plain,
		locked:                         "open " + locked + ".lock",
		sealed:                         "open " + sealed + ": permission",
		folder:                         "is a directory",
	} {
		err := gateOn(t, arxiv.Wall()).Hold(path)
		if err == nil || !strings.Contains(err.Error(), says) {
			t.Errorf("%s: %v", path, err)
		}
		again := gateOn(t, arxiv.Wall()).Hold(path)
		if again == nil || strings.Contains(again.Error(), "another run") {
			t.Errorf("%s was left locked: %v", path, again)
		}
	}
}

// unmasked is a test's umask, put back when the test ends.
type unmasked int

// restore puts the umask back.
func (mask unmasked) restore() { syscall.Umask(int(mask)) }

// usualMask is the umask the mode test runs under. It's defined: none,
// so a file or folder has the mode it was made with, every bit of it.
const usualMask = 0o000

// The modes the record's folder and file are made with, under the
// usual mask. They're defined: written by their owner, read by others.
const (
	recordFolder os.FileMode = 0o755 // defined: as above
	recordFile   os.FileMode = 0o644 // defined: as above
)

// The record's folder and file are written by their owner and read by
// others, whatever the run's umask lets through of that.
func TestTheRecordIsTheOwnersToWrite(t *testing.T) {
	before := unmasked(syscall.Umask(usualMask))
	t.Cleanup(before.restore)
	path := recordIn(t)
	gate := held(t, arxiv.Wall(), path)
	released(t, gate)
	folder, folderErr := os.Stat(filepath.Dir(path))
	file, fileErr := os.Stat(path)
	if folderErr != nil || fileErr != nil ||
		folder.Mode().Perm() != recordFolder ||
		file.Mode().Perm() != recordFile {
		t.Errorf("%v %v, %v %v", folder, folderErr, file, fileErr)
	}
}

// Anti-test: a record that cannot be written says why when the gate is
// let go: a folder no longer written in, or a folder where it should be.
func TestARecordThatCannotBeWrittenSaysWhy(t *testing.T) {
	shut := recordIn(t)
	gate := held(t, arxiv.Wall(), shut)
	closedTo(t, filepath.Dir(shut))
	if err := gate.Release(); err == nil ||
		!strings.Contains(err.Error(), "open "+shut+".part") {
		t.Errorf("a record was written in a closed folder: %v", err)
	}
	blocked := recordIn(t)
	gate = held(t, arxiv.Wall(), blocked)
	inside := filepath.Join(blocked, "inside")
	if err := os.MkdirAll(inside, openFolder); err != nil {
		t.Fatal(err)
	}
	if err := gate.Release(); err == nil {
		t.Error("a record was put over a folder")
	}
}
