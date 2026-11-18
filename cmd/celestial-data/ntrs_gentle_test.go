package main

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
)

// The pace NTRS is asked at and the pace a host with no policy of its
// own is, as their numbers files say. They're the numbers' own values,
// said again so a test shows which of the two a host got.
const (
	ntrsPace    = 2 * time.Second
	defaultPace = time.Second
)

// stillClock is a mock clock that stands still, so each request after the
// first is a pace behind the last, and the sleep it asks for is the pace.
func stillClock(control *gomock.Controller) *mocks.MockClock {
	clock := mocks.NewMockClock(control)
	clock.EXPECT().Now().Return(begun()).AnyTimes()
	return clock
}

// gently is celestial-data asking NTRS through its hosts, as run gives
// them, on a clock a test holds, with NTRS asked through a mock Doer and
// PDFs through a mock web.
type gently struct {
	command
	clock *mocks.MockClock
	get   *mocks.MockWeb
	doer  *mocks.MockDoer
}

// askingNTRSGently is NTRS asked through hosts that keep their records in
// folder, with the policies given: ntrsPolicies for what run gives, or
// none for a host with no policy of its own.
func askingNTRSGently(t *testing.T, folder string, hosts func(string,
	web.Clock, web.Random) (*web.Hosts, error)) gently {
	t.Helper()
	control := gomock.NewController(t)
	random := mocks.NewMockRandom(control)
	random.EXPECT().Float64().Return(0.5).AnyTimes()
	clock := stillClock(control)
	doer, get := mocks.NewMockDoer(control), mocks.NewMockWeb(control)
	hosted, err := hosts(folder, clock, random)
	if err != nil {
		t.Fatal(err)
	}
	return gently{command: command{get: get, reports: doer,
		hosts: hosted, errs: io.Discard,
		picked: picked(t, numbersRaw)}, clock: clock, get: get,
		doer: doer}
}

// registered is the hosts run gives: those the tool registers its
// policies with.
func registered(folder string, clock web.Clock,
	random web.Random) (*web.Hosts, error) {
	return hostsOn(folder, clock, random)
}

// unregistered is hosts with no policy of any host's own.
func unregistered(folder string, clock web.Clock,
	random web.Random) (*web.Hosts, error) {
	return web.NewHosts(folder, clock, random, nil), nil
}

// citation is the answer of the first crafted citation, as a Doer gives
// it.
func citation(t *testing.T) func(*http.Request) (*http.Response, error) {
	return func(*http.Request) (*http.Response, error) {
		return ntrsAnswer(t, http.StatusOK, "@citation.json"), nil
	}
}

// ntrs-fetch asks NTRS for a citation and then for its PDF, one request
// after the other to the same host, and both are a pace of NTRS's own
// apart: the sleep between them is the two seconds of ntrs-pace.
//
// Anti-test: a host with no policy of its own is a second apart, so the
// two seconds are NTRS's, not the default.
func TestNTRSFetchIsPacedAtNTRSsOwnPace(t *testing.T) {
	for pace, hosts := range map[time.Duration]func(string, web.Clock,
		web.Random) (*web.Hosts, error){ntrsPace: registered,
		defaultPace: unregistered} {
		tool := askingNTRSGently(t, t.TempDir(), hosts)
		tool.doer.EXPECT().Do(gomock.Any()).DoAndReturn(citation(t))
		tool.get.EXPECT().Get(firstPDF).DoAndReturn(served)
		tool.clock.EXPECT().Sleep(gomock.Any(), pace).Return(nil)
		_, errs, code := ran(tool.command, "ntrs-fetch", "20990000001",
			"--into", t.TempDir())
		if code != exitOK {
			t.Errorf("%v: exit %d: %s", pace, code, errs)
		}
	}
}

// NTRS asking us to stop is said with the switch that asks again, and
// the next run asks nothing until the wait has passed, saying so. With
// the switch it asks again, and says what it is doing.
//
// Anti-test: the run that is stopped does not ask NTRS at all.
func TestNTRSSearchIsStoppedAndTheSwitchAsksAgain(t *testing.T) {
	folder := t.TempDir()
	busy := func(*http.Request) (*http.Response, error) {
		answer := ntrsAnswer(t, http.StatusTooManyRequests, "")
		answer.Header.Set("Retry-After", "90")
		return answer, nil
	}
	first := askingNTRSGently(t, folder, registered)
	first.doer.EXPECT().Do(gomock.Any()).DoAndReturn(busy)
	_, errs, code := ran(first.command, "ntrs", "x")
	if code == exitOK || !strings.Contains(errs, "asked us to stop") ||
		!strings.HasSuffix(strings.TrimSpace(errs), "--i-mean-it") {
		t.Errorf("first: exit %d: %q", code, errs)
	}
	// the second asks NTRS nothing: its mock Doer expects no call
	second := askingNTRSGently(t, folder, registered)
	_, errs, code = ran(second.command, "ntrs", "x")
	if code == exitOK || !strings.HasSuffix(strings.TrimSpace(errs),
		"--i-mean-it") {
		t.Errorf("second: exit %d: %q", code, errs)
	}
	third := askingNTRSGently(t, folder, registered)
	third.doer.EXPECT().Do(gomock.Any()).DoAndReturn(func(
		*http.Request) (*http.Response, error) {
		return ntrsAnswer(t, http.StatusOK, "@none-found.json"), nil
	})
	third.clock.EXPECT().Sleep(gomock.Any(), gomock.Any()).Return(nil).
		AnyTimes()
	out, errs, code := ran(third.command, "ntrs", "x", "--i-mean-it")
	if code != exitOK || !strings.Contains(errs, "asking anyway") ||
		!strings.Contains(out, "citations match") {
		t.Errorf("third: exit %d: %q %q", code, out, errs)
	}
}

// A search that fails with no word from NTRS is asked again, with the
// same body: NTRS's policy says a search is safe to repeat. The second
// answer is the one said.
//
// Anti-test: a host with no policy of its own is not asked again, since
// the default says no POST is safe to repeat.
func TestNTRSSearchIsRepeatedOnlyUnderNTRSsPolicy(t *testing.T) {
	for repeated, hosts := range map[bool]func(string, web.Clock,
		web.Random) (*web.Hosts, error){true: registered, false: unregistered} {
		tool := askingNTRSGently(t, t.TempDir(), hosts)
		var bodies []string
		// a connection that ended early is a failure with no word
		answers := []error{io.ErrUnexpectedEOF, nil}
		tool.doer.EXPECT().Do(gomock.Any()).DoAndReturn(func(
			request *http.Request) (*http.Response, error) {
			raw, _ := io.ReadAll(request.Body)
			bodies = append(bodies, string(raw))
			failure := answers[len(bodies)-1]
			if failure != nil {
				return nil, failure
			}
			return ntrsAnswer(t, http.StatusOK, "@none-found.json"), nil
		}).MinTimes(1)
		tool.clock.EXPECT().Sleep(gomock.Any(), gomock.Any()).Return(nil).
			AnyTimes()
		out, errs, code := ran(tool.command, "ntrs", "x")
		if repeated != (code == exitOK) || repeated != (len(bodies) == 2) ||
			repeated && bodies[0] != bodies[1] ||
			!repeated && !strings.Contains(errs, "unexpected EOF") ||
			repeated && !strings.Contains(out, "citations match") {
			t.Errorf("%v: exit %d: %q %q %q", repeated, code, out, errs,
				bodies)
		}
	}
}

// Both verbs take --i-mean-it, and its help says what it does.
//
// Anti-test: it is said of no verb that asks nothing through the web.
func TestTheNTRSVerbsTakeTheSwitch(t *testing.T) {
	for _, verb := range []string{"ntrs", "ntrs-fetch"} {
		help := helpOf(t, verb)
		if !strings.Contains(help, "--i-mean-it") {
			t.Errorf("%s: %q", verb, help)
		}
	}
	if help := helpOf(t, "archives"); strings.Contains(help, "i-mean-it") {
		t.Errorf("archives: %q", help)
	}
}

// ntrs-fetch given --i-mean-it has the hosts insist, so a stop on its
// citation is asked again once, and what it asked for is said.
func TestNTRSFetchInsistsWhenGivenTheSwitch(t *testing.T) {
	tool := askingNTRSGently(t, t.TempDir(), registered)
	tool.doer.EXPECT().Do(gomock.Any()).DoAndReturn(func(
		*http.Request) (*http.Response, error) {
		answer := ntrsAnswer(t, http.StatusTooManyRequests, "")
		answer.Header.Set("Retry-After", "90")
		return answer, nil
	}).Times(2)
	tool.clock.EXPECT().Sleep(gomock.Any(), gomock.Any()).Return(nil).
		AnyTimes()
	_, errs, code := ran(tool.command, "ntrs-fetch", "20990000001",
		"--into", t.TempDir(), "--i-mean-it")
	if code == exitOK || !strings.Contains(errs, "asking once more") ||
		strings.Contains(errs, "to ask anyway, add") {
		t.Errorf("exit %d: %q", code, errs)
	}
}

// Anti-test: without the switch the same stop is asked once and names it.
func TestNTRSFetchNamesTheSwitchOnAStop(t *testing.T) {
	tool := askingNTRSGently(t, t.TempDir(), registered)
	tool.doer.EXPECT().Do(gomock.Any()).DoAndReturn(func(
		*http.Request) (*http.Response, error) {
		return ntrsAnswer(t, http.StatusTooManyRequests, ""), nil
	})
	_, errs, code := ran(tool.command, "ntrs-fetch", "20990000001",
		"--into", t.TempDir())
	if code == exitOK || !strings.Contains(errs, "to ask anyway, add "+
		"--i-mean-it") {
		t.Errorf("exit %d: %q", code, errs)
	}
}

// keptIn is the names of the files a folder keeps.
func keptIn(t *testing.T, folder string) []string {
	t.Helper()
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// A PDF longer than ntrs-pdf-most is refused whether NTRS says its length
// or not, and nothing of it is kept, not even the part that came.
//
// Anti-test: a PDF of exactly the most is kept.
func TestNTRSFetchRefusesAPDFOverTheMost(t *testing.T) {
	for says, length := range map[string]int64{"says": pdfLength,
		"does not say": -1} {
		tool, _, get := askingNTRS(t, map[string]*http.Response{
			ntrsFirst: ntrsAnswer(t, http.StatusOK, "@citation.json")})
		tool.picked = picked(t, numbersWith(map[string]string{
			"ntrs-whole": "30", "ntrs-pdf-most": "10"}))
		get.EXPECT().Get(firstPDF).Return(io.NopCloser(strings.NewReader(
			pdf)), length, nil)
		into := t.TempDir()
		_, errs, code := ran(tool, "ntrs-fetch", "20990000001", "--into",
			into)
		if code == exitOK || !strings.Contains(errs, "ntrs-pdf-most") ||
			len(keptIn(t, into)) != none {
			t.Errorf("NTRS %s its length: exit %d: %q, %v", says, code,
				errs, keptIn(t, into))
		}
	}
	tool, _, get := askingNTRS(t, map[string]*http.Response{
		ntrsFirst: ntrsAnswer(t, http.StatusOK, "@citation.json")})
	tool.picked = picked(t, numbersWith(map[string]string{
		"ntrs-whole": "30", "ntrs-pdf-most": "16"}))
	get.EXPECT().Get(firstPDF).DoAndReturn(served)
	into := t.TempDir()
	if _, errs, code := ran(tool, "ntrs-fetch", "20990000001", "--into",
		into); code != exitOK || len(keptIn(t, into)) != 1 {
		t.Errorf("exit %d: %q, %v", code, errs, keptIn(t, into))
	}
}

// A capped web asks the web inside for a length and for a post as it
// is, and gives what it answers, an error too.
func TestACappedWebPassesHeadAndPostOn(t *testing.T) {
	inner := quiet(t)
	inner.EXPECT().Head(firstPDF).Return(pdfLength, nil)
	inner.EXPECT().Post(ntrsSearch, gomock.Any()).Return("", errors.New("no"))
	capped := cappedWeb{inner: inner, most: 1}
	length, err := capped.Head(firstPDF)
	where, postErr := capped.Post(ntrsSearch, nil)
	if length != pdfLength || err != nil || where != "" || postErr == nil {
		t.Errorf("%d, %v, %q, %v", length, err, where, postErr)
	}
}

// A PDF that fails as it is read is the failure of the read, and the
// body is closed. The cap does not hide it.
func TestNTRSFetchSaysWhyAPDFFailedToRead(t *testing.T) {
	tool, _, get := askingNTRS(t, map[string]*http.Response{
		ntrsFirst: ntrsAnswer(t, http.StatusOK, "@citation.json")})
	get.EXPECT().Get(firstPDF).Return(io.NopCloser(deadLine{}), int64(-1), nil)
	into := t.TempDir()
	_, errs, code := ran(tool, "ntrs-fetch", "20990000001", "--into", into)
	if code == exitOK || !strings.Contains(errs, "the line went dead") ||
		len(keptIn(t, into)) != none {
		t.Errorf("exit %d: %q, %v", code, errs, keptIn(t, into))
	}
}

// deadLine is a body that fails as it is read.
type deadLine struct{}

// Read fails.
func (deadLine) Read([]byte) (int, error) {
	return 0, errors.New("the line went dead")
}

// A web that cannot be asked for lack of a PDF's own time limit is
// refused, naming it, so a PDF that stalls is never waited on for ever.
//
// Anti-test: with the number, the web the network gives is a client of
// the web package.
func TestNTRSFetchHoldsAPDFToItsOwnTimeLimit(t *testing.T) {
	missing := command{picked: picked(t, numbersWith(map[string]string{
		"wait": "30"}))}
	if _, err := missing.ntrsBare(); err == nil ||
		!strings.Contains(err.Error(), "ntrs-pdf-whole") {
		t.Errorf("%v", err)
	}
	given := command{picked: picked(t, numbersRaw)}
	bare, err := given.ntrsBare()
	if _, isClient := bare.(web.Client); err != nil || !isClient {
		t.Errorf("%T, %v", bare, err)
	}
}

// With no ntrs-pdf-most, or none that makes sense, no PDF is asked for.
func TestNTRSFetchWithNoMostIsRefused(t *testing.T) {
	tool, _, _ := askingNTRS(t, map[string]*http.Response{
		ntrsFirst: ntrsAnswer(t, http.StatusOK, "@citation.json")})
	tool.picked = picked(t, numbersWith(map[string]string{
		"ntrs-whole": "30"}))
	_, errs, code := ran(tool, "ntrs-fetch", "20990000001", "--into",
		t.TempDir())
	if code == exitOK || !strings.Contains(errs, "ntrs-pdf-most") {
		t.Errorf("exit %d: %q", code, errs)
	}
}

// The policies run registers hold NTRS's host, in the name the hosts
// use, and its own policy.
func TestTheToolRegistersNTRSsPolicy(t *testing.T) {
	kept := ntrsPolicies()
	if kept["ntrs.nasa.gov"].Defaulted ||
		kept["ntrs.nasa.gov"].Pace != ntrsPace {
		t.Errorf("%v", kept)
	}
	if hosts, err := hostsOf(filepath.Join(t.TempDir(), "none")); err != nil || hosts == nil {
		t.Errorf("hosts: %v, %v", hosts, err)
	}
}
