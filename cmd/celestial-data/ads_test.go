package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/ads"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/secrets"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
)

// The token the tests keep, and the query they ask: a star's Gaia id,
// anywhere in a paper.
const (
	adsToken = "tok-not-a-real-token"
	asked    = `full:"4081136684118965760"`
)

// twoFound is an answer of ADS's shape, crafted: two papers of the
// three that match, the first on arXiv.
const twoFound = `{"response": {"numFound": 3, "docs": [
  {"bibcode": "2026Crft....1....1A", "year": "2026",
   "title": ["A crafted paper"], "author": ["Aardvark, A.", "Bee, B."],
   "identifier": ["arXiv:2611.00001"]},
  {"bibcode": "2025Crft....2....2B", "year": "2025",
   "title": ["Another crafted paper"], "author": ["Cat, C."]}]}}`

// tokenKept is the path of a file the token is kept in, as
// celestial-data-secrets ads-login keeps it.
func tokenKept(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ads-token")
	if err := ads.Save(adsToken, path); err != nil {
		t.Fatal(err)
	}
	return path
}

// tokenAt answers ADS_SESSION with a path and any other name with
// nothing.
type tokenAt string

// lookup is the path for ADS_SESSION, nothing for the rest.
func (path tokenAt) lookup(name string) string {
	if name == ads.Session {
		return string(path)
	}
	return ""
}

// askingADS is celestial-data reading the token from a path, with ADS a
// mock that takes no call it is not told to expect.
func askingADS(t *testing.T, path string) (command, *mocks.MockDoer) {
	t.Helper()
	ask := mocks.NewMockDoer(gomock.NewController(t))
	tool := command{picked: picked(t, numbersRaw), ask: ask,
		lookup: tokenAt(path).lookup}
	return tool, ask
}

// adsAnswer is ADS's answer: a status, its body and its headers.
func adsAnswer(status int, body string, headers ...string) *http.Response {
	answer := &http.Response{StatusCode: status, Header: http.Header{},
		Body: io.NopCloser(strings.NewReader(body))}
	for at := headerName; at+headerValue < len(headers); at += headerPair {
		answer.Header.Set(headers[at], headers[at+headerValue])
	}
	return answer
}

// How adsAnswer reads its headers: a name, then its value.
const (
	headerName  = 0 // defined: the first header's name comes first
	headerValue = 1 // defined: a value follows its name
	headerPair  = 2 // defined: a name and its value
)

// keepingADS is ADS answering with an answer, keeping the request it
// was sent.
type keepingADS struct {
	answer *http.Response
	sent   *http.Request
}

// do keeps the request and gives the answer.
func (kept *keepingADS) do(request *http.Request) (*http.Response, error) {
	kept.sent = request
	return kept.answer, nil
}

// the test: ads reads the kept token, asks ADS the words given once,
// and says how many match, each paper given, and the searches left
// today.
func TestADSSaysWhatItFound(t *testing.T) {
	tool, ask := askingADS(t, tokenKept(t))
	kept := &keepingADS{answer: adsAnswer(http.StatusOK, twoFound,
		"X-RateLimit-Remaining", "4998")}
	ask.EXPECT().Do(gomock.Any()).DoAndReturn(kept.do)
	out, errs, code := ran(tool, "ads", "full:", `"4081136684118965760"`)
	want := "3 papers match; the newest 2:\n" +
		"  2026Crft....1....1A 2026 Aardvark, A.: A crafted paper " +
		"(arXiv 2611.00001)\n" +
		"  2025Crft....2....2B 2025 Cat, C.: Another crafted paper\n" +
		"ADS allows 4998 more searches today\n"
	if code != exitOK || out != want {
		t.Fatalf("exit %d: %q%s", code, out, errs)
	}
	query := kept.sent.URL.Query()
	if query.Get("q") != `full: "4081136684118965760"` ||
		query.Get("rows") != "20" ||
		kept.sent.Header.Get("Authorization") != "Bearer "+adsToken {
		t.Errorf("asked %v", kept.sent.URL)
	}
}

// bareFound is an answer of ADS's shape, crafted: a paper given with no
// title and no author.
const bareFound = `{"response": {"numFound": 1, "docs": [
  {"bibcode": "2024Crft....3....3C", "year": "2024"}]}}`

// the anti-test: a paper with no author and no title says so, and is
// not given another paper's.
func TestADSSaysAPaperWithNoAuthorOrTitle(t *testing.T) {
	tool, ask := askingADS(t, tokenKept(t))
	ask.EXPECT().Do(gomock.Any()).Return(adsAnswer(http.StatusOK,
		bareFound), nil)
	out, errs, code := ran(tool, "ads", asked)
	want := "1 papers match; the newest 1:\n" +
		"  2024Crft....3....3C 2024 no author named: no title given\n"
	if code != exitOK || out != want {
		t.Errorf("exit %d: %q%s", code, out, errs)
	}
}

// the anti-test: what ads says never holds the token, and an answer
// that does not say the searches left says nothing of them.
func TestADSSaysNoToken(t *testing.T) {
	tool, ask := askingADS(t, tokenKept(t))
	ask.EXPECT().Do(gomock.Any()).Return(adsAnswer(http.StatusOK,
		twoFound), nil)
	out, errs, code := ran(tool, "ads", asked)
	if code != exitOK || strings.Contains(out+errs, adsToken) ||
		strings.Contains(out, "searches today") {
		t.Errorf("exit %d: %s%s", code, out, errs)
	}
}

// the test: --most takes ads-most's place, as how many papers to ask,
// down to one, the fewest.
func TestADSMostTakesTheNumbersPlace(t *testing.T) {
	tool, ask := askingADS(t, tokenKept(t))
	kept := &keepingADS{answer: adsAnswer(http.StatusOK, twoFound)}
	ask.EXPECT().Do(gomock.Any()).DoAndReturn(kept.do)
	_, errs, code := ran(tool, "ads", asked, "--most", "1")
	if rows := kept.sent.URL.Query().Get("rows"); code != exitOK ||
		rows != "1" {
		t.Errorf("exit %d, %s rows: %s", code, rows, errs)
	}
}

// the anti-test: no words, --most below one or not a number, and no
// ads-most to pick are each refused before ADS is asked.
func TestADSRefusesWhatItCannotAsk(t *testing.T) {
	path := tokenKept(t)
	for says, words := range map[string][]string{
		"needs words":       {"ads", "  "},
		"at least one":      {"ads", asked, "--most", "0"},
		"is a whole number": {"ads", asked, "--most", "many"},
	} {
		tool, _ := askingADS(t, path)
		_, errs, code := ran(tool, words...)
		if code == exitOK || !strings.Contains(errs, says) {
			t.Errorf("%v: exit %d: %s", words, code, errs)
		}
	}
	tool, _ := askingADS(t, path)
	tool.picked = picked(t, numbersWith(nil))
	if _, errs, code := ran(tool, "ads", asked); code == exitOK ||
		!strings.Contains(errs, "ads-most") {
		t.Errorf("exit %d: %s", code, errs)
	}
}

// adsEnvironment answers ADS_SESSION and HOME with what it holds, and
// any other name with nothing.
type adsEnvironment struct{ session, home string }

// lookup is the variable's value.
func (env adsEnvironment) lookup(name string) string {
	return map[string]string{ads.Session: env.session,
		"HOME": env.home}[name]
}

// the test: with no ADS_SESSION, ads reads the token ads-login kept
// beside the secrets' config.
func TestADSFindsTheTokenBesideTheConfig(t *testing.T) {
	home := t.TempDir()
	path := secrets.Beside(home, ads.TokenFile)
	if err := os.MkdirAll(filepath.Dir(path), homeFolder); err != nil {
		t.Fatal(err)
	}
	if err := ads.Save(adsToken, path); err != nil {
		t.Fatal(err)
	}
	tool, ask := askingADS(t, "")
	tool.lookup = adsEnvironment{home: home}.lookup
	kept := &keepingADS{answer: adsAnswer(http.StatusOK, twoFound)}
	ask.EXPECT().Do(gomock.Any()).DoAndReturn(kept.do)
	_, errs, code := ran(tool, "ads", asked)
	if code != exitOK ||
		kept.sent.Header.Get("Authorization") != "Bearer "+adsToken {
		t.Errorf("exit %d: %s", code, errs)
	}
}

// homeFolder is the mode of a test home's folders. It's defined:
// entered, read and written by its owner alone.
const homeFolder = 0o700

// the anti-test: with no token kept where ads looks, or one others can
// read, or no home and no ADS_SESSION, ads is refused, saying why and
// what to run, before ADS is asked.
func TestADSWithNoTokenKeptIsRefused(t *testing.T) {
	open := filepath.Join(t.TempDir(), "open")
	if err := os.WriteFile(open, []byte(adsToken), anyReader); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	missing := filepath.Join(t.TempDir(), "none")
	for says, env := range map[string]adsEnvironment{
		"neither names one":      {},
		"a kept token is a file": {session: open},
		"no ads token is kept in " + missing + ": run " +
			"celestial-data-secrets ads-login": {session: missing},
		"no ads token is kept in " +
			secrets.Beside(home, ads.TokenFile): {home: home},
	} {
		tool, _ := askingADS(t, "")
		tool.lookup = env.lookup
		_, errs, code := ran(tool, "ads", asked)
		if code == exitOK || !strings.Contains(errs, says) ||
			strings.Contains(errs, adsToken) {
			t.Errorf("%v: exit %d: %s", env, code, errs)
		}
	}
}

// anyReader is a mode anyone can read. It's defined: read by anyone,
// written by its owner.
const anyReader = 0o644

// the anti-test: ads changes nothing, so it has no dry run to offer;
// asked for one, it runs nothing.
func TestADSHasNoDryRun(t *testing.T) {
	tool, _ := askingADS(t, tokenKept(t))
	if _, errs, code := ran(tool, "ads", asked, "--dry-run"); code !=
		exitUsage || !strings.Contains(errs, "--dry-run") {
		t.Errorf("exit %d: %s", code, errs)
	}
}

// the anti-test: when ADS asks us to stop, ads stops, asks nothing more,
// and says until when: a count of seconds as a UTC time, any other
// reset as written, and none as no time given.
func TestADSStopsWhenADSAsks(t *testing.T) {
	for reset, says := range map[string]string{
		"1794009600": "until 2026-11-07T00:00:00Z; nothing more",
		"tomorrow":   "until tomorrow; nothing more",
		"":           "and gave no time; nothing more",
	} {
		tool, ask := askingADS(t, tokenKept(t))
		ask.EXPECT().Do(gomock.Any()).Return(adsAnswer(
			http.StatusTooManyRequests, "", "X-RateLimit-Reset",
			reset), nil)
		_, errs, code := ran(tool, "ads", asked)
		if code == exitOK || !strings.Contains(errs, "ADS asked us to "+
			"stop "+says) {
			t.Errorf("%q: exit %d: %s", reset, code, errs)
		}
	}
}

// the anti-test: ADS refusing the token is an error that says so.
func TestADSRefusingTheTokenIsAnError(t *testing.T) {
	tool, ask := askingADS(t, tokenKept(t))
	ask.EXPECT().Do(gomock.Any()).Return(adsAnswer(
		http.StatusUnauthorized, ""), nil)
	if _, errs, code := ran(tool, "ads", asked); code == exitOK ||
		!strings.Contains(errs, "ADS refused the token") {
		t.Errorf("exit %d: %s", code, errs)
	}
}

// the test: with no asker given, ADS is asked through the network's,
// with the whole request held to ads-whole.
func TestADSIsAskedThroughTheNetwork(t *testing.T) {
	tool := command{picked: picked(t, numbersRaw)}
	asker, err := tool.adsAsker()
	client, isClient := asker.(*http.Client)
	whole, wholeErr := tool.picked.Get("ads-whole")
	if err != nil || wholeErr != nil || !isClient ||
		client.Timeout != web.Seconds(whole) {
		t.Errorf("%v, %v, %v", asker, err, wholeErr)
	}
}

// the anti-test: with no ads-whole to hold the request to, ADS is not
// asked.
func TestADSWithNoWaitIsRefused(t *testing.T) {
	tool, _ := askingADS(t, tokenKept(t))
	tool.ask = nil
	tool.picked = picked(t, numbersWith(map[string]string{
		"ads-most": "20"}))
	if _, errs, code := ran(tool, "ads", asked); code == exitOK ||
		!strings.Contains(errs, "ads-whole") {
		t.Errorf("exit %d: %s", code, errs)
	}
}

// Example_ads searches for a star's Gaia id anywhere in a paper, with
// the token kept where ADS_SESSION says. ADS is a mock here, and its
// answer is crafted.
func Example_ads() {
	folder, err := os.MkdirTemp("", "ads")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(folder)
	path := filepath.Join(folder, "ads-token")
	if err := ads.Save(adsToken, path); err != nil {
		panic(err)
	}
	ask := mocks.NewMockDoer(gomock.NewController(stopping{}))
	ask.EXPECT().Do(gomock.Any()).Return(adsAnswer(http.StatusOK,
		twoFound, "X-RateLimit-Remaining", "4998"), nil)
	tool := command{picked: celestialdata.NewMeasured(numbersFile,
		numbersRaw), lookup: tokenAt(path).lookup, ask: ask}
	var out, errs bytes.Buffer
	tool.run([]string{"ads", asked}, strings.NewReader(""), &out, &errs)
	fmt.Print(out.String(), errs.String())
	// Output:
	// 3 papers match; the newest 2:
	//   2026Crft....1....1A 2026 Aardvark, A.: A crafted paper (arXiv 2611.00001)
	//   2025Crft....2....2B 2025 Cat, C.: Another crafted paper
	// ADS allows 4998 more searches today
}
