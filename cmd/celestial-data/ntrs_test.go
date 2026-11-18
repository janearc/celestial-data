package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
)

// crafted is where the replies of NTRS's shape, crafted for the ntrs
// package's tests, are kept. None was recorded from NTRS.
const crafted = "../../ntrs/testdata/"

// The addresses NTRS keeps what these tests ask for at, as its API
// description gives them.
const (
	ntrsSearch  = "https://ntrs.nasa.gov/api/citations/search"
	ntrsFirst   = "https://ntrs.nasa.gov/api/citations/20990000001"
	ntrsSecond  = "https://ntrs.nasa.gov/api/citations/20980000002"
	firstPDF    = ntrsFirst + "/downloads/report.pdf"
	secondPDF   = ntrsSecond + "/downloads/20980000002.pdf"
	firstLabel  = "20990000001.pdf (A crafted report about nothing)"
	secondLabel = "20980000002.pdf (A second crafted report)"
)

// secondCitation is a citation of NTRS's shape, crafted: the second
// one, which holds a PDF named by its id.
const secondCitation = `{"id": 20980000002, "title": "A second crafted ` +
	`report", "downloads": [{"mimetype": "application/pdf", "links": ` +
	`{"pdf": "/api/citations/20980000002/downloads/20980000002.pdf"}}]}`

// ntrsAnswer is NTRS's answer: a status and a body, or the text of a
// file of crafted replies when the body names one.
func ntrsAnswer(t *testing.T, code int, body string) *http.Response {
	t.Helper()
	if file, isFile := strings.CutPrefix(body, "@"); isFile {
		raw, err := os.ReadFile(crafted + file)
		if err != nil {
			t.Fatal(err)
		}
		body = string(raw)
	}
	return &http.Response{StatusCode: code, Header: http.Header{},
		Body: io.NopCloser(strings.NewReader(body))}
}

// fakeNTRS answers each request with the answer kept for its address,
// and keeps what it was sent, in order. An address it holds no answer
// for fails the test.
type fakeNTRS struct {
	t       *testing.T
	answers map[string]*http.Response
	sent    []*http.Request
	bodies  []string
}

// do is the fake's part of a Doer.
func (fake *fakeNTRS) do(request *http.Request) (*http.Response, error) {
	fake.sent = append(fake.sent, request)
	var body []byte
	if request.Body != nil {
		body, _ = io.ReadAll(request.Body)
	}
	fake.bodies = append(fake.bodies, string(body))
	answer, ok := fake.answers[request.URL.String()]
	if !ok {
		fake.t.Errorf("asked for %s, which no answer is kept for",
			request.URL)
		return nil, errors.New("no answer kept")
	}
	return answer, nil
}

// addresses are the methods and addresses the fake was sent, in order.
func (fake *fakeNTRS) addresses() []string {
	var kept []string
	for _, request := range fake.sent {
		kept = append(kept, request.Method+" "+request.URL.String())
	}
	return kept
}

// askingNTRS is celestial-data asking NTRS through a fake that answers
// as told, and fetching through a mock web that takes no call it is not
// told to expect.
func askingNTRS(t *testing.T, answers map[string]*http.Response) (command,
	*fakeNTRS, *mocks.MockWeb) {
	t.Helper()
	control := gomock.NewController(t)
	fake := &fakeNTRS{t: t, answers: answers}
	reports := mocks.NewMockDoer(control)
	reports.EXPECT().Do(gomock.Any()).DoAndReturn(fake.do).AnyTimes()
	get := mocks.NewMockWeb(control)
	tool := command{picked: picked(t, numbersRaw), reports: reports,
		get: get}
	return tool, fake, get
}

// the test: ntrs asks NTRS once, in the way its API description gives,
// and says how many citations match, each by its id, year, first author
// and title, and whether it holds a PDF.
func TestNTRSSaysWhatItFound(t *testing.T) {
	tool, fake, _ := askingNTRS(t, map[string]*http.Response{
		ntrsSearch: ntrsAnswer(t, http.StatusOK, "@two-citations.json")})
	out, errs, code := ran(tool, "ntrs", "Sagittarius", "A*")
	want := "115 citations match; the first 2:\n" +
		"  20990000001 2099 Quill, Ann: A crafted report about " +
		"nothing (PDF)\n" +
		"  20980000002 2098 Fen, Dee: A second crafted report " +
		"(no PDF)\n"
	if code != exitOK || out != want || errs != "" ||
		fmt.Sprint(fake.addresses()) != "[POST "+ntrsSearch+"]" ||
		fake.bodies[0] != `{"q":"Sagittarius A*","page":{"size":20,`+
			`"from":0}}` {
		t.Errorf("exit %d: %q%s; %v %q", code, out, errs,
			fake.addresses(), fake.bodies)
	}
}

// the anti-test: a citation with no year, no author and no title says
// so, and is not given another citation's.
func TestNTRSSaysACitationWithNoYearAuthorOrTitle(t *testing.T) {
	tool, _, _ := askingNTRS(t, map[string]*http.Response{
		ntrsSearch: ntrsAnswer(t, http.StatusOK, "@bare-citation.json")})
	out, errs, code := ran(tool, "ntrs", "bare")
	want := "3 citations match; the first 3:\n" +
		"  20970000003 no year given no author named: no title given " +
		"(PDF)\n" +
		"  20960000004 no year given no author named: A crafted " +
		"report with no date (no PDF)\n" +
		"  20950000005 no year given no author named: A crafted " +
		"report with a short date (no PDF)\n"
	if code != exitOK || out != want {
		t.Errorf("exit %d: %q%s", code, out, errs)
	}
}

// the test: --most takes ntrs-most's place, as how many citations to
// ask, from one to a hundred, and is sent as the page's size.
func TestNTRSMostTakesTheNumbersPlace(t *testing.T) {
	for _, most := range []string{"1", "100"} {
		tool, fake, _ := askingNTRS(t, map[string]*http.Response{
			ntrsSearch: ntrsAnswer(t, http.StatusOK,
				"@none-found.json")})
		_, errs, code := ran(tool, "ntrs", "x", "--most", most)
		if code != exitOK || !strings.Contains(fake.bodies[0],
			`"size":`+most+`,`) {
			t.Errorf("--most %s: exit %d, %q: %s", most, code,
				fake.bodies, errs)
		}
	}
}

// the anti-test: no words, --most that is not a number, below one or
// above what NTRS gives, and no ntrs-most to pick are each refused
// before NTRS is asked.
func TestNTRSRefusesWhatItCannotAsk(t *testing.T) {
	for says, words := range map[string][]string{
		"needs words":       {"ntrs", "  "},
		"is a whole number": {"ntrs", "x", "--most", "many"},
		"from 1 to 100":     {"ntrs", "x", "--most", "0"},
		"not 101":           {"ntrs", "x", "--most", "101"},
	} {
		tool, fake, _ := askingNTRS(t, nil)
		_, errs, code := ran(tool, words...)
		if code == exitOK || !strings.Contains(errs, says) ||
			len(fake.sent) != noCitations {
			t.Errorf("%v: exit %d: %s", words, code, errs)
		}
	}
	tool, _, _ := askingNTRS(t, nil)
	tool.picked = picked(t, numbersWith(nil))
	if _, errs, code := ran(tool, "ntrs", "x"); code == exitOK ||
		!strings.Contains(errs, "ntrs-most") {
		t.Errorf("exit %d: %s", code, errs)
	}
}

// the anti-test: NTRS asking us to stop is said, and a search that fails
// otherwise is said as it is.
func TestNTRSSaysWhenItIsAskedToStop(t *testing.T) {
	for says, code := range map[string]int{
		"asked us to stop": http.StatusTooManyRequests,
		"status 500":       http.StatusInternalServerError} {
		tool, _, _ := askingNTRS(t, map[string]*http.Response{
			ntrsSearch: ntrsAnswer(t, code, "")})
		out, errs, exit := ran(tool, "ntrs", "x")
		if exit == exitOK || !strings.Contains(errs, says) || out != "" {
			t.Errorf("%d: exit %d: %q %s", code, exit, out, errs)
		}
	}
}

// the test: with no asker given, NTRS is asked through the network's,
// with the whole request held to ntrs-whole.
func TestNTRSIsAskedThroughTheNetwork(t *testing.T) {
	tool := command{picked: picked(t, numbersRaw)}
	asker, err := tool.ntrsAsker()
	client, isClient := asker.(*http.Client)
	whole, wholeErr := tool.picked.Get("ntrs-whole")
	if err != nil || wholeErr != nil || !isClient ||
		client.Timeout != web.Seconds(whole) {
		t.Errorf("%v, %v, %v", asker, err, wholeErr)
	}
}

// the anti-test: with no ntrs-whole to hold the request to, NTRS is not
// asked, by either verb.
func TestNTRSWithNoWaitIsRefused(t *testing.T) {
	for _, words := range [][]string{{"ntrs", "x"},
		{"ntrs-fetch", "20990000001", "--into", t.TempDir()}} {
		tool := command{picked: picked(t, numbersWith(map[string]string{
			"ntrs-most": "20", "wait": "30"})), get: quiet(t)}
		if _, errs, code := ran(tool, words...); code == exitOK ||
			!strings.Contains(errs, "ntrs-whole") {
			t.Errorf("%v: exit %d: %s", words, code, errs)
		}
	}
}

// Example_ntrs searches for a star, and says what it found. NTRS is a
// mock here, and its answer is crafted.
func Example_ntrs() {
	raw, err := os.ReadFile(crafted + "two-citations.json")
	if err != nil {
		panic(err)
	}
	ask := mocks.NewMockDoer(gomock.NewController(stopping{}))
	ask.EXPECT().Do(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusOK, Header: http.Header{},
		Body: io.NopCloser(bytes.NewReader(raw))}, nil)
	tool := command{picked: celestialdata.NewMeasured(numbersFile,
		numbersRaw), reports: ask}
	var out, errs bytes.Buffer
	tool.run([]string{"ntrs", "Sagittarius", "A*", "--most", "2"},
		strings.NewReader(""), &out, &errs)
	fmt.Print(out.String(), errs.String())
	// Output:
	// 115 citations match; the first 2:
	//   20990000001 2099 Quill, Ann: A crafted report about nothing (PDF)
	//   20980000002 2098 Fen, Dee: A second crafted report (no PDF)
}

// the test: ntrs-fetch asks for each citation, then for its PDF at the
// address the citation gives, one at a time, and keeps the PDF as its id
// and .pdf, in a folder it makes.
func TestNTRSFetchBringsEachPDF(t *testing.T) {
	tool, fake, get := askingNTRS(t, map[string]*http.Response{
		ntrsFirst:  ntrsAnswer(t, http.StatusOK, "@citation.json"),
		ntrsSecond: ntrsAnswer(t, http.StatusOK, secondCitation)})
	into := filepath.Join(t.TempDir(), "reports")
	gomock.InOrder(get.EXPECT().Get(firstPDF).DoAndReturn(served),
		get.EXPECT().Get(secondPDF).DoAndReturn(served))
	out, errs, code := ran(tool, "ntrs-fetch", "20990000001",
		"20980000002", "20990000001", "--into", into)
	held, err := os.ReadFile(filepath.Join(into, "20990000001.pdf"))
	want := "fetched  " + firstLabel + "\n" +
		"fetched  20980000002.pdf (A second crafted report)\n" +
		"ntrs-fetch: 2 fetched, 0 kept, 0 not on NTRS, 0 with no " +
		"PDF, in " + into + "; requests to NTRS: 4, one at a time\n"
	if code != exitOK || out != want || err != nil || string(held) != pdf ||
		fmt.Sprint(fake.addresses()) != "[GET "+ntrsFirst+" GET "+
			ntrsSecond+"]" {
		t.Errorf("exit %d: %q%s; %v, %q", code, out, errs, err, held)
	}
}

// the anti-test: a PDF already in the folder is kept and asks NTRS
// nothing at all, and needs nothing of it: not even its numbers, which
// are missing here.
func TestNTRSFetchKeepsWhatIsThereAndAsksNothing(t *testing.T) {
	into := t.TempDir()
	put(t, into, "20990000001.pdf")
	tool := command{picked: picked(t, numbersWith(nil))}
	out, errs, code := ran(tool, "ntrs-fetch", "20990000001", "--into",
		into)
	want := "kept     20990000001.pdf (already there)\n" +
		"ntrs-fetch: 0 fetched, 1 kept, 0 not on NTRS, 0 with no PDF, " +
		"in " + into + "; requests to NTRS: 0, one at a time\n"
	if code != exitOK || out != want {
		t.Errorf("exit %d: %q %q", code, out, errs)
	}
}

// the anti-test: what is in the folder under a PDF's name and is not a
// PDF is refused, never taken for the PDF, and a folder that will not
// read is said. NTRS is asked nothing.
func TestNTRSFetchRefusesWhatIsInTheWay(t *testing.T) {
	into := t.TempDir()
	if err := os.WriteFile(filepath.Join(into, "20990000001.pdf"),
		[]byte("not a PDF"), paperMode); err != nil {
		t.Fatal(err)
	}
	tool, fake, _ := askingNTRS(t, nil)
	_, errs, code := ran(tool, "ntrs-fetch", "20990000001", "--into", into)
	if code == exitOK || !strings.Contains(errs, "is not a PDF") {
		t.Errorf("exit %d: %s", code, errs)
	}
	notAFolder := filepath.Join(t.TempDir(), "file")
	put(t, filepath.Dir(notAFolder), "file")
	_, errs, code = ran(tool, "ntrs-fetch", "20990000001", "--into",
		notAFolder)
	if code == exitOK || len(fake.sent) != noCitations {
		t.Errorf("exit %d: %s", code, errs)
	}
}

// the anti-test: no folder, no ids, and an id that is not digits are
// each refused before NTRS is asked, and an id that is not digits is
// named.
func TestNTRSFetchRefusesWhatItCannotAskFor(t *testing.T) {
	into := t.TempDir()
	for _, each := range []struct {
		says  string
		words []string
	}{
		{"needs --into", []string{"ntrs-fetch", "20990000001"}},
		{"needs ids", []string{"ntrs-fetch", "--into", into}},
		{`"20990000001/../../x"`, []string{"ntrs-fetch",
			"20990000001/../../x", "--into", into}},
		{`"20990000001.pdf"`, []string{"ntrs-fetch", "20990000001.pdf",
			"--into", into}},
		{`"x"`, []string{"ntrs-fetch", "20990000001", "x", "--into",
			into}},
	} {
		tool, fake, _ := askingNTRS(t, nil)
		_, errs, code := ran(tool, each.words...)
		if code == exitOK || !strings.Contains(errs, each.says) ||
			len(fake.sent) != noCitations {
			t.Errorf("%v: exit %d: %s", each.words, code, errs)
		}
	}
}

// the test: a citation NTRS does not have, one that holds no PDF, and
// one whose PDF address is another citation's are each said, and the
// rest are still asked for.
func TestNTRSFetchSaysWhatItCannotBringAndGoesOn(t *testing.T) {
	elsewhereCited := `{"id": 20970000009, "downloads": [{"mimetype": ` +
		`"application/pdf", "links": {"pdf": "` +
		`/api/citations/20123450000/downloads/a.pdf"}}]}`
	tool, fake, get := askingNTRS(t, map[string]*http.Response{
		ntrsFirst: ntrsAnswer(t, http.StatusNotFound, ""),
		ntrsSecond: ntrsAnswer(t, http.StatusOK,
			"@citation-no-pdf.json"),
		"https://ntrs.nasa.gov/api/citations/20970000009": ntrsAnswer(
			t, http.StatusOK, elsewhereCited),
		"https://ntrs.nasa.gov/api/citations/20960000008": ntrsAnswer(
			t, http.StatusOK, strings.ReplaceAll(secondCitation,
				"20980000002", "20960000008"))})
	into := t.TempDir()
	get.EXPECT().Get("https://ntrs.nasa.gov/api/citations/20960000008/" +
		"downloads/20960000008.pdf").DoAndReturn(served)
	out, errs, code := ran(tool, "ntrs-fetch", "20990000001",
		"20980000002", "20970000009", "20960000008", "--into", into)
	want := "not on NTRS 20990000001\n" +
		"no PDF   20980000002\n" +
		"fetched  20960000008.pdf (A second crafted report)\n" +
		"ntrs-fetch: 1 fetched, 0 kept, 1 not on NTRS, 1 with no PDF, " +
		"in " + into + "; requests to NTRS: 5, one at a time\n"
	if code == exitOK || out != want || len(fake.sent) != 4 ||
		!strings.Contains(errs, "20990000001") ||
		!strings.Contains(errs, "holds no PDF") ||
		!strings.Contains(errs, "not a file of its own") {
		t.Errorf("exit %d: %q %q", code, out, errs)
	}
}

// the anti-test: when NTRS asks us to stop, the run stops at once, says
// which ids it has not asked for, and asks for no more, though a PDF
// before it was fetched and kept. The citation that is last has none
// after it to say.
func TestNTRSFetchStopsWhenNTRSAsksUsTo(t *testing.T) {
	tool, fake, get := askingNTRS(t, map[string]*http.Response{
		ntrsFirst:  ntrsAnswer(t, http.StatusOK, "@citation.json"),
		ntrsSecond: ntrsAnswer(t, http.StatusTooManyRequests, "")})
	into := t.TempDir()
	get.EXPECT().Get(firstPDF).DoAndReturn(served)
	out, errs, code := ran(tool, "ntrs-fetch", "20990000001",
		"20980000002", "20970000003", "--into", into)
	if code == exitOK || len(fake.sent) != 2 ||
		!strings.Contains(out, "fetched  "+firstLabel) ||
		!strings.Contains(errs, "asked us to stop") ||
		!strings.Contains(errs, "stopped with 1 not asked for: "+
			"20970000003") {
		t.Errorf("exit %d: %q %q", code, out, errs)
	}
	last, fake2, _ := askingNTRS(t, map[string]*http.Response{
		ntrsSecond: ntrsAnswer(t, http.StatusTooManyRequests, "")})
	_, errs, code = ran(last, "ntrs-fetch", "20980000002", "--into", into)
	if code == exitOK || strings.Contains(errs, "stopped with") ||
		len(fake2.sent) != 1 {
		t.Errorf("exit %d: %q", code, errs)
	}
}

// the anti-test: a PDF that fails to come, or comes and is not a PDF,
// stops the run, as anything unexpected does, and what came is not kept.
func TestNTRSFetchStopsOnAPDFThatFails(t *testing.T) {
	into := t.TempDir()
	tool, _, get := askingNTRS(t, map[string]*http.Response{
		ntrsFirst:  ntrsAnswer(t, http.StatusOK, "@citation.json"),
		ntrsSecond: ntrsAnswer(t, http.StatusOK, secondCitation)})
	get.EXPECT().Get(firstPDF).Return(nil, int64(0), errors.New("the "+
		"line went dead"))
	_, errs, code := ran(tool, "ntrs-fetch", "20990000001",
		"20980000002", "--into", into)
	if code == exitOK || !strings.Contains(errs, "the line went dead") ||
		!strings.Contains(errs, "stopped with 1 not asked for: "+
			"20980000002") {
		t.Errorf("exit %d: %q", code, errs)
	}
	tool, _, get = askingNTRS(t, map[string]*http.Response{
		ntrsFirst: ntrsAnswer(t, http.StatusOK, "@citation.json")})
	html := "<html>sign in</html>"
	get.EXPECT().Get(firstPDF).Return(io.NopCloser(strings.NewReader(
		html)), int64(len(html)), nil)
	_, errs, code = ran(tool, "ntrs-fetch", "20990000001", "--into", into)
	_, statErr := os.Stat(filepath.Join(into, "20990000001.pdf"))
	if code == exitOK || !strings.Contains(errs, "not a PDF") ||
		!errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("exit %d: %q, %v", code, errs, statErr)
	}
}

// the anti-test: a lookup that fails in the network, or with a status
// that is not 404 or 429, stops the run too, saying what.
func TestNTRSFetchStopsOnALookupThatFails(t *testing.T) {
	tool, _, _ := askingNTRS(t, map[string]*http.Response{
		ntrsFirst: ntrsAnswer(t, http.StatusInternalServerError, "")})
	_, errs, code := ran(tool, "ntrs-fetch", "20990000001",
		"20980000002", "--into", t.TempDir())
	if code == exitOK || !strings.Contains(errs, "20990000001: ntrs: "+
		"NTRS answered with status 500") ||
		!strings.Contains(errs, "stopped with 1 not asked for") {
		t.Errorf("exit %d: %q", code, errs)
	}
}

// the test: a dry run looks each citation up, says what it would fetch,
// and fetches nothing: its mock web expects no call. Nothing is made in
// the folder.
func TestNTRSFetchDryRunFetchesNothing(t *testing.T) {
	tool, _, _ := askingNTRS(t, map[string]*http.Response{
		ntrsFirst: ntrsAnswer(t, http.StatusOK, "@citation.json")})
	into := filepath.Join(t.TempDir(), "reports")
	out, errs, code := ran(tool, "ntrs-fetch", "20990000001", "--into",
		into, "--dry-run")
	want := "would fetch " + firstLabel + "\n" +
		"ntrs-fetch: 1 to fetch, 0 kept, 0 not on NTRS, 0 with no " +
		"PDF, in " + into + "; requests to NTRS: 1, one at a time\n"
	_, statErr := os.Stat(into)
	if code != exitOK || out != want ||
		!errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("exit %d: %q %q, %v", code, out, errs, statErr)
	}
}

// the anti-test: with no wait to hold the web to, a PDF is not fetched.
func TestNTRSFetchWithNoWebIsRefused(t *testing.T) {
	tool, _, _ := askingNTRS(t, nil)
	tool.get = nil
	tool.picked = picked(t, numbersWith(map[string]string{
		"ntrs-whole": "30"}))
	_, errs, code := ran(tool, "ntrs-fetch", "20990000001", "--into",
		t.TempDir())
	if code == exitOK || !strings.Contains(errs, "wait") {
		t.Errorf("exit %d: %q", code, errs)
	}
}

// Example_ntrsFetch says what a dry run would bring. NTRS is a mock
// here, and its answer is crafted.
func Example_ntrsFetch() {
	raw, err := os.ReadFile(crafted + "citation.json")
	if err != nil {
		panic(err)
	}
	ask := mocks.NewMockDoer(gomock.NewController(stopping{}))
	ask.EXPECT().Do(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusOK, Header: http.Header{},
		Body: io.NopCloser(bytes.NewReader(raw))}, nil)
	tool := command{picked: celestialdata.NewMeasured(numbersFile,
		numbersRaw), reports: ask,
		get: mocks.NewMockWeb(gomock.NewController(stopping{}))}
	var out, errs bytes.Buffer
	tool.run([]string{"ntrs-fetch", "20990000001", "--into", "reports",
		"--dry-run"}, strings.NewReader(""), &out, &errs)
	fmt.Print(out.String(), errs.String())
	// Output:
	// would fetch 20990000001.pdf (A crafted report about nothing)
	// ntrs-fetch: 1 to fetch, 0 kept, 0 not on NTRS, 0 with no PDF, in reports; requests to NTRS: 1, one at a time
}

// ntrs is one of the archives celestial-data lists, asked by name only
// and holding nothing near a place, so a fetch of it brings nothing.
func TestNTRSIsListedAndFindsNothingNearAPlace(t *testing.T) {
	out, errs, code := ran(command{get: quiet(t),
		picked: picked(t, numbersRaw)}, "archives")
	if code != exitOK || !strings.Contains(out, "ntrs             NASA's "+
		"Technical Reports Server") {
		t.Errorf("exit %d: %q %q", code, out, errs)
	}
	out, errs, code = ran(command{get: quiet(t),
		picked: picked(t, numbersRaw)}, "find", "--archive", "ntrs",
		"--ra", "266.4", "--dec", "-29", "--radius", "0.1")
	if code != exitOK || strings.Contains(out, "20990000001") {
		t.Errorf("exit %d: %q %q", code, out, errs)
	}
}
