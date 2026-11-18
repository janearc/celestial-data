package web_test

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/janearc/celestial-data/web"
)

// plainFile is the mode of the files the tests write. It's defined: read
// and written by its owner.
const plainFile = 0o644

// recordIn is where a test keeps its record.
func recordIn(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "example-gate")
}

// heldAt is the example host's gentle holding the record for the run at
// path, on a clock at a time, with the gentle that clock keeps.
func heldAt(t *testing.T, path string, at time.Time) (*web.Gentle,
	*moment) {
	t.Helper()
	clock, keeping := clockAt(t, at)
	gentle := gentleOn(t, heldExample(t), clock)
	if err := gentle.Hold(path); err != nil {
		t.Fatal(err)
	}
	return gentle, keeping
}

// released is a gentle's record written and its lock let go.
func released(t *testing.T, gentle *web.Gentle) {
	t.Helper()
	if err := gentle.Release(); err != nil {
		t.Fatal(err)
	}
}

// recordOf is the text of a record.
func recordOf(t *testing.T, path string) string {
	t.Helper()
	return string(readFile(t, path))
}

// The record holds when the last request began and the time the host
// asked to be left until, in the lines the gate has always kept.
func TestTheRecordHoldsTheLastStartAndTheTimeAskedFor(t *testing.T) {
	path := recordIn(t)
	gentle, _ := heldAt(t, path, start)
	refuse(t, gentle, http.StatusTooManyRequests, "120")
	released(t, gentle)
	want := "last " + start.Format(time.RFC3339Nano) + "\nuntil " +
		start.Add(2*time.Minute).Format(time.RFC3339Nano) + "\nasked 120\n"
	if got := recordOf(t, path); got != want {
		t.Errorf("%q, not %q", got, want)
	}
}

// Anti-test: a refusal that named no Retry-After keeps the two lines the
// record has always had, and no more.
func TestARefusalWithNoRetryAfterKeepsOnlyTheTwoOldLines(t *testing.T) {
	path := recordIn(t)
	gentle, _ := heldAt(t, path, start)
	refuse(t, gentle, http.StatusTooManyRequests, "")
	released(t, gentle)
	want := "last " + start.Format(time.RFC3339Nano) + "\nuntil " +
		start.Add(300*time.Second).Format(time.RFC3339Nano) + "\n"
	if got := recordOf(t, path); got != want {
		t.Errorf("%q, not %q", got, want)
	}
}

// Blank lines in a record are ignored.
func TestBlankLinesInARecordAreIgnored(t *testing.T) {
	path := recordIn(t)
	text := "\nlast " + start.Format(time.RFC3339Nano) + "\n  \n\n"
	if err := os.WriteFile(path, []byte(text), plainFile); err != nil {
		t.Fatal(err)
	}
	gentle, keeping := heldAt(t, path, start)
	askOnce(t, gentle)
	released(t, gentle)
	if len(keeping.slept) != 1 {
		t.Errorf("the last start was not read: %v", keeping.slept)
	}
}

// A wait kept stops the next run, which asks nothing until it has
// passed.
func TestAWaitKeptStopsTheNextRun(t *testing.T) {
	path := recordIn(t)
	gentle, _ := heldAt(t, path, start)
	refuse(t, gentle, http.StatusTooManyRequests, "120")
	released(t, gentle)
	next, _ := heldAt(t, path, start.Add(time.Minute))
	stoppedBy(t, next)
	released(t, next)
}

// Anti-test: a wait that has passed asks again, and the pace from the
// last run's last request is kept.
func TestAWaitPassedAsksAgainAndThePaceCarriesOver(t *testing.T) {
	path := recordIn(t)
	gentle, _ := heldAt(t, path, start)
	refuse(t, gentle, http.StatusTooManyRequests, "1")
	released(t, gentle)
	next, keeping := heldAt(t, path, start.Add(2*time.Second))
	askOnce(t, next)
	released(t, next)
	if len(keeping.slept) != 1 || keeping.slept[0] != time.Second {
		t.Errorf("%v", keeping.slept)
	}
}

// An override is kept in the record beside the wait it overrode, and the
// next run keeps it.
func TestAnOverrideIsKeptBesideTheWaitItOverrode(t *testing.T) {
	path := recordIn(t)
	gentle, _ := heldAt(t, path, start)
	refuse(t, gentle, http.StatusTooManyRequests, "120")
	released(t, gentle)
	at := start.Add(time.Minute)
	next, _ := heldAt(t, path, at)
	var said bytes.Buffer
	next.Insist(&said)
	askOnce(t, next)
	released(t, next)
	until := start.Add(2 * time.Minute).Format(time.RFC3339Nano)
	got := recordOf(t, path)
	if !strings.Contains(got, "until "+until+"\nasked 120\n") ||
		!strings.Contains(got, "overrode "+at.Format(time.RFC3339Nano)+
			" "+until+" 120\n") ||
		!strings.Contains(said.String(), "its Retry-After was 120") {
		t.Errorf("%q, %q", got, said.String())
	}
	third, _ := heldAt(t, path, at.Add(time.Hour))
	askOnce(t, third)
	released(t, third)
	if strings.Count(recordOf(t, path), "overrode ") != 1 {
		t.Errorf("an earlier override was lost: %q", recordOf(t, path))
	}
}

// An override of the circuit is kept as one, with no wait.
func TestAnOverrideOfTheCircuitIsKeptAsOne(t *testing.T) {
	path := recordIn(t)
	gentle, _ := heldAt(t, path, start)
	gentle.GiveUp()
	gentle.GiveUp()
	gentle.Insist(&bytes.Buffer{})
	askOnce(t, gentle)
	released(t, gentle)
	want := " " + time.Time{}.Format(time.RFC3339Nano) + " circuit\n"
	if !strings.Contains(recordOf(t, path), want) {
		t.Errorf("%q", recordOf(t, path))
	}
}

// Anti-test: with no override, a record has no overrode line; and it
// keeps only the newest overrides, as many as the policy says.
func TestNoOverrideIsNoLineAndOnlyTheNewestAreKept(t *testing.T) {
	path := recordIn(t)
	gentle, _ := heldAt(t, path, start)
	askOnce(t, gentle)
	released(t, gentle)
	if strings.Contains(recordOf(t, path), "overrode") {
		t.Errorf("%q", recordOf(t, path))
	}
	policy := example(t)
	policy.OverridesKept = 1
	for run := range 2 {
		clock, _ := clockAt(t, start.Add(time.Duration(run)*time.Hour))
		again := gentleOn(t, policy, clock)
		if err := again.Hold(path); err != nil {
			t.Fatal(err)
		}
		again.GiveUp()
		again.GiveUp()
		again.Insist(&bytes.Buffer{})
		askOnce(t, again)
		released(t, again)
	}
	if strings.Count(recordOf(t, path), "overrode ") != 1 {
		t.Errorf("%q", recordOf(t, path))
	}
}

// Anti-test: overrides are capped in memory as well as in the record: a
// run that overrides twice, with the policy keeping one, holds one.
func TestOverridesAreCappedInMemoryToo(t *testing.T) {
	path := recordIn(t)
	policy := example(t)
	policy.OverridesKept = 1
	clock, _ := clockAt(t, start)
	gentle := gentleOn(t, policy, clock)
	if err := gentle.Hold(path); err != nil {
		t.Fatal(err)
	}
	gentle.Insist(&bytes.Buffer{})
	for range 2 {
		gentle.GiveUp()
		gentle.GiveUp()
		askOnce(t, gentle)
		gentle.Judge(http.StatusOK, "", nil)
	}
	released(t, gentle)
	if strings.Count(recordOf(t, path), "overrode ") != 1 {
		t.Errorf("%q", recordOf(t, path))
	}
}

// A wait kept with no Retry-After is overridden as one that named none.
func TestAnOverrideOfAWaitThatNamedNoRetryAfterSaysNone(t *testing.T) {
	path := recordIn(t)
	text := "last " + start.Format(time.RFC3339Nano) + "\nuntil " +
		start.Add(time.Hour).Format(time.RFC3339Nano) + "\n"
	if err := os.WriteFile(path, []byte(text), plainFile); err != nil {
		t.Fatal(err)
	}
	gentle, _ := heldAt(t, path, start)
	var said bytes.Buffer
	gentle.Insist(&said)
	askOnce(t, gentle)
	released(t, gentle)
	if !strings.Contains(said.String(), "it named no Retry-After") ||
		!strings.Contains(recordOf(t, path), " none\n") {
		t.Errorf("%q", said.String())
	}
}

// What the host wrote as its Retry-After may hold spaces, and is kept as
// it was written.
func TestAnOverrideLineKeepsARetryAfterWithSpaces(t *testing.T) {
	path := recordIn(t)
	line := "overrode " + start.Format(time.RFC3339Nano) + " " +
		start.Format(time.RFC3339Nano) + " Wed, 21 Oct 2026 07:28:00 GMT\n"
	if err := os.WriteFile(path, []byte(line), plainFile); err != nil {
		t.Fatal(err)
	}
	gentle, _ := heldAt(t, path, start)
	released(t, gentle)
	if !strings.Contains(recordOf(t, path), "Wed, 21 Oct 2026 07:28:00 GMT\n") {
		t.Errorf("%q", recordOf(t, path))
	}
}

// Anti-test: a record that is not the gentle's is refused, whatever its
// name, saying so, and the lock is let go so a second try is no worse.
func TestARecordNotTheGentlesIsRefused(t *testing.T) {
	when := start.Format(time.RFC3339Nano)
	for name, text := range map[string]string{
		"a time that is not":   "last yesterday\n",
		"a name unknown":       "next " + when + "\nlast soon\n",
		"an override of two":   "overrode " + when + " " + when + "\n",
		"an override no time":  "overrode soon " + when + " none\n",
		"an override no until": "overrode " + when + " soon none\n",
	} {
		path := recordIn(t)
		if err := os.WriteFile(path, []byte(text), plainFile); err != nil {
			t.Fatal(err)
		}
		clock, _ := clockAt(t, start)
		gentle := gentleOn(t, example(t), clock)
		err := gentle.Hold(path)
		if err == nil || !strings.Contains(err.Error(),
			"is not the gate's record") {
			t.Errorf("%s: %v", name, err)
		}
		if err := gentle.Hold(path); err == nil ||
			strings.Contains(err.Error(), "another run") {
			t.Errorf("%s, again: %v", name, err)
		}
	}
}

// A name the gentle does not know, with a time it can read, is ignored.
func TestALineOfAnUnknownNameWithATimeIsIgnored(t *testing.T) {
	path := recordIn(t)
	text := "next " + start.Format(time.RFC3339Nano) + "\n"
	if err := os.WriteFile(path, []byte(text), plainFile); err != nil {
		t.Fatal(err)
	}
	gentle, _ := heldAt(t, path, start)
	askOnce(t, gentle)
	released(t, gentle)
}

// A second run is refused while one holds the record, and may hold it
// once the first has released it.
func TestASecondRunWhileOneHoldsIsRefused(t *testing.T) {
	path := recordIn(t)
	first, _ := heldAt(t, path, start)
	clock, _ := clockAt(t, start)
	second := gentleOn(t, heldExample(t), clock)
	err := second.Hold(path)
	if err == nil || !strings.Contains(err.Error(),
		"another run is asking Example: it holds "+path+".lock") {
		t.Errorf("%v", err)
	}
	released(t, first)
	if err := second.Hold(path); err != nil {
		t.Errorf("after the release: %v", err)
	}
}

// A second run's refusal, and a record that is not a record, are
// RecordRefused, for a caller to tell from a failure to open.
func TestARefusedRecordIsARecordRefused(t *testing.T) {
	path := recordIn(t)
	first, _ := heldAt(t, path, start)
	// The first run is kept alive to the end of the test. Were it
	// collected, its lock file would close and let the second run in.
	defer runtime.KeepAlive(first)
	clock, _ := clockAt(t, start)
	var refused *web.RecordRefused
	err := gentleOn(t, heldExample(t), clock).Hold(path)
	if !errors.As(err, &refused) || refused.Error() != refused.Why {
		t.Errorf("another run: %v", err)
	}
	notOurs := recordIn(t)
	if err := os.WriteFile(notOurs, []byte("hello\n"), plainFile); err != nil {
		t.Fatal(err)
	}
	err = gentleOn(t, heldExample(t), clock).Hold(notOurs)
	if !errors.As(err, &refused) {
		t.Errorf("not a record: %v", err)
	}
}

// Anti-test: a folder that will not open is no RecordRefused.
func TestAFailureToOpenIsNotARecordRefused(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, plainFile); err != nil {
		t.Fatal(err)
	}
	clock, _ := clockAt(t, start)
	var refused *web.RecordRefused
	err := gentleOn(t, heldExample(t), clock).Hold(filepath.Join(file, "gate"))
	if err == nil || errors.As(err, &refused) {
		t.Errorf("%v", err)
	}
}

// A gentle that holds no record has nothing to release.
func TestAGentleWithNoRecordHasNothingToRelease(t *testing.T) {
	gentle, _ := exampleAt(t)
	askOnce(t, gentle)
	released(t, gentle)
}

// Anti-test: a record that cannot be made, locked or read is refused,
// saying why.
func TestARecordThatCannotBeMadeLockedOrReadIsRefused(t *testing.T) {
	clock, _ := clockAt(t, start)
	gentle := gentleOn(t, heldExample(t), clock)
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, plainFile); err != nil {
		t.Fatal(err)
	}
	if err := gentle.Hold(filepath.Join(file, "under", "gate")); err == nil {
		t.Error("a record under a file")
	}
	shut := t.TempDir()
	if err := os.Chmod(shut, 0o555); err != nil {
		t.Fatal(err)
	}
	if err := gentle.Hold(filepath.Join(shut, "gate")); err == nil {
		t.Error("a lock in a folder that cannot be written")
	}
	hidden := recordIn(t)
	if err := os.WriteFile(hidden, nil, 0); err != nil {
		t.Fatal(err)
	}
	if err := gentle.Hold(hidden); err == nil {
		t.Error("a record that cannot be read")
	}
	folder := recordIn(t)
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := gentle.Hold(folder); err == nil {
		t.Error("a record that is a folder")
	}
}

// A record that cannot be written stops the request that would have
// noted its start, and holds nothing; and when the release cannot write,
// it says so and still lets the lock go.
func TestARecordThatCannotBeWrittenStopsTheRequestAndTheRelease(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "example-gate")
	gentle, _ := heldAt(t, path, start)
	if err := os.Chmod(folder, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(folder, 0o755)
	err := gentle.Enter(t.Context())
	if err == nil || !strings.Contains(err.Error(), "open "+path+".part") {
		t.Errorf("%v", err)
	}
	if web.Holding(gentle) || gentle.Asked() != 0 {
		t.Error("a request that was not noted holds the gentle")
	}
	if err := gentle.Release(); err == nil {
		t.Error("a release that could not write")
	}
	if err := gentle.Release(); err != nil {
		t.Errorf("a second release: %v", err)
	}
}

// A record that cannot be put in its place is an error, and no part is
// left as the record.
func TestARecordThatCannotBePutInItsPlaceIsAnError(t *testing.T) {
	path := recordIn(t)
	gentle, _ := heldAt(t, path, start)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := gentle.Enter(t.Context()); err == nil {
		t.Error("written over a folder")
	}
}
