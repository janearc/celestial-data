package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/janearc/celestial-data/dasch"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
)

// The place the dasch tests ask about, as words on the command line: the
// circle round the star whose light curve DASCH was recorded giving.
var starWords = []string{"--ra", "287.50086", "--dec", "-22.23481",
	"--radius", "0.0014"}

// What DASCH answers the tests with. The first is the catalogue search,
// crafted: the columns are the four DASCH answered with, and the numbers
// are the star's. The second is a light curve of two lines, crafted.
const (
	sources = `["ref_number,gsc_bin_index,ra_deg,dec_deg",` +
		`"419100022221405,52510421,287.50084,-22.2348"]`
	curve = `["date_jd,series","2412329.520962,b"]`
	// curveFile is the file the light curve is kept in, and
	// curveText what it holds.
	curveFile = "dasch-dr7-apass-419100022221405-lightcurve.csv"
	curveText = "date_jd,series\n2412329.520962,b\n"
)

// ok is an answer of 200 with a body.
func ok(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK,
		Header: http.Header{},
		Body:   io.NopCloser(strings.NewReader(body))}
}

// askingDASCH is celestial-data asking DASCH through a mock doer that answers
// each body in turn, and notes how many were asked.
func askingDASCH(t *testing.T, bodies ...string) command {
	t.Helper()
	doer := mocks.NewMockDoer(gomock.NewController(t))
	for _, body := range bodies {
		doer.EXPECT().Do(gomock.Any()).Return(ok(body), nil)
	}
	tool := through(t, quiet(t))
	tool.curves = doer
	return tool
}

// find names the light curve of the star's source, and the address that
// holds the question. It is asked by name.
func TestFindNamesTheDASCHLightCurve(t *testing.T) {
	words := append([]string{"find", "--archive", "dasch"}, starWords...)
	out, errs, code := ran(askingDASCH(t, sources), words...)
	for _, want := range []string{"dasch apass-419100022221405 " +
		curveFile + "\n", "&ref_number=419100022221405&"} {
		if code != ranOK || !strings.Contains(out, want) {
			t.Errorf("no %q in %d %q %q", want, code, out, errs)
		}
	}
}

// fetch brings the light curve home as a CSV file in a folder for dasch,
// asking DASCH twice: the sources, and then the light curve.
func TestFetchBringsTheDASCHLightCurveHome(t *testing.T) {
	folder := t.TempDir()
	words := append([]string{"fetch", "--archive", "dasch", "--into",
		folder}, starWords...)
	home := filepath.Join(folder, "dasch", curveFile)
	out, errs, code := ran(askingDASCH(t, sources, curve), words...)
	got, err := os.ReadFile(home)
	if code != ranOK || out != "fetched "+home+"\n" || err != nil ||
		string(got) != curveText {
		t.Errorf("%d %q %q %v %q", code, out, errs, err, got)
	}
}

// Anti-test: a dry fetch says what it would bring and asks DASCH only
// for the sources, never for the light curve.
func TestADryFetchAsksDASCHForNoLightCurve(t *testing.T) {
	folder := t.TempDir()
	words := append([]string{"fetch", "--archive", "dasch", "--into",
		folder, "--dry-run"}, starWords...)
	out, errs, code := ran(askingDASCH(t, sources), words...)
	if code != ranOK || !strings.Contains(out, "would fetch ") ||
		errs != "" {
		t.Errorf("%d %q %q", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(folder, "dasch")); err == nil {
		t.Error("a dry fetch made a folder")
	}
}

// Anti-test: DASCH is asked nothing when a call names other archives.
func TestDASCHIsNotAskedWhenNotNamed(t *testing.T) {
	words := append([]string{"find", "--archive", "hips"}, starWords...)
	tool := askingDASCH(t)
	if _, errs, code := ran(tool, words...); code != ranOK {
		t.Errorf("%d %q", code, errs)
	}
}

// the test: with no doer given, DASCH is asked through the network's,
// with the whole request held to dasch-whole.
func TestDASCHIsAskedThroughTheNetwork(t *testing.T) {
	tool := command{picked: picked(t, numbersRaw)}
	asker, err := tool.daschAsker()
	client, isClient := asker.(*http.Client)
	whole, wholeErr := tool.picked.Get("dasch-whole")
	if err != nil || wholeErr != nil || !isClient ||
		client.Timeout != web.Seconds(whole) {
		t.Errorf("%v, %v, %v", asker, err, wholeErr)
	}
}

// Anti-test: the network's asker does not follow a redirect. A POST
// followed would be sent again, perhaps somewhere else; the redirect's
// own answer comes back, and where it pointed is never asked.
func TestDASCHsAskerKeepsARedirectsAnswer(t *testing.T) {
	elsewhere := false
	server := httptest.NewServer(http.HandlerFunc(
		func(answer http.ResponseWriter, asked *http.Request) {
			if asked.URL.Path == "/elsewhere" {
				elsewhere = true
				return
			}
			http.Redirect(answer, asked, "/elsewhere", http.StatusFound)
		}))
	defer server.Close()
	asker, err := command{picked: picked(t, numbersRaw)}.daschAsker()
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/querycat",
		strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	answered, err := asker.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	answered.Body.Close()
	if answered.StatusCode != http.StatusFound || elsewhere {
		t.Errorf("answered %d; elsewhere asked: %v", answered.StatusCode,
			elsewhere)
	}
}

// Anti-test: when DASCH asks us to stop, a fetch of several light
// curves stops at the one it was refused, asks for none after it, and
// says how many it did not ask for.
func TestAFetchStopsWhenDASCHAsksItTo(t *testing.T) {
	const three = `["ref_number,gsc_bin_index,ra_deg,dec_deg",` +
		`"419100022221405,52510421,287.50084,-22.2348",` +
		`"419100022221406,52510421,287.50090,-22.2349",` +
		`"419100022221407,52510421,287.50080,-22.2347"]`
	stop := &http.Response{StatusCode: http.StatusTooManyRequests,
		Header: http.Header{"Retry-After": {"30"}},
		Body:   io.NopCloser(strings.NewReader(""))}
	tool, _ := hostedDASCH(t, ok(three), stop)
	words := append([]string{"fetch", "--archive", "dasch", "--into",
		t.TempDir()}, starWords...)
	out, errs, code := ran(tool, words...)
	if code == ranOK || !strings.Contains(errs, "asked us to stop") ||
		!strings.Contains(errs,
			"stopped: 2 more of its files were not asked for") {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

// Anti-test: with no dasch-whole to hold the request to, DASCH is not
// asked, and no archive is.
func TestDASCHWithNoWaitIsRefused(t *testing.T) {
	tool := through(t, quiet(t))
	tool.picked = picked(t, numbersWith(map[string]string{"most": "5"}))
	words := append([]string{"find", "--archive", "dasch"}, starWords...)
	if _, errs, code := ran(tool, words...); code == ranOK ||
		!strings.Contains(errs, "dasch-whole") {
		t.Errorf("%d %q", code, errs)
	}
}

// DASCH's questions are asked through the hosts, which keep a record of
// the pace in the cache folder, as arXiv's gate does.
func TestDASCHIsAskedThroughTheHostsAndTheirRecordKept(t *testing.T) {
	tool, folder := hostedDASCH(t, ok(sources), ok(curve))
	words := append([]string{"fetch", "--archive", "dasch", "--into",
		t.TempDir()}, starWords...)
	_, errs, code := ran(tool, words...)
	record := web.RecordPath(folder, dasch.Host)
	if kept, err := os.ReadFile(record); code != ranOK || err != nil ||
		!strings.HasPrefix(string(kept), "last ") {
		t.Errorf("%d %q %q, %v", code, errs, kept, err)
	}
}

// Anti-test: a tool with no hosts asks DASCH as it is and keeps no
// record.
func TestDASCHIsAskedAsItIsWithNoHosts(t *testing.T) {
	tool := askingDASCH(t)
	asker, err := tool.daschAsker()
	if err != nil || asker != tool.curves {
		t.Errorf("%v, %v", asker, err)
	}
}

// DASCH's own numbers are what the hosts ask it by: its host is its
// own, with its pace, and it is not left to the defaults.
func TestDASCHHasNumbersOfItsOwnInTheHosts(t *testing.T) {
	policies, err := hostPolicies()
	if err != nil {
		t.Fatal(err)
	}
	policy, ok := policies[dasch.Host]
	if !ok || policy.Host != "DASCH" || policy.Defaulted ||
		policy.Pace != time.Second || !policy.RepeatsPosts {
		t.Errorf("%v, %+v", ok, policy)
	}
}

// When DASCH asks us to stop, the run says so, says what DASCH asked
// for, names --i-mean-it, and the next run asks nothing until the wait
// has passed.
func TestADASCHStopIsSaidAndKeptForTheNextRun(t *testing.T) {
	tool, folder := hostedDASCH(t, refusing429(farOff))
	words := append([]string{"find", "--archive", "dasch"}, starWords...)
	_, errs, code := ran(tool, words...)
	for _, want := range []string{"DASCH asked us to stop",
		"its Retry-After is " + farOff, "to ask anyway, add --i-mean-it"} {
		if code == ranOK || !strings.Contains(errs, want) {
			t.Errorf("no %q in %d %q", want, code, errs)
		}
	}
	next := nextRun(t, folder)
	_, errs, code = ran(next, words...)
	if code == ranOK || !strings.Contains(errs, "it asked to be left "+
		"until") {
		t.Errorf("%d %q", code, errs)
	}
}

// With --i-mean-it, the run after a stop says what DASCH asked for and
// asks anyway.
func TestTheSwitchAsksDASCHAgainAfterAStop(t *testing.T) {
	tool, folder := hostedDASCH(t, refusing429(farOff))
	words := append([]string{"find", "--archive", "dasch"}, starWords...)
	ran(tool, words...)
	next := nextRun(t, folder, ok(sources))
	out, errs, code := ran(next, append(words, "--i-mean-it")...)
	if code != ranOK || !strings.Contains(errs, "asking anyway, as told "+
		"to") || !strings.Contains(out, "dasch apass-419100022221405") {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

// Another run's gentle shares its record, so this run waits out that
// run's pace and asks DASCH successfully.
func TestAnotherRunAskingDASCHSharesItsPace(t *testing.T) {
	tool, folder := hostedDASCH(t, ok(sources))
	policy, _ := dasch.Policy()
	clock := stillClock(gomock.NewController(t))
	other, err := web.NewGentle(policy, clock, web.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Hold(web.RecordPath(folder, dasch.Host)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { other.Release() })
	if err := other.Enter(context.Background()); err != nil {
		t.Fatal(err)
	}
	other.Leave()
	clock.EXPECT().Sleep(gomock.Any(), time.Second).Return(nil)
	tool.hosts = web.NewHosts(folder, clock, web.Unix(),
		map[string]web.Policy{dasch.Host: policy})
	words := append([]string{"find", "--archive", "dasch"}, starWords...)
	out, errs, code := ran(tool, words...)
	if code != ranOK || !strings.Contains(out,
		"dasch apass-419100022221405") || errs != "" {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

// A fetch that DASCH stops leaves the light curves after it unasked,
// and says how many.
func TestAFetchDASCHStopsSaysHowManyWereNotAsked(t *testing.T) {
	two := `["ref_number,gsc_bin_index,ra_deg,dec_deg",` +
		`"419100022221405,52510421,287.50084,-22.2348",` +
		`"419100022221406,52510421,287.50085,-22.2349"]`
	tool, _ := hostedDASCH(t, ok(two), refusing429(farOff))
	words := append([]string{"fetch", "--archive", "dasch", "--into",
		t.TempDir()}, starWords...)
	_, errs, code := ran(tool, words...)
	if code == ranOK || !strings.Contains(errs, "stopped: 1 more of "+
		"its files were not asked for") {
		t.Errorf("%d %q", code, errs)
	}
}

// refusing429 is a 429 that asks to be left alone so long.
func refusing429(after string) *http.Response {
	answer := ok("")
	answer.StatusCode = http.StatusTooManyRequests
	answer.Header.Set("Retry-After", after)
	return answer
}

// hostedDASCH is celestial-data asking DASCH through hosts that keep
// their records in a folder of the test's own, on a clock that never
// waits. The doer answers each reply in turn, and is asked no more.
func hostedDASCH(t *testing.T, replies ...*http.Response) (command,
	string) {
	t.Helper()
	folder := t.TempDir()
	return nextRun(t, folder, replies...), folder
}

// nextRun is a run asking DASCH through hosts that keep their records in
// a folder, answered by the replies in turn.
func nextRun(t *testing.T, folder string,
	replies ...*http.Response) command {
	t.Helper()
	control := gomock.NewController(t)
	doer := mocks.NewMockDoer(control)
	for _, each := range replies {
		doer.EXPECT().Do(gomock.Any()).Return(each, nil)
	}
	random := mocks.NewMockRandom(control)
	random.EXPECT().Float64().Return(0.5).AnyTimes()
	tool := through(t, quiet(t))
	tool.curves = doer
	// a record from an earlier run may put the last request ahead of this
	// clock, which then sleeps; the sleep is no more than the pace
	clock := hourly(control)
	clock.EXPECT().Sleep(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	policies, err := hostPolicies()
	if err != nil {
		t.Fatal(err)
	}
	tool.hosts = web.NewHosts(folder, clock, random, policies)
	return tool
}

// Anti-tests: a fetch with no one to ask DASCH through, and a fetch with
// no numbers for its jobs, are refused: the web it brings files through
// cannot be made.
func TestFetchWebNeedsSomeoneToAskDASCHAndNumbersForJobs(t *testing.T) {
	var told bytes.Buffer
	lacking := command{picked: picked(t, numbersWith(nil))}
	for name, tool := range map[string]command{
		"no one to ask": through(t, quiet(t)),
		"no jobs":       lacking,
	} {
		got, err := tool.fetchWeb(&search{get: quiet(t)}, &told)
		if got != nil || err == nil {
			t.Errorf("%s: %v, %v", name, got, err)
		}
	}
	// a fetch missing one number of its jobs names that number
	for missing, kept := range map[string]string{"job-every": "job-asks",
		"job-asks": "job-every"} {
		tool := command{picked: picked(t, numbersWith(map[string]string{
			kept: "3"}))}
		got, err := tool.fetchWeb(&search{get: quiet(t)}, &told)
		if got != nil || err == nil || !strings.Contains(err.Error(),
			missing) {
			t.Errorf("no %s: %v, %v", missing, got, err)
		}
	}
}
