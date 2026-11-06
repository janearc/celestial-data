package arxiv_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	library "github.com/Epistemic-Technology/arxiv/arxiv"
	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/arxiv"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
)

// The ids a query asks, and the papers a look gives, in these tests.
const (
	noPaper   = 0 // defined: none
	onePaper  = 1 // defined: one
	twoPapers = 2 // defined: the two papers recorded
)

// wait is the Retry-After the answers that ask us to wait give here.
// It's defined: any wait will do.
const wait = "30"

// answered is an answer of a status with a recorded reply as its body,
// and a Retry-After when one is given.
func answered(t *testing.T, code int, reply, retry string) *http.Response {
	t.Helper()
	body, err := os.ReadFile("testdata/" + reply)
	if err != nil {
		t.Fatal(err)
	}
	answer := &http.Response{StatusCode: code,
		Status: fmt.Sprintf("%d %s", code, http.StatusText(code)),
		Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(body))}
	if retry != "" {
		answer.Header.Set("Retry-After", retry)
	}
	return answer
}

// asked is the search for ids, as Look asks it.
func asked(ids ...string) library.SearchParams {
	return library.SearchParams{IdList: ids, MaxResults: len(ids)}
}

// ids are ids these tests read, failing the test when one will not.
func ids(t *testing.T, written ...string) []arxiv.ID {
	var read []arxiv.ID
	for _, each := range written {
		read = append(read, parsed(t, each))
	}
	return read
}

// papersOver is papers found through a mock searcher, most ids a query,
// with a gate on a clock that never waits.
func papersOver(t *testing.T, most int) (*arxiv.Papers,
	*mocks.MockSearcher, *arxiv.Gate) {
	t.Helper()
	search := mocks.NewMockSearcher(gomock.NewController(t))
	gate := gateOn(t, hourly(t))
	papers, err := arxiv.New(search, gate, most)
	if err != nil {
		t.Fatal(err)
	}
	return papers, search, gate
}

// described is each product on a line: its id, its file, where it is
// fetched from, and what it is.
func described(products []*celestialdata.Product) []string {
	var lines []string
	for _, product := range products {
		line := fmt.Sprintf("%s %s %s %s: %s", product.Archive(),
			product.ID(), product.Name(), product.URL(), product.About())
		lines = append(lines, line)
	}
	return lines
}

// writtenAs is ids as arXiv writes them.
func writtenAs(read []arxiv.ID) []string {
	var lines []string
	for _, id := range read {
		lines = append(lines, id.String())
	}
	return lines
}

// Papers are found in the order asked, though arXiv answers in its own,
// each named for the version arXiv has, fetched from export.arxiv.org,
// and said by who wrote it, when, and its title.
func TestPapersAreFoundAsTheirVersions(t *testing.T) {
	papers, search, gate := papersOver(t, twoPapers)
	answer := answered(t, http.StatusOK, "two-papers.xml", "")
	search.EXPECT().RawSearch(gomock.Any(),
		asked("1809.03507", "astro-ph/0602086")).Return(answer, nil)
	found, missing, err := papers.Look(context.Background(),
		ids(t, "1809.03507", "astro-ph/0602086"))
	want := []string{"arxiv 1809.03507v2 1809.03507v2.pdf " +
		"https://export.arxiv.org/pdf/1809.03507v2: Bennett and Bovy " +
		"2018: Vertical waves in the solar neighbourhood in Gaia DR2",
		"arxiv astro-ph/0602086v1 astro-ph_0602086v1.pdf " +
			"https://export.arxiv.org/pdf/astro-ph/0602086v1: Kaplan " +
			"2006: The IAU Resolutions on Astronomical Reference " +
			"Systems, Time Scales, and Earth Rotation Models"}
	if err != nil || len(missing) != noPaper ||
		!slices.Equal(described(found), want) ||
		gate.Asked() != oneRequest {
		t.Errorf("%q, %v, %v, %d asked", described(found), missing, err,
			gate.Asked())
	}
}

// Anti-test: a paper arXiv does not have is said missing, and nothing
// is found for it.
func TestAPaperArXivDoesNotHaveIsMissing(t *testing.T) {
	papers, search, _ := papersOver(t, twoPapers)
	answer := answered(t, http.StatusOK, "no-such-paper.xml", "")
	search.EXPECT().RawSearch(gomock.Any(), asked("1809.99999")).
		Return(answer, nil)
	found, missing, err := papers.Look(context.Background(),
		ids(t, "1809.99999"))
	if err != nil || len(found) != noPaper ||
		!slices.Equal(writtenAs(missing), []string{"1809.99999"}) {
		t.Errorf("%v, %v, %v", found, missing, err)
	}
}

// A paper is said by its authors as a citation writes them: one, two
// joined by and, the first and et al., or that none or no name was
// given; and its title on one line.
func TestAPaperIsSaidAsACitationSaysIt(t *testing.T) {
	papers, search, _ := papersOver(t, len(crafted))
	answer := answered(t, http.StatusOK, "crafted-authors.xml", "")
	search.EXPECT().RawSearch(gomock.Any(), asked(crafted...)).
		Return(answer, nil)
	found, _, err := papers.Look(context.Background(),
		ids(t, crafted...))
	var abouts []string
	for _, product := range found {
		abouts = append(abouts, product.About())
	}
	want := []string{"no author named 2020: No one wrote this",
		"Bovy 2020: One wrote this",
		"someone unnamed and Bovy 2020: A nameless one and another",
		"Abuter et al. 2020: Three wrote this"}
	if err != nil || !slices.Equal(abouts, want) {
		t.Errorf("%q, %v", abouts, err)
	}
}

// crafted are the ids of the made-up papers in crafted-authors.xml.
var crafted = []string{"2001.00001", "2001.00002", "2001.00003",
	"2001.00004"}

// A version asked for is the version found.
func TestAVersionAskedForIsFound(t *testing.T) {
	papers, search, _ := papersOver(t, twoPapers)
	answer := answered(t, http.StatusOK, "two-papers.xml", "")
	search.EXPECT().RawSearch(gomock.Any(), asked("1809.03507v2")).
		Return(answer, nil)
	found, _, err := papers.Look(context.Background(),
		ids(t, "1809.03507v2"))
	want := []string{"arxiv 1809.03507v2 1809.03507v2.pdf " +
		"https://export.arxiv.org/pdf/1809.03507v2: Bennett and Bovy " +
		"2018: Vertical waves in the solar neighbourhood in Gaia DR2"}
	if err != nil || !slices.Equal(described(found), want) {
		t.Errorf("%q, %v", described(found), err)
	}
}

// Anti-test: a version arXiv did not answer is missing, as a paper it
// does not have is, and the other ids of its query are found.
func TestAVersionArXivDidNotAnswerIsMissing(t *testing.T) {
	papers, search, _ := papersOver(t, twoPapers)
	answer := answered(t, http.StatusOK, "two-papers.xml", "")
	search.EXPECT().RawSearch(gomock.Any(),
		asked("1809.03507v1", "astro-ph/0602086")).Return(answer, nil)
	found, missing, err := papers.Look(context.Background(),
		ids(t, "1809.03507v1", "astro-ph/0602086"))
	if err != nil || len(found) != onePaper ||
		!slices.Equal(writtenAs(missing), []string{"1809.03507v1"}) {
		t.Errorf("%q, %v, %v", described(found), missing, err)
	}
}

// Ids are asked so many a query, in order, each query through the gate,
// and what is found and missing gathers across them.
func TestIdsAreAskedSoManyAQuery(t *testing.T) {
	papers, search, gate := papersOver(t, onePaper)
	two := answered(t, http.StatusOK, "two-papers.xml", "")
	none := answered(t, http.StatusOK, "no-such-paper.xml", "")
	first := search.EXPECT().RawSearch(gomock.Any(), asked("1809.03507")).
		Return(two, nil)
	second := search.EXPECT().RawSearch(gomock.Any(),
		asked("1809.99999")).Return(none, nil)
	gomock.InOrder(first, second)
	found, missing, err := papers.Look(context.Background(),
		ids(t, "1809.03507", "1809.99999"))
	if err != nil || len(found) != onePaper ||
		len(missing) != onePaper || gate.Asked() != twoRequests {
		t.Errorf("%v, %v, %v, %d asked", found, missing, err,
			gate.Asked())
	}
}

// asking is an answer that asks us to wait, and what the look says of
// it.
type asking struct {
	code        int
	retry, says string
}

// Anti-test: arXiv asking us to stop, with a 403, 429 or 503, stops the
// look at once with what it said, its Retry-After when it gave one, and
// keeps what was found before.
func TestArXivAskingUsToStopStopsTheLook(t *testing.T) {
	for _, each := range []asking{
		{http.StatusTooManyRequests, wait, "arXiv asked us to stop: 429 " +
			"Too Many Requests; its Retry-After is " + wait},
		{http.StatusServiceUnavailable, "", "arXiv asked us to stop: " +
			"503 Service Unavailable"},
		{http.StatusForbidden, "", "arXiv asked us to stop: 403 " +
			"Forbidden"},
	} {
		papers, search, gate := papersOver(t, onePaper)
		two := answered(t, http.StatusOK, "two-papers.xml", "")
		busy := answered(t, each.code, "no-such-paper.xml", each.retry)
		first := search.EXPECT().RawSearch(gomock.Any(),
			asked("1809.03507")).Return(two, nil)
		second := search.EXPECT().RawSearch(gomock.Any(),
			asked("1809.99999")).Return(busy, nil)
		gomock.InOrder(first, second)
		found, _, err := papers.Look(context.Background(),
			ids(t, "1809.03507", "1809.99999", "1412.3451"))
		if !arxiv.AskedToStop(err) || err.Error() != each.says ||
			len(found) != onePaper || gate.Asked() != twoRequests {
			t.Errorf("%d: %v, %v", each.code, found, err)
		}
	}
}

// A query arXiv refuses is refused in arXiv's words.
func TestAQueryArXivRefusesIsRefusedInItsWords(t *testing.T) {
	papers, search, _ := papersOver(t, twoPapers)
	answer := answered(t, http.StatusBadRequest, "not-an-id.xml", "")
	search.EXPECT().RawSearch(gomock.Any(), asked("1809.03507")).
		Return(answer, nil)
	_, _, err := papers.Look(context.Background(), ids(t, "1809.03507"))
	if !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "400 Bad Request; incorrect id "+
			"format for not-an-id") {
		t.Errorf("%v", err)
	}
}

// Anti-test: an entry that is not a paper, in an answer that says it
// is a success, is refused in its own words.
func TestAnEntryThatIsNotAPaperIsRefused(t *testing.T) {
	papers, search, _ := papersOver(t, twoPapers)
	answer := answered(t, http.StatusOK, "not-an-id.xml", "")
	search.EXPECT().RawSearch(gomock.Any(), asked("1809.03507")).
		Return(answer, nil)
	_, _, err := papers.Look(context.Background(), ids(t, "1809.03507"))
	if !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "which is not a paper: incorrect "+
			"id format for not-an-id") {
		t.Errorf("%v", err)
	}
}

// failure is an answer, or a search's error, and what the look says.
type failure struct {
	answer *http.Response
	err    error
	says   string
}

// Anti-test: any other answer, a search that fails, and a body that is
// not Atom are failures that name the adapter, and ask no wait.
func TestOtherAnswersAreFailures(t *testing.T) {
	broken := answered(t, http.StatusOK, "two-papers.xml", "")
	broken.Body = io.NopCloser(strings.NewReader("<feed><entry>"))
	errored := answered(t, http.StatusInternalServerError,
		"two-papers.xml", "")
	for _, each := range []failure{
		{errored, nil, "arxiv: the API answered 500 Internal Server " +
			"Error"},
		{nil, errors.New("no route to host"), "arxiv: no route to host"},
		{broken, nil, "arxiv: XML syntax error"},
	} {
		papers, search, _ := papersOver(t, twoPapers)
		search.EXPECT().RawSearch(gomock.Any(), asked("1809.03507")).
			Return(each.answer, each.err)
		_, _, err := papers.Look(context.Background(),
			ids(t, "1809.03507"))
		if arxiv.AskedToStop(err) || err == nil ||
			!strings.HasPrefix(err.Error(), each.says) {
			t.Errorf("%v", err)
		}
	}
}

// Anti-test: a look whose context has ended asks nothing.
func TestAnEndedLookAsksNothing(t *testing.T) {
	papers, _, gate := papersOver(t, twoPapers)
	_, _, err := papers.Look(ended(), ids(t, "1809.03507"))
	if err != context.Canceled || gate.Asked() != noRequest {
		t.Errorf("%v, %d asked", err, gate.Asked())
	}
}

// making is what New is given.
type making struct {
	search arxiv.Searcher
	gate   *arxiv.Gate
	most   int
}

// Anti-test: papers refuse to be made without a searcher, a gate, or
// one id a query at least.
func TestPapersRefuseWhatTheyNeed(t *testing.T) {
	_, search, gate := papersOver(t, onePaper)
	for _, given := range []making{{nil, gate, onePaper},
		{search, nil, onePaper}, {search, gate, noPaper}} {
		papers, err := arxiv.New(given.search, given.gate, given.most)
		if papers != nil || !errors.Is(err, celestialdata.Refused()) {
			t.Errorf("%v, %v", papers, err)
		}
	}
}

// refusing is a server that answers every request with one status, and
// a Retry-After with it.
type refusing int

// ServeHTTP answers with the status, and the wait.
func (code refusing) ServeHTTP(answer http.ResponseWriter,
	_ *http.Request) {
	answer.Header().Set("Retry-After", wait)
	answer.WriteHeader(int(code))
}

// refusal is a web's refusal of a request to a server answering with a
// status, as web.Client makes it.
func refusal(t *testing.T, code int) error {
	t.Helper()
	server := httptest.NewServer(refusing(code))
	t.Cleanup(server.Close)
	client := web.New(seconds(t, "patient-wait"), seconds(t,
		"patient-wait"), "celestial-data-test")
	_, _, err := client.Get(server.URL)
	return err
}

// refusedThrough is what a paced web says of a PDF the web under it
// refuses with err, for a Get and a Head.
func refusedThrough(t *testing.T, err error) (error, error) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Get(paper).Return(nil, int64(noPaper), err)
	get.EXPECT().Head(paper).Return(int64(noPaper), err)
	gate := gateOn(t, hourly(t))
	through := pacedOn(context.Background(), t, get, gate)
	_, _, got := through.Get(paper)
	_, headed := through.Head(paper)
	return got, headed
}

// A PDF refused with 403, 429 or 503 through a paced web is arXiv
// asking us to stop, said with the request and the wait asked, wrapped
// or not.
func TestAStopAskedIsKnownFromAPDF(t *testing.T) {
	for _, code := range []int{http.StatusForbidden,
		http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		refused := refusal(t, code)
		got, headed := refusedThrough(t, refused)
		says := "arXiv asked us to stop: " + refused.Error() +
			"; its Retry-After is " + wait
		for _, err := range []error{got, headed,
			fmt.Errorf("fetching: %w", got)} {
			if !arxiv.AskedToStop(err) || !strings.HasSuffix(err.Error(),
				says) {
				t.Errorf("%d: %v", code, err)
			}
		}
	}
}

// Anti-test: any other refusal through a paced web is given as it is,
// and asks no stop, and nor does any other error.
func TestOtherErrorsAskNoStop(t *testing.T) {
	missing := refusal(t, http.StatusNotFound)
	got, headed := refusedThrough(t, missing)
	for _, err := range []error{got, headed, nil, errors.New("gone")} {
		if arxiv.AskedToStop(err) {
			t.Errorf("%v", err)
		}
	}
	if got != missing || headed != missing {
		t.Errorf("%v, %v", got, headed)
	}
}

// counting is an API that counts what is asked of it and answers 503.
type counting struct {
	hits  atomic.Int32
	query atomic.Value
}

// ServeHTTP counts the request, keeps its query, and answers 503.
func (api *counting) ServeHTTP(answer http.ResponseWriter,
	request *http.Request) {
	api.hits.Add(oneRequest)
	api.query.Store(request.URL.RawQuery)
	answer.WriteHeader(http.StatusServiceUnavailable)
}

// The searcher asks the API at its address for the ids, once: its own
// retries are off, so a 503 is given back, not asked again.
func TestTheSearcherAsksOnceAndGivesTheAnswerBack(t *testing.T) {
	api := &counting{}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	searcher := arxiv.NewSearcher(server.URL, seconds(t, "patient-wait"))
	answer, err := searcher.RawSearch(context.Background(),
		asked("1809.03507"))
	if err != nil {
		t.Fatal(err)
	}
	answer.Body.Close()
	query, _ := api.query.Load().(string)
	if answer.StatusCode != http.StatusServiceUnavailable ||
		api.hits.Load() != oneRequest ||
		!strings.Contains(query, "id_list=1809.03507") {
		t.Errorf("%d, %d hits, %q", answer.StatusCode, api.hits.Load(),
			query)
	}
}

// slow is an API that waits so long before it answers.
type slow time.Duration

// ServeHTTP waits, and answers nothing more.
func (delay slow) ServeHTTP(http.ResponseWriter, *http.Request) {
	time.Sleep(time.Duration(delay))
}

// Anti-test: the searcher gives up on an API slower than its wait.
func TestTheSearcherGivesUpOnASlowAPI(t *testing.T) {
	delay := slow(seconds(t, "slow-api"))
	server := httptest.NewServer(delay)
	t.Cleanup(server.Close)
	searcher := arxiv.NewSearcher(server.URL, seconds(t, "hasty-wait"))
	if _, err := searcher.RawSearch(context.Background(),
		asked("1809.03507")); err == nil {
		t.Error("a slow API was waited on")
	}
}
