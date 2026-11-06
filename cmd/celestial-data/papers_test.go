package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	library "github.com/Epistemic-Technology/arxiv/arxiv"
	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/arxiv"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
)

// replies is where arXiv's replies, recorded for the tests, are kept.
const replies = "../../arxiv/testdata/"

// The addresses of the two recorded papers' PDFs.
const (
	bennettPDF = "https://export.arxiv.org/pdf/1809.03507v2"
	kaplanPDF  = "https://export.arxiv.org/pdf/astro-ph/0602086v1"
)

// pdf is what a fetched paper holds in these tests.
const pdf = "%PDF-1.5 a paper"

// pdfLength is how long the paper is. It's defined: pdf's own length.
const pdfLength = int64(len(pdf))

// pdfWait is the Retry-After a refused PDF gives here. It's defined:
// any wait will do.
const pdfWait = "60"

// paperMode is the mode a paper put in a folder by a test has. It's
// defined: read and written by its owner, read by others.
const paperMode = 0o644

// morning is when the clocks in these tests begin. It's defined: any
// time will do.
const morning = "2026-11-06T09:00:00Z"

// readingsARequest is how often a gate reads its clock for a request
// that waits for nothing. It's defined by the gate: once for the wait,
// once for when the request began.
const readingsARequest = 2

// reply is an answer of a status with a recorded reply as its body, and
// a Retry-After when one is given.
func reply(code int, recorded, retry string) *http.Response {
	body, err := os.ReadFile(replies + recorded)
	if err != nil {
		panic(err)
	}
	answer := &http.Response{StatusCode: code,
		Status: fmt.Sprintf("%d %s", code, http.StatusText(code)),
		Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(body))}
	if retry != "" {
		answer.Header.Set("Retry-After", retry)
	}
	return answer
}

// query is the search papers makes for ids.
func query(ids ...string) library.SearchParams {
	return library.SearchParams{IdList: ids, MaxResults: len(ids)}
}

// ticking is a time that moves on by a step each time it is read.
type ticking struct {
	now  time.Time
	step time.Duration
}

// read is the time, a step on from the last reading.
func (clock *ticking) read() time.Time {
	clock.now = clock.now.Add(clock.step)
	return clock.now
}

// begun is the morning, as a time.
func begun() time.Time {
	at, err := time.Parse(time.RFC3339, morning)
	if err != nil {
		panic(err)
	}
	return at
}

// hourly is a mock clock an hour later each time it is read, so a gate
// on it never waits.
func hourly(control *gomock.Controller) *mocks.MockClock {
	clock := mocks.NewMockClock(control)
	hours := &ticking{now: begun(), step: time.Hour}
	clock.EXPECT().Now().DoAndReturn(hours.read).AnyTimes()
	return clock
}

// arxivTool is celestial-data asking through mocks: a web, arXiv's
// search and a clock that never waits, on the numbers given.
type arxivTool struct {
	command
	get    *mocks.MockWeb
	search *mocks.MockSearcher
}

// arxivOver is celestial-data on mocks that report to a reporter, its
// gate's record kept at record, with the numbers given.
func arxivOver(reporter gomock.TestReporter, record string,
	raw []byte) arxivTool {
	control := gomock.NewController(reporter)
	get, search := mocks.NewMockWeb(control), mocks.NewMockSearcher(control)
	picked := celestialdata.NewMeasured(numbersFile, raw)
	return arxivTool{command: command{get: get, search: search,
		clock: hourly(control), picked: picked, record: record}, get: get,
		search: search}
}

// recordIn is where a test keeps the gate's record, in its own folder.
func recordIn(t *testing.T) string {
	return filepath.Join(t.TempDir(), "arxiv-gate")
}

// served is a paper, as a web serves it.
func served(string) (io.ReadCloser, int64, error) {
	content := strings.NewReader(pdf)
	return io.NopCloser(content), content.Size(), nil
}

// refusing is a server that answers every request with one status, and
// a Retry-After with it.
type refusing int

// ServeHTTP answers with the status, and the wait.
func (code refusing) ServeHTTP(answer http.ResponseWriter,
	_ *http.Request) {
	answer.Header().Set("Retry-After", pdfWait)
	answer.WriteHeader(int(code))
}

// refusal is a web's refusal of a PDF from a server answering with a
// status, as web.Client makes it.
func refusal(t *testing.T, code int) error {
	t.Helper()
	server := httptest.NewServer(refusing(code))
	t.Cleanup(server.Close)
	_, _, err := web.New(time.Minute, time.Minute, agent).Get(server.URL)
	return err
}

// put puts a paper in a folder under a name, failing the test when it
// cannot.
func put(t *testing.T, folder, name string) {
	t.Helper()
	path := filepath.Join(folder, name)
	if err := os.WriteFile(path, []byte(pdf), paperMode); err != nil {
		t.Fatal(err)
	}
}

func Example_papers() {
	folder, err := os.MkdirTemp("", "papers-example")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(folder)
	record := filepath.Join(folder, "arxiv-gate")
	tool := arxivOver(stopping{}, record, numbersRaw)
	answer := reply(http.StatusOK, "two-papers.xml", "")
	tool.search.EXPECT().RawSearch(gomock.Any(),
		query("1809.03507", "astro-ph/0602086")).Return(answer, nil)
	out, _, _ := ran(tool.command, "papers", "1809.03507",
		"arXiv:astro-ph/0602086", "--into", "papers", "--dry-run")
	fmt.Print(out)
	// Output:
	// would fetch 1809.03507v2.pdf (Bennett and Bovy 2018: Vertical waves in the solar neighbourhood in Gaia DR2)
	// would fetch astro-ph_0602086v1.pdf (Kaplan 2006: The IAU Resolutions on Astronomical Reference Systems, Time Scales, and Earth Rotation Models)
	// papers: 2 to fetch, 0 kept, 0 not on arXiv, in papers; requests to arXiv: 1, one at a time
}

// A paper not yet in the folder is looked up and fetched into it, and
// one already there, in any version, is kept and asks nothing.
func TestPapersFetchesWhatIsNotThereAndKeepsWhatIs(t *testing.T) {
	into := t.TempDir()
	put(t, into, "astro-ph_0602086v1.pdf")
	tool := arxivOver(t, recordIn(t), numbersRaw)
	answer := reply(http.StatusOK, "two-papers.xml", "")
	tool.search.EXPECT().RawSearch(gomock.Any(), query("1809.03507")).
		Return(answer, nil)
	tool.get.EXPECT().Get(bennettPDF).DoAndReturn(served)
	out, errs, code := ran(tool.command, "papers", "1809.03507",
		"astro-ph/0602086", "--into", into)
	held, err := os.ReadFile(filepath.Join(into, "1809.03507v2.pdf"))
	if code != exitOK || err != nil || string(held) != pdf ||
		out != "kept     astro-ph_0602086v1.pdf (already there)\n"+
			"fetched  1809.03507v2.pdf (Bennett and Bovy 2018: Vertical "+
			"waves in the solar neighbourhood in Gaia DR2)\n"+
			"papers: 1 fetched, 1 kept, 0 not on arXiv, in "+into+
			"; requests to arXiv: 2, one at a time\n" {
		t.Errorf("%d %q %q, %v %q", code, out, errs, err, held)
	}
}

// Anti-test: papers already there ask arXiv nothing at all, and need
// nothing of it: not even its numbers, which are missing here.
func TestPapersAlreadyThereAskNothing(t *testing.T) {
	into := t.TempDir()
	put(t, into, "1809.03507v1.pdf")
	put(t, into, "astro-ph_0602086v1.pdf")
	unpaced := strings.ReplaceAll(string(numbersRaw), "name: arxiv-pace",
		"name: arxiv-gone")
	tool := arxivOver(t, recordIn(t), []byte(unpaced))
	out, _, code := ran(tool.command, "papers", "1809.03507",
		"astro-ph/0602086v1", "--into", into)
	if code != exitOK || !strings.HasSuffix(out, "papers: 0 fetched, "+
		"2 kept, 0 not on arXiv, in "+into+"; requests to arXiv: 0, "+
		"one at a time\n") {
		t.Errorf("%d %q", code, out)
	}
}

// A list gives ids one a line, as words, arXiv: or addresses, with what
// follows a # and blank lines passed over, and each id asked once.
func TestPapersReadsAListOfIDs(t *testing.T) {
	folder := t.TempDir()
	list := filepath.Join(folder, "papers.txt")
	text := "# celestial's sources\n\nhttps://arxiv.org/pdf/1809.03507 " +
		"# bennett and bovy\n  arXiv:astro-ph/0602086\n1809.03507\n"
	if err := os.WriteFile(list, []byte(text), paperMode); err != nil {
		t.Fatal(err)
	}
	tool := arxivOver(t, recordIn(t), numbersRaw)
	answer := reply(http.StatusOK, "two-papers.xml", "")
	tool.search.EXPECT().RawSearch(gomock.Any(),
		query("1809.03507", "astro-ph/0602086")).Return(answer, nil)
	_, errs, code := ran(tool.command, "papers", "--list", list,
		"--into", folder, "--dry-run")
	if code != exitOK {
		t.Errorf("%d %q", code, errs)
	}
}

// Anti-test: a call with no folder, no ids, an id that is not one in
// the words or the list, or a list that is not there, is refused, and
// arXiv is asked nothing.
func TestPapersRefusesBeforeAskingAnything(t *testing.T) {
	into := t.TempDir()
	list := filepath.Join(into, "papers.txt")
	text := []byte("1809.03507\nnot-an-id\n")
	if err := os.WriteFile(list, text, paperMode); err != nil {
		t.Fatal(err)
	}
	for says, words := range map[string][]string{
		"needs --into":          {"1809.03507"},
		"needs ids":             {"--into", into},
		`"1809.035"`:            {"1809.035", "--into", into},
		`"not-an-id"`:           {"--list", list, "--into", into},
		"could not read --list": {"--list", list + ".gone", "--into", into},
	} {
		tool := arxivOver(t, recordIn(t), numbersRaw)
		asked := append([]string{"papers"}, words...)
		_, errs, code := ran(tool.command, asked...)
		if code == exitOK || !strings.Contains(errs, says) {
			t.Errorf("%s: %d %q", says, code, errs)
		}
	}
}

// A paper arXiv does not have is said, and the run fails, naming it.
func TestPapersSaysWhatIsNotOnArXiv(t *testing.T) {
	tool := arxivOver(t, recordIn(t), numbersRaw)
	answer := reply(http.StatusOK, "no-such-paper.xml", "")
	tool.search.EXPECT().RawSearch(gomock.Any(), query("1809.99999")).
		Return(answer, nil)
	out, errs, code := ran(tool.command, "papers", "1809.99999",
		"--into", t.TempDir())
	if code == exitOK || !strings.HasPrefix(out,
		"not on arXiv 1809.99999\n") ||
		!strings.Contains(errs, "1 not on arXiv: 1809.99999") ||
		!strings.Contains(out, ", 1 not on arXiv, in") {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

// When arXiv's API asks us to stop, the run stops, says what it had not
// fetched and that nothing more was asked, and fetches nothing.
func TestPapersStopsWhenTheAPIAsksUsToStop(t *testing.T) {
	tool := arxivOver(t, recordIn(t), numbersRaw)
	busy := reply(http.StatusTooManyRequests, "no-such-paper.xml", "60")
	tool.search.EXPECT().RawSearch(gomock.Any(),
		query("1809.03507", "astro-ph/0602086")).Return(busy, nil)
	out, errs, code := ran(tool.command, "papers", "1809.03507",
		"astro-ph/0602086", "--into", t.TempDir())
	if code == exitOK || !strings.Contains(errs, "arXiv asked us to "+
		"stop: 429 Too Many Requests; its Retry-After is 60. Stopped "+
		"with 2 not fetched: 1809.03507 astro-ph/0602086. Nothing more "+
		"was asked") || !strings.Contains(out, "requests to arXiv: 1,") {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

// When arXiv refuses a PDF with a 403 or a 503, the run stops there,
// saying the refusal and its Retry-After: that paper and those after it
// are not fetched, and nothing more is asked.
func TestPapersStopsWhenAPDFIsRefusedWithAStop(t *testing.T) {
	for _, code := range []int{http.StatusForbidden,
		http.StatusServiceUnavailable} {
		tool := arxivOver(t, recordIn(t), numbersRaw)
		answer := reply(http.StatusOK, "two-papers.xml", "")
		tool.search.EXPECT().RawSearch(gomock.Any(),
			query("1809.03507", "astro-ph/0602086")).Return(answer, nil)
		down := refusal(t, code)
		tool.get.EXPECT().Get(bennettPDF).Return(nil, pdfLength, down)
		out, errs, exit := ran(tool.command, "papers", "1809.03507",
			"astro-ph/0602086", "--into", t.TempDir())
		says := "arXiv asked us to stop: " + down.Error() + "; its " +
			"Retry-After is " + pdfWait + ". Stopped with 2 not fetched: " +
			"1809.03507v2 astro-ph/0602086v1"
		if exit == exitOK || !strings.Contains(errs, says) ||
			!strings.Contains(out, "0 fetched") {
			t.Errorf("%d: %d %q %q", code, exit, out, errs)
		}
	}
}

// Anti-test: a PDF that fails for another reason is said, and the rest
// are still fetched.
func TestAPDFThatFailsOtherwiseLetsTheRestCome(t *testing.T) {
	tool := arxivOver(t, recordIn(t), numbersRaw)
	answer := reply(http.StatusOK, "two-papers.xml", "")
	tool.search.EXPECT().RawSearch(gomock.Any(),
		query("1809.03507", "astro-ph/0602086")).Return(answer, nil)
	gone := refusal(t, http.StatusNotFound)
	tool.get.EXPECT().Get(bennettPDF).Return(nil, pdfLength, gone)
	tool.get.EXPECT().Get(kaplanPDF).DoAndReturn(served)
	out, errs, code := ran(tool.command, "papers", "1809.03507",
		"astro-ph/0602086", "--into", t.TempDir())
	if code == exitOK || !strings.Contains(errs, "1809.03507v2: GET") ||
		!strings.Contains(out, "1 fetched") {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

// Anti-test: an API answer that fails, and is not arXiv asking us to
// stop, is said as it is, with no word of stopping.
func TestPapersSaysAFailedLookAsItIs(t *testing.T) {
	tool := arxivOver(t, recordIn(t), numbersRaw)
	tool.search.EXPECT().RawSearch(gomock.Any(), query("1809.03507")).
		Return(nil, errors.New("no route to host"))
	_, errs, code := ran(tool.command, "papers", "1809.03507", "--into",
		t.TempDir())
	if code == exitOK || !strings.Contains(errs, "no route to host") ||
		strings.Contains(errs, "Stopped") {
		t.Errorf("%d %q", code, errs)
	}
}

// Every request goes through one gate: the PDF waits out the pace from
// the start of the API's query before it, as a second query would.
func TestPapersPacesThePDFsWithTheAPI(t *testing.T) {
	control := gomock.NewController(t)
	tool := arxivOver(t, recordIn(t), numbersRaw)
	clock := mocks.NewMockClock(control)
	tool.clock = clock
	pace := web.Seconds(tool.picked.Value("arxiv-pace"))
	start := begun()
	queried := clock.EXPECT().Now().Return(start).Times(readingsARequest)
	fetching := clock.EXPECT().Now().Return(start.Add(time.Second))
	sleeping := clock.EXPECT().Sleep(gomock.Any(), pace-time.Second).
		Return(nil)
	fetched := clock.EXPECT().Now().Return(start.Add(pace))
	gomock.InOrder(queried, fetching, sleeping, fetched)
	answer := reply(http.StatusOK, "two-papers.xml", "")
	tool.search.EXPECT().RawSearch(gomock.Any(), query("1809.03507")).
		Return(answer, nil)
	tool.get.EXPECT().Get(bennettPDF).DoAndReturn(served)
	_, errs, code := ran(tool.command, "papers", "1809.03507", "--into",
		t.TempDir())
	if code != exitOK {
		t.Errorf("%d %q", code, errs)
	}
}

// Anti-test: numbers that would ask faster than arXiv allows, or none a
// query, or that are missing, are refused before anything is asked.
func TestPapersRefusesNumbersThatWouldAskAmiss(t *testing.T) {
	text := string(numbersRaw)
	for says, raw := range map[string]string{
		"quicker than arXiv allows": strings.ReplaceAll(text,
			"name: arxiv-pace\n    value: 3", "name: arxiv-pace\n"+
				"    value: 2"),
		"at least one id a query": strings.ReplaceAll(text,
			"name: arxiv-most\n    value: 20", "name: arxiv-most\n"+
				"    value: 0"),
		"arxiv-pace": strings.ReplaceAll(text, "name: arxiv-pace",
			"name: arxiv-gone"),
	} {
		tool := arxivOver(t, recordIn(t), []byte(raw))
		_, errs, code := ran(tool.command, "papers", "1809.03507",
			"--into", t.TempDir())
		if code == exitOK || !strings.Contains(errs, says) {
			t.Errorf("%s: %d %q", says, code, errs)
		}
	}
}

// Anti-test: a folder that will not read is said, and arXiv is asked
// nothing.
func TestPapersSaysAFolderThatWillNotRead(t *testing.T) {
	folder := t.TempDir()
	put(t, folder, "not-a-folder")
	tool := arxivOver(t, recordIn(t), numbersRaw)
	_, errs, code := ran(tool.command, "papers", "1809.03507", "--into",
		filepath.Join(folder, "not-a-folder"))
	if code == exitOK || !strings.Contains(errs, "not a directory") {
		t.Errorf("%d %q", code, errs)
	}
}

// A command given no searcher or clock asks arXiv's API through the
// library and keeps the wall's time.
func TestPapersOwnSearcherAndClockAreArXivsAndTheWalls(t *testing.T) {
	var tool command
	if tool.clockOf() != arxiv.Wall() {
		t.Errorf("the clock is %T", tool.clockOf())
	}
	searcher := tool.searcherOf(time.Minute.Seconds())
	if _, client := searcher.(*library.Client); !client {
		t.Errorf("the searcher is %T", searcher)
	}
}
