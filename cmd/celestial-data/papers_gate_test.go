package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/janearc/celestial-data/arxiv"
	"github.com/janearc/celestial-data/mocks"
	"go.uber.org/mock/gomock"
)

// page is what arXiv sends when it sends a page and not a PDF.
const page = "<html>not yet</html>"

// paged is a page, as a web serves it in place of a paper.
func paged(string) (io.ReadCloser, int64, error) {
	content := strings.NewReader(page)
	return io.NopCloser(content), content.Size(), nil
}

// Two papers are fetched one after the other, each into its own file.
func TestPapersFetchesOneAfterAnother(t *testing.T) {
	into := t.TempDir()
	tool := arxivOver(t, recordIn(t), numbersRaw)
	answer := reply(http.StatusOK, "two-papers.xml", "")
	tool.search.EXPECT().RawSearch(gomock.Any(),
		query("1809.03507", "astro-ph/0602086")).Return(answer, nil)
	first := tool.get.EXPECT().Get(bennettPDF).DoAndReturn(served)
	second := tool.get.EXPECT().Get(kaplanPDF).DoAndReturn(served)
	gomock.InOrder(first, second)
	out, errs, code := ran(tool.command, "papers", "1809.03507",
		"astro-ph/0602086", "--into", into)
	for _, file := range []string{"1809.03507v2.pdf",
		"astro-ph_0602086v1.pdf"} {
		if held, err := os.ReadFile(filepath.Join(into, file)); err != nil ||
			string(held) != pdf {
			t.Errorf("%s: %q, %v", file, held, err)
		}
	}
	if code != exitOK || !strings.Contains(out, "2 fetched") {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

// Anti-test: what arXiv sends that is not a PDF is not kept, and the
// run says so.
func TestPapersKeepsNothingThatIsNotAPDF(t *testing.T) {
	into := t.TempDir()
	tool := arxivOver(t, recordIn(t), numbersRaw)
	answer := reply(http.StatusOK, "two-papers.xml", "")
	tool.search.EXPECT().RawSearch(gomock.Any(), query("1809.03507")).
		Return(answer, nil)
	tool.get.EXPECT().Get(bennettPDF).DoAndReturn(paged)
	_, errs, code := ran(tool.command, "papers", "1809.03507", "--into",
		into)
	_, err := os.Stat(filepath.Join(into, "1809.03507v2.pdf"))
	if code == exitOK || !os.IsNotExist(err) ||
		!strings.Contains(errs, "1809.03507v2: arXiv sent what is not a "+
			"PDF, and it was not kept") {
		t.Errorf("%d %q %v", code, errs, err)
	}
}

// Anti-test: a file of a paper's name that is not a PDF, or that is a
// folder, is not taken for the paper; the run is refused, saying how to
// clear it, and arXiv is asked nothing.
func TestPapersTakesNoOtherFileForAPaper(t *testing.T) {
	into := t.TempDir()
	path := filepath.Join(into, "1809.03507v2.pdf")
	if err := os.WriteFile(path, []byte(page), paperMode); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(into, "astro-ph_0602086v1.pdf")
	if err := os.Mkdir(folder, paperFolder); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"1809.03507", "astro-ph/0602086"} {
		tool := arxivOver(t, recordIn(t), numbersRaw)
		_, errs, code := ran(tool.command, "papers", id, "--into", into)
		if code == exitOK || !strings.Contains(errs, "is not a PDF") ||
			!strings.Contains(errs, "Move it aside and run again") {
			t.Errorf("%s: %d %q", id, code, errs)
		}
	}
}

// paperFolder is the mode of a folder a test makes where a paper might
// be. It's defined: written by its owner.
const paperFolder = 0o755

// Anti-test: while another run holds the gate's record, a run is
// refused, saying the lock, and arXiv is asked nothing.
func TestPapersWaitsForNoOtherRun(t *testing.T) {
	record := recordIn(t)
	other := arxiv.Wall()
	gate, err := arxiv.NewGate(time.Minute, other)
	if err != nil {
		t.Fatal(err)
	}
	if err := gate.Hold(record); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(holding{gate}.release)
	tool := arxivOver(t, record, numbersRaw)
	_, errs, code := ran(tool.command, "papers", "1809.03507", "--into",
		t.TempDir())
	if code == exitOK || !strings.Contains(errs, "another run is asking "+
		"arXiv") {
		t.Errorf("%d %q", code, errs)
	}
}

// A run after arXiv asked us to stop asks nothing while the wait it
// asked has not passed, and says when it will have.
func TestPapersCarriesAStopToTheNextRun(t *testing.T) {
	record := recordIn(t)
	first := arxivOver(t, record, numbersRaw)
	busy := reply(http.StatusTooManyRequests, "no-such-paper.xml",
		farOff)
	first.search.EXPECT().RawSearch(gomock.Any(), query("1809.03507")).
		Return(busy, nil)
	ran(first.command, "papers", "1809.03507", "--into", t.TempDir())
	next := arxivOver(t, record, numbersRaw)
	_, errs, code := ran(next.command, "papers", "1809.03507", "--into",
		t.TempDir())
	if code == exitOK || !strings.Contains(errs, "arXiv asked us to "+
		"stop: it asked to be left until") {
		t.Errorf("%d %q", code, errs)
	}
}

// holding is another run's gate, let go when the test ends.
type holding struct{ gate *arxiv.Gate }

// release lets the other run's gate go.
func (other holding) release() { other.gate.Release() }

// farOff is a Retry-After past any hour the tests' clock reaches. It's
// defined: a year, in seconds.
const farOff = "31536000"

// The gate's record is kept in celestial-data's folder of the user's
// cache, when no test says where.
func TestPapersKeepsTheRecordInTheUsersCache(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", "")
	var tool command
	path, err := tool.recordPath()
	cache, cacheErr := os.UserCacheDir()
	want := filepath.Join(cache, "celestial-data", "arxiv-gate")
	if err != nil || cacheErr != nil || path != want {
		t.Errorf("%q, %v, %v", path, err, cacheErr)
	}
}

// Anti-test: with no cache folder for the user, papers says it has
// nowhere to keep arXiv's pace. (pzm itself refuses to run without a
// home, so this is asked of the command directly.)
func TestPapersRefusesWithNowhereToKeepThePace(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	says := "keeps arXiv's pace between runs in the user's cache " +
		"folder, and finds none"
	var bare command
	path, err := bare.recordPath()
	if path != "" || err == nil || !strings.Contains(err.Error(), says) {
		t.Errorf("%q, %v", path, err)
	}
	tool := arxivOver(t, "", numbersRaw)
	run := &papersRun{out: io.Discard, into: t.TempDir()}
	err = tool.fetchPapers(run, []arxiv.ID{parsedID(t, "1809.03507")})
	if err == nil || !strings.Contains(err.Error(), says) {
		t.Errorf("%v", err)
	}
}

// parsedID is an id, failing the test when it will not read.
func parsedID(t *testing.T, written string) arxiv.ID {
	t.Helper()
	id, err := arxiv.ParseID(written)
	if err != nil {
		t.Fatal(err)
	}
	return *id
}

// Anti-test: a dry run asks the API and nothing more: its clock is read
// for the one query, and nothing is waited for a PDF.
func TestPapersDryRunWaitsForNoPDF(t *testing.T) {
	control := gomock.NewController(t)
	tool := arxivOver(t, recordIn(t), numbersRaw)
	clock := mocks.NewMockClock(control)
	tool.clock = clock
	clock.EXPECT().Now().Return(begun()).Times(readingsARequest)
	answer := reply(http.StatusOK, "two-papers.xml", "")
	tool.search.EXPECT().RawSearch(gomock.Any(),
		query("1809.03507", "astro-ph/0602086")).Return(answer, nil)
	_, errs, code := ran(tool.command, "papers", "1809.03507",
		"astro-ph/0602086", "--into", t.TempDir(), "--dry-run")
	if code != exitOK {
		t.Errorf("%d %q", code, errs)
	}
}
