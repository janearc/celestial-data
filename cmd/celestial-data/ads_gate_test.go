package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/janearc/celestial-data/ads"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
)

// stillAt is a mock clock that stays at the time a test begins, and
// passes any sleep at once, so a gate on it never waits and never moves
// past a wait it was asked to keep.
func stillAt(control *gomock.Controller) *mocks.MockClock {
	clock := mocks.NewMockClock(control)
	clock.EXPECT().Now().Return(begun()).AnyTimes()
	clock.EXPECT().Sleep(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	return clock
}

// gatedADS is ads asked through ADS's gentle, its record kept beside the
// one given, on a clock that stays still. It has hosts, which is how the
// command knows it is run for real, and ADS is a mock.
func gatedADS(t *testing.T, record string) (command, *mocks.MockDoer) {
	t.Helper()
	tool, ask := askingADS(t, tokenKept(t))
	control := gomock.NewController(t)
	tool.hosts = web.NewHosts("", stillAt(control), web.Unix(), nil)
	tool.clock = stillAt(control)
	tool.record = record
	return tool, ask
}

// adsRecord is where ADS's record is kept for a record beside it: the
// same folder, as the command keeps it.
func adsRecord(record string) string {
	return web.RecordPath(filepath.Dir(record), "ads")
}

// stoppedBy is an answer of 429 with a Retry-After of two minutes, as
// ADS might send one.
func stoppedBy() *http.Response {
	return adsAnswer(http.StatusTooManyRequests, "", "Retry-After", "120")
}

// the test: a 429 that gives only X-RateLimit-Reset is kept until that
// reset, which the stop says, so the next run asks ADS nothing.
func TestADSIsLeftAloneUntilItsReset(t *testing.T) {
	record := recordIn(t)
	reset := begun().Add(3 * time.Hour)
	tool, ask := gatedADS(t, record)
	ask.EXPECT().Do(gomock.Any()).Return(adsAnswer(
		http.StatusTooManyRequests, "", "X-RateLimit-Reset",
		strconv.FormatInt(reset.Unix(), 10)), nil)
	_, errs, code := ran(tool, "ads", asked)
	if code == exitOK || !strings.Contains(errs, "it may be asked again "+
		"at "+reset.UTC().Format(time.RFC3339)) {
		t.Fatalf("exit %d: %s", code, errs)
	}
	next, _ := gatedADS(t, record)
	if _, errs, code = ran(next, "ads", asked); code == exitOK ||
		!strings.Contains(errs, "asked to be left until "+
			reset.UTC().Format(time.RFC3339)) {
		t.Errorf("the next run: exit %d: %s", code, errs)
	}
}

// the test: ads declares --i-mean-it, as the other verbs that ask a
// server do, and its help says what it does.
func TestADSTakesTheSwitch(t *testing.T) {
	help := helpOf(t, "ads")
	if !strings.Contains(help, "--i-mean-it") ||
		!strings.Contains(help, "asks a host again before its stop has "+
			"passed; says what it asked first") {
		t.Errorf("%q", help)
	}
}

// the test: when ADS asks us to stop, the stop says which host, why and
// until when, names the switch, and the wait is kept, so the next run
// asks ADS nothing and says so.
func TestADSStopsAndTheNextRunAsksNothing(t *testing.T) {
	record := recordIn(t)
	tool, ask := gatedADS(t, record)
	ask.EXPECT().Do(gomock.Any()).Return(stoppedBy(), nil)
	_, errs, code := ran(tool, "ads", asked)
	for _, says := range []string{"ADS asked us to stop",
		"its Retry-After is 120", "it may be asked again at",
		"to ask anyway, add --i-mean-it"} {
		if code == exitOK || !strings.Contains(errs, says) {
			t.Errorf("%q: exit %d: %s", says, code, errs)
		}
	}
	next, _ := gatedADS(t, record)
	_, errs, code = ran(next, "ads", asked)
	if code == exitOK || !strings.Contains(errs, "asked to be left "+
		"until") || !strings.Contains(errs, "--i-mean-it") {
		t.Errorf("the next run: exit %d: %s", code, errs)
	}
}

// the test: with --i-mean-it the next run says what ADS asked for, asks
// anyway, and the override is kept in the record beside the wait.
func TestADSAsksAnywayWhenToldTo(t *testing.T) {
	record := recordIn(t)
	first, ask := gatedADS(t, record)
	ask.EXPECT().Do(gomock.Any()).Return(stoppedBy(), nil)
	ran(first, "ads", asked)
	next, ask := gatedADS(t, record)
	ask.EXPECT().Do(gomock.Any()).Return(adsAnswer(http.StatusOK,
		twoFound), nil)
	out, errs, code := ran(next, "ads", asked, "--i-mean-it")
	kept, err := os.ReadFile(adsRecord(record))
	if code != exitOK || !strings.Contains(out, "3 papers match") ||
		!strings.Contains(errs, "ADS: ") ||
		!strings.Contains(errs, "its Retry-After was 120; asking "+
			"anyway, as told to") || err != nil ||
		!strings.Contains(string(kept), "overrode") {
		t.Errorf("exit %d: %s%s %q %v", code, out, errs, kept, err)
	}
}

// the anti-test: a run that is not told to, with the wait still kept,
// does not override it, and a run that is told to with no wait to
// override says nothing of one.
func TestADSOverridesNothingUnlessToldTo(t *testing.T) {
	tool, ask := gatedADS(t, recordIn(t))
	ask.EXPECT().Do(gomock.Any()).Return(adsAnswer(http.StatusOK,
		twoFound), nil)
	out, errs, code := ran(tool, "ads", asked, "--i-mean-it")
	kept, err := os.ReadFile(adsRecord(tool.record))
	if code != exitOK || !strings.Contains(out, "3 papers match") ||
		errs != "" || err != nil ||
		strings.Contains(string(kept), "overrode") {
		t.Errorf("exit %d: %s%s %q %v", code, out, errs, kept, err)
	}
}

// the test: the record is held around the verb and let go after it, so
// the pace carries to the next run and the lock is free.
func TestADSKeepsItsRecordAndLetsGoOfIt(t *testing.T) {
	record := recordIn(t)
	tool, ask := gatedADS(t, record)
	ask.EXPECT().Do(gomock.Any()).Return(adsAnswer(http.StatusOK,
		twoFound), nil)
	if _, errs, code := ran(tool, "ads", asked); code != exitOK {
		t.Fatalf("exit %d: %s", code, errs)
	}
	kept, err := os.ReadFile(adsRecord(record))
	if err != nil || !strings.HasPrefix(string(kept), "last ") {
		t.Errorf("%q, %v", kept, err)
	}
	holder, err := ads.NewGentle(stillAt(gomock.NewController(t)),
		web.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if err := holder.Hold(adsRecord(record)); err != nil {
		t.Errorf("the record was not let go: %v", err)
	}
}

// A second run shares ADS's record, waits out the first run's pace,
// and asks successfully while the first still holds its gentle.
func TestADSRunSharesAnotherRunsPace(t *testing.T) {
	record := recordIn(t)
	holder, err := ads.NewGentle(stillAt(gomock.NewController(t)),
		web.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if err := holder.Hold(adsRecord(record)); err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	if err := holder.Enter(context.Background()); err != nil {
		t.Fatal(err)
	}
	holder.Leave()
	tool, ask := gatedADS(t, record)
	clock := stillClock(gomock.NewController(t))
	clock.EXPECT().Sleep(gomock.Any(), time.Second).Return(nil)
	tool.clock = clock
	ask.EXPECT().Do(gomock.Any()).Return(adsAnswer(http.StatusOK,
		twoFound), nil)
	out, errs, code := ran(tool, "ads", asked)
	if code != exitOK || !strings.Contains(out, "3 papers match") ||
		errs != "" {
		t.Errorf("exit %d: %s%s", code, out, errs)
	}
}

// the test: ADS's record is kept beside the one a test gives, and with
// none given, in celestial-data's folder of the user's cache.
func TestADSKeepsItsRecordInTheCacheFolder(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("HOME", cache)
	t.Setenv("XDG_CACHE_HOME", "")
	beside := filepath.Join(t.TempDir(), "arxiv-gate")
	given := command{record: beside}
	if folder := given.adsFolder(); folder != filepath.Dir(beside) {
		t.Errorf("beside a record: %q", folder)
	}
	if folder := (command{}).adsFolder(); folder != cacheFolder() ||
		!strings.HasPrefix(folder, cache) {
		t.Errorf("in the cache: %q", folder)
	}
}

// the anti-test: with no cache folder to keep a record in, there is no
// folder, and none is kept.
func TestADSWithNoCacheFolderKeepsNoRecord(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	if folder := (command{}).adsFolder(); folder != "" {
		t.Errorf("%q", folder)
	}
}

// the anti-test: neither a stop, what an override says, the record on
// disk, nor anything ads writes holds the token or its header.
func TestADSWritesNoTokenThroughTheGentle(t *testing.T) {
	record := recordIn(t)
	first, ask := gatedADS(t, record)
	ask.EXPECT().Do(gomock.Any()).Return(stoppedBy(), nil)
	out, errs, _ := ran(first, "ads", asked)
	next, ask := gatedADS(t, record)
	ask.EXPECT().Do(gomock.Any()).Return(adsAnswer(http.StatusOK,
		twoFound), nil)
	nextOut, nextErrs, _ := ran(next, "ads", asked, "--i-mean-it")
	kept, err := os.ReadFile(adsRecord(record))
	if err != nil {
		t.Fatal(err)
	}
	all := out + errs + nextOut + nextErrs + string(kept)
	if !strings.Contains(nextErrs, "asking anyway") ||
		strings.Contains(all, adsToken) ||
		strings.Contains(strings.ToLower(all), "authorization") ||
		strings.Contains(strings.ToLower(all), "bearer") {
		t.Errorf("%s", all)
	}
}
