package arxiv_test

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/janearc/celestial-data/arxiv"
	"github.com/janearc/celestial-data/mocks"
	"go.uber.org/mock/gomock"
)

// The two versions of the paper in two-versions.xml, as found.
const (
	firstVersion = "arxiv 1809.03507v1 1809.03507v1.pdf " +
		"https://export.arxiv.org/pdf/1809.03507v1: Bennett and Bovy " +
		"2018: Vertical waves in the solar neighbourhood in Gaia DR2"
	latestVersion = "arxiv 1809.03507v2 1809.03507v2.pdf " +
		"https://export.arxiv.org/pdf/1809.03507v2: Bennett and Bovy " +
		"2018: Vertical waves in the solar neighbourhood in Gaia DR2"
)

// versions is ids asked together, and the papers a look finds for them.
type versions struct {
	asked []string
	found []string
}

// An id with a version is that version, and one without is the latest
// arXiv answered, though both are in one answer; a version named twice
// is found once.
func TestEachIDIsItsVersionAndAVersionIsFoundOnce(t *testing.T) {
	for _, each := range []versions{
		{[]string{"1809.03507v1", "1809.03507"},
			[]string{firstVersion, latestVersion}},
		{[]string{"1809.03507", "1809.03507v2"},
			[]string{latestVersion}},
	} {
		papers, search, _ := papersOver(t, twoPapers)
		answer := answered(t, http.StatusOK, "two-versions.xml", "")
		search.EXPECT().RawSearch(gomock.Any(), asked(each.asked...)).
			Return(answer, nil)
		found, _, err := papers.Look(context.Background(),
			ids(t, each.asked...))
		if err != nil || !slices.Equal(described(found), each.found) {
			t.Errorf("%v: %q, %v", each.asked, described(found), err)
		}
	}
}

// An old-style id with a subject is asked for, and found, by the name
// arXiv gives the paper, without the subject.
func TestAnOldStyleIDIsAskedAsArXivNamesIt(t *testing.T) {
	papers, search, _ := papersOver(t, twoPapers)
	answer := answered(t, http.StatusOK, "old-style.xml", "")
	search.EXPECT().RawSearch(gomock.Any(), asked("math/0309136")).
		Return(answer, nil)
	found, missing, err := papers.Look(context.Background(),
		ids(t, "math.GT/0309136"))
	line := strings.Join(described(found), "")
	if err != nil || len(missing) != noPaper || len(found) != onePaper ||
		!strings.HasPrefix(line, "arxiv math/0309136v1 "+
			"math_0309136v1.pdf ") {
		t.Errorf("%q, %v, %v", described(found), missing, err)
	}
}

// closing is a body that says whether it was closed.
type closing struct {
	content io.Reader
	closed  bool
}

// Read reads what the body holds.
func (body *closing) Read(into []byte) (int, error) {
	return body.content.Read(into)
}

// Close notes that it was closed.
func (body *closing) Close() error {
	body.closed = true
	return nil
}

// Each answer the API gives is closed, whether it is read or refused.
func TestEachAnswerIsClosed(t *testing.T) {
	for _, code := range []int{http.StatusOK,
		http.StatusTooManyRequests, http.StatusInternalServerError} {
		papers, search, _ := papersOver(t, twoPapers)
		answer := answered(t, code, "two-papers.xml", "")
		body := &closing{content: answer.Body}
		answer.Body = body
		search.EXPECT().RawSearch(gomock.Any(), asked("1809.03507")).
			Return(answer, nil)
		papers.Look(context.Background(), ids(t, "1809.03507"))
		if !body.closed {
			t.Errorf("%d: the answer was not closed", code)
		}
	}
}

// The API is asked at export.arxiv.org, over https: the host arXiv sets
// aside for programs.
func TestTheAPIIsTheHostArXivSetsAsideForPrograms(t *testing.T) {
	if !strings.HasPrefix(arxiv.API, "https://export.arxiv.org/") {
		t.Errorf("the API is %s", arxiv.API)
	}
}

// Anti-test: after the API asked us to stop, the next query in the run
// is not sent while the wait it asked has not passed, and says so.
func TestAQueryWithinTheWaitTheAPIAskedIsNotSent(t *testing.T) {
	start, step := begun(t), seconds(t, "step")
	lasts := seconds(t, "asked-wait")
	clock := readings(t, start, start, start, start.Add(step))
	search := mocks.NewMockSearcher(gomock.NewController(t))
	gate := gateOn(t, clock)
	papers, err := arxiv.New(search, gate, onePaper)
	if err != nil {
		t.Fatal(err)
	}
	busy := answered(t, http.StatusTooManyRequests, "no-such-paper.xml",
		wait)
	search.EXPECT().RawSearch(gomock.Any(), asked("1809.03507")).
		Return(busy, nil)
	papers.Look(context.Background(), ids(t, "1809.03507"))
	_, _, err = papers.Look(context.Background(), ids(t, "1412.3451"))
	until := start.Add(lasts).Format(time.RFC3339)
	if !arxiv.AskedToStop(err) || gate.Asked() != onePaper ||
		!strings.Contains(err.Error(), "left until "+until) {
		t.Errorf("%v, %d asked", err, gate.Asked())
	}
}

// Anti-test: after arXiv asked us to stop, the next request in the run
// is not sent while the wait it asked has not passed, and says so.
func TestARequestWithinTheWaitAskedIsNotSent(t *testing.T) {
	start, step := begun(t), seconds(t, "step")
	asked := seconds(t, "asked-wait")
	clock := readings(t, start, start, start, start.Add(step))
	get := mocks.NewMockWeb(gomock.NewController(t))
	busy := refusal(t, http.StatusTooManyRequests)
	get.EXPECT().Get(paper).Return(nil, int64(noPaper), busy)
	gate := gateOn(t, clock)
	through := pacedOn(context.Background(), t, get, gate)
	through.Get(paper)
	_, err := through.Head(paper)
	until := start.Add(asked).Format(time.RFC3339)
	if !arxiv.AskedToStop(err) || gate.Asked() != onePaper ||
		!strings.Contains(err.Error(), "left until "+until) {
		t.Errorf("%v, %d asked", err, gate.Asked())
	}
}

// Anti-test: an old-style id without a subject is asked for as it is
// written; only a subject is taken from one.
func TestAnOldStyleIDWithoutASubjectIsAskedAsWritten(t *testing.T) {
	papers, search, _ := papersOver(t, twoPapers)
	answer := answered(t, http.StatusOK, "old-style.xml", "")
	search.EXPECT().RawSearch(gomock.Any(), asked("math/0309136")).
		Return(answer, nil)
	found, _, err := papers.Look(context.Background(),
		ids(t, "math/0309136"))
	if err != nil || len(found) != onePaper {
		t.Errorf("%q, %v", described(found), err)
	}
}

// Anti-test: an answer that is not Atom is closed too, and nothing is
// found in it.
func TestAnAnswerThatWillNotReadIsClosed(t *testing.T) {
	papers, search, _ := papersOver(t, twoPapers)
	answer := answered(t, http.StatusOK, "two-papers.xml", "")
	body := &closing{content: strings.NewReader("<feed><entry>")}
	answer.Body = body
	search.EXPECT().RawSearch(gomock.Any(), asked("1809.03507")).
		Return(answer, nil)
	found, _, err := papers.Look(context.Background(),
		ids(t, "1809.03507"))
	if err == nil || len(found) != noPaper || !body.closed {
		t.Errorf("%v, %v, closed %v", found, err, body.closed)
	}
}
