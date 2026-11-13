package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

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
	doer := mocks.NewMockDoer(gomock.NewController(t))
	gomock.InOrder(doer.EXPECT().Do(gomock.Any()).Return(ok(three), nil),
		doer.EXPECT().Do(gomock.Any()).Return(stop, nil))
	tool := through(t, quiet(t))
	tool.curves = doer
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
