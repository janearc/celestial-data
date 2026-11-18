package ntrs_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/ntrs"
	"go.uber.org/mock/gomock"
	"pgregory.net/rapid"
)

// The agent and query these tests ask with. They're defined: any will
// do.
const (
	agent = "celestial-data-tests"
	query = "Sagittarius A* & the \"galactic centre\""
)

// The addresses NTRS keeps its search and its first crafted citation
// at, as its API description gives them.
const (
	endpoint = "https://ntrs.nasa.gov/api/citations/search"
	crafted  = "https://ntrs.nasa.gov/api/citations/20990000001"
	pdfAt    = crafted + "/downloads/20990000001.pdf"
)

// The counts these tests expect. The citations are counted from the
// fixtures in testdata, which are crafted.
const (
	noCitations = 0   // defined: none
	oneCitation = 1   // defined: one
	aFew        = 5   // defined: a few rows asked for
	total       = 115 // defined: stats.total in two-citations.json
	bodyKeys    = 2   // defined: q and page
	oneRune     = 1   // defined: the least a query holds
	unbounded   = -1  // defined: rapid's word for no bound
	justOver    = 101 // defined: one more than NTRS gives a page
	theMost     = 100 // defined: what NTRS gives a page
)

// closing is a body that counts how many times it was closed.
type closing struct {
	io.Reader
	closed int
}

// Close counts a close.
func (body *closing) Close() error {
	body.closed++
	return nil
}

// reply is an answer of a status with the text of a file in testdata as
// its body, or the text given when no file is named.
func reply(t *testing.T, code int, file, text string) (*http.Response,
	*closing) {
	t.Helper()
	if file != "" {
		raw, err := os.ReadFile("testdata/" + file)
		if err != nil {
			t.Fatal(err)
		}
		text = string(raw)
	}
	body := &closing{Reader: strings.NewReader(text)}
	return &http.Response{StatusCode: code,
		Status: fmt.Sprintf("%d %s", code, http.StatusText(code)),
		Header: http.Header{}, Body: body}, body
}

// keeper answers with an answer and keeps the request it was sent.
type keeper struct {
	answer *http.Response
	sent   *http.Request
}

// do is the keeper's part of a Doer.
func (kept *keeper) do(request *http.Request) (*http.Response, error) {
	kept.sent = request
	return kept.answer, nil
}

// doerAnswering is a mock Doer that answers once with an answer, and
// the keeper that keeps the request it was sent.
func doerAnswering(ctrl *gomock.Controller, answer *http.Response) (
	*mocks.MockDoer, *keeper) {
	kept := &keeper{answer: answer}
	doer := mocks.NewMockDoer(ctrl)
	doer.EXPECT().Do(gomock.Any()).DoAndReturn(kept.do)
	return doer, kept
}

// search is a search of the usual query through a mock Doer that answers
// with an answer. It gives what was found, the request sent, and the
// error.
func search(t *testing.T, answer *http.Response) (*ntrs.Found,
	*http.Request, error) {
	t.Helper()
	doer, kept := doerAnswering(gomock.NewController(t), answer)
	found, err := ntrs.Search(context.Background(), doer, query, aFew,
		agent)
	return found, kept.sent, err
}

// lookup is a lookup of the first crafted citation through a mock Doer
// that answers with an answer.
func lookup(t *testing.T, answer *http.Response) (*ntrs.Citation,
	*http.Request, error) {
	t.Helper()
	doer, kept := doerAnswering(gomock.NewController(t), answer)
	cited, err := ntrs.Lookup(context.Background(), doer, "20990000001",
		agent)
	return cited, kept.sent, err
}

// askedToStop is whether an error is NTRS asking us to stop.
func askedToStop(err error) bool {
	var stopped *ntrs.Stopped
	return errors.As(err, &stopped)
}

// described is a citation's every field on a line: its id, year, first
// author, title and PDF address.
func described(t *testing.T, cited ntrs.Citation) string {
	t.Helper()
	address, err := cited.PDF()
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%s|%s|%s|%s|%t|%s", cited.ID(), cited.Year(),
		cited.FirstAuthor(), cited.Title(), cited.HasPDF(), address)
}

// A search is one POST to the search endpoint, over https, on the host
// NTRS keeps for its API, of JSON that holds the query and the page, with
// the agent as the user agent.
//
// Anti-test: the query is in the body and not in the address, and the
// body holds nothing else.
func TestASearchAsksTheAPIAsNTRSDocumentsIt(t *testing.T) {
	answer, _ := reply(t, http.StatusOK, "two-citations.json", "")
	_, sent, err := search(t, answer)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(sent.Body)
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	page := map[string]any{"size": float64(aFew), "from": float64(0)}
	got := []any{sent.Method, sent.URL.String(),
		sent.Header.Get("Content-Type"), sent.Header.Get("User-Agent")}
	want := []any{http.MethodPost, endpoint, "application/json", agent}
	if !slices.Equal(got, want) || body["q"] != query ||
		len(body) != bodyKeys || fmt.Sprint(body["page"]) !=
		fmt.Sprint(page) {
		t.Errorf("%q, want %q; body %s", got, want, raw)
	}
}

// A search gives how many citations NTRS says match, the citations it
// gave in its order, each with its id, year, first author by sequence,
// title and PDF.
//
// Anti-test: the first author is the one of sequence 0, not the first
// listed; the second citation has no PDF though the first has; and the
// PDF is the PDF, not the text download before it.
func TestASearchGivesWhatNTRSFound(t *testing.T) {
	answer, _ := reply(t, http.StatusOK, "two-citations.json", "")
	found, _, err := search(t, answer)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, cited := range found.Citations() {
		lines = append(lines, described(t, cited))
	}
	want := []string{"20990000001|2099|Quill, Ann|A crafted report " +
		"about nothing|true|" + pdfAt,
		"20980000002|2098|Fen, Dee|A second crafted report|false|"}
	if found.Count() != total || !slices.Equal(lines, want) {
		t.Errorf("%d, %q", found.Count(), lines)
	}
}

// A citation with no title, no named author, a date that is no year, or
// no date, has none of them, and a PDF given by its original link only
// is found; one with no link at all is passed over for the next.
//
// Anti-test: a citation with all of them has them (the test above).
func TestACitationWithoutThingsHasNone(t *testing.T) {
	answer, _ := reply(t, http.StatusOK, "bare-citation.json", "")
	found, _, err := search(t, answer)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, cited := range found.Citations() {
		lines = append(lines, described(t, cited))
	}
	want := []string{"20970000003||||true|https://ntrs.nasa.gov/api/" +
		"citations/20970000003/downloads/x.pdf",
		"20960000004|||A crafted report with no date|false|",
		"20950000005|||A crafted report with a short date|false|"}
	if !slices.Equal(lines, want) {
		t.Errorf("%q, want %q", lines, want)
	}
}

// An answer that found nothing is no citations and a count of none.
//
// Anti-test: an answer that found citations does not have a count of
// none.
func TestAnAnswerThatFoundNothingHasNoCitations(t *testing.T) {
	answer, _ := reply(t, http.StatusOK, "none-found.json", "")
	found, _, err := search(t, answer)
	if err != nil || found.Count() != noCitations ||
		len(found.Citations()) != noCitations {
		t.Errorf("%v, %v", found, err)
	}
	full, _ := reply(t, http.StatusOK, "two-citations.json", "")
	some, _, _ := search(t, full)
	if some.Count() == noCitations {
		t.Errorf("a count of none")
	}
}

// The citations a search gives are a copy: changing them does not change
// what the search found.
//
// Anti-test: what is given again is as it was.
func TestWhatASearchGivesIsACopy(t *testing.T) {
	answer, _ := reply(t, http.StatusOK, "two-citations.json", "")
	found, _, err := search(t, answer)
	if err != nil {
		t.Fatal(err)
	}
	cited := found.Citations()
	cited[noCitations] = ntrs.Citation{}
	if found.Citations()[noCitations].ID() == "0" {
		t.Errorf("changed")
	}
}

// An answer that is not JSON, or is JSON without results, is an error
// that says so, and finds nothing.
//
// Anti-test: an answer that found nothing is not one without results.
func TestAnAnswerThatIsNotAnAnswerIsAnError(t *testing.T) {
	for file, says := range map[string]string{
		"not-json.txt":    "not JSON",
		"no-results.json": "no results",
	} {
		answer, _ := reply(t, http.StatusOK, file, "")
		found, _, err := search(t, answer)
		if found != nil || err == nil ||
			!strings.Contains(err.Error(), says) {
			t.Errorf("%s: %v, %v", file, found, err)
		}
	}
}

// A search with no query, or for fewer citations than one or more than
// NTRS gives, is refused before anything is sent: its mock Doer expects
// no call.
//
// Anti-test: the fewest and the most are not refused.
func TestASearchRefusesWhatItCannotAsk(t *testing.T) {
	for _, each := range []struct {
		words string
		rows  int
	}{{"", aFew}, {query, noCitations}, {query, justOver}} {
		doer := mocks.NewMockDoer(gomock.NewController(t))
		found, err := ntrs.Search(context.Background(), doer,
			each.words, each.rows, agent)
		if found != nil || err == nil {
			t.Errorf("%q, %d: %v, %v", each.words, each.rows, found,
				err)
		}
	}
	for _, rows := range []int{oneCitation, theMost} {
		answer, _ := reply(t, http.StatusOK, "none-found.json", "")
		doer, _ := doerAnswering(gomock.NewController(t), answer)
		if _, err := ntrs.Search(context.Background(), doer, query,
			rows, agent); err != nil {
			t.Errorf("%d: %v", rows, err)
		}
	}
}

// A 429 is NTRS asking us to stop: a *Stopped, with the wait it asked
// as it wrote it, or none. A 404 wraps ErrNotFound. Any other status is
// an error that names it. Each body is closed once.
//
// Anti-test: none of the others is NTRS asking us to stop, and a 429 is
// not a not-found.
func TestEachStatusIsWhatItIs(t *testing.T) {
	for _, each := range []struct {
		code  int
		retry string
		says  string
	}{{http.StatusTooManyRequests, "90", "wait 90"},
		{http.StatusTooManyRequests, "", "gave no time"},
		{http.StatusNotFound, "", "no such page"},
		{http.StatusInternalServerError, "", "status 500"}} {
		answer, body := reply(t, each.code, "", "")
		answer.Header.Set("Retry-After", each.retry)
		found, _, err := search(t, answer)
		var stopped *ntrs.Stopped
		stop := errors.As(err, &stopped)
		if found != nil || err == nil ||
			!strings.Contains(err.Error(), each.says) ||
			stop != (each.code == http.StatusTooManyRequests) ||
			errors.Is(err, ntrs.ErrNotFound) !=
				(each.code == http.StatusNotFound) ||
			stop && stopped.Wait() != each.retry ||
			body.closed != oneCitation {
			t.Errorf("%d: %v, %d closes", each.code, err, body.closed)
		}
	}
}

// A Doer that fails is an error that wraps the Doer's own, and finds
// nothing. A missing context is an error, and sends nothing.
//
// Anti-test: neither is NTRS asking us to stop.
func TestADoerThatFailsOrNoContextIsAnError(t *testing.T) {
	broke := errors.New("the line went dead")
	doer := mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).Return(nil, broke)
	found, err := ntrs.Search(context.Background(), doer, query, aFew,
		agent)
	if found != nil || !errors.Is(err, broke) || askedToStop(err) {
		t.Errorf("%v, %v", found, err)
	}
	var none context.Context
	doer = mocks.NewMockDoer(gomock.NewController(t))
	found, err = ntrs.Search(none, doer, query, aFew, agent)
	if found != nil || err == nil || askedToStop(err) {
		t.Errorf("%v, %v", found, err)
	}
	cited, err := ntrs.Lookup(none, doer, "20990000001", agent)
	if cited != nil || err == nil || askedToStop(err) {
		t.Errorf("%v, %v", cited, err)
	}
}

// A lookup is one GET of the citation's own address, which gives the
// citation, and the address of its PDF, which is not named by its id.
//
// Anti-test: the citation looked up is the one that was asked for, and
// no body is sent.
func TestALookupAsksForOneCitation(t *testing.T) {
	answer, _ := reply(t, http.StatusOK, "citation.json", "")
	cited, sent, err := lookup(t, answer)
	if err != nil {
		t.Fatal(err)
	}
	want := "20990000001|2099|Quill, Ann|A crafted report about " +
		"nothing|true|" + crafted + "/downloads/report.pdf"
	if sent.Method != http.MethodGet || sent.URL.String() != crafted ||
		sent.Body != nil || sent.Header.Get("User-Agent") != agent ||
		described(t, *cited) != want {
		t.Errorf("%s %s: %s", sent.Method, sent.URL, described(t, *cited))
	}
}

// A citation that holds no download has no PDF, and an empty address
// with no error.
//
// Anti-test: a citation that holds one has it.
func TestACitationWithNoPDFHasNone(t *testing.T) {
	answer, _ := reply(t, http.StatusOK, "citation-no-pdf.json", "")
	cited, _, err := lookup(t, answer)
	address, refused := cited.PDF()
	if err != nil || cited.HasPDF() || address != "" || refused != nil {
		t.Errorf("%v, %q, %v", err, address, refused)
	}
}

// A PDF address that is not a file of the citation's own folder is
// refused when it is asked for, and never given.
//
// Anti-test: the citation is still found, and says it has a PDF.
func TestAPDFOfAnotherCitationIsRefused(t *testing.T) {
	answer, _ := reply(t, http.StatusOK, "citation-elsewhere.json", "")
	cited, _, err := lookup(t, answer)
	address, refused := cited.PDF()
	if err != nil || !cited.HasPDF() || address != "" ||
		!errors.Is(refused, celestialdata.Refused()) {
		t.Errorf("%v, %q, %v", err, address, refused)
	}
}

// Each PDF address that is not one plain file in the citation's folder
// of downloads is refused.
//
// Anti-test: one that is, is not.
func TestOnlyAPlainFileOfItsOwnFolderIsAPDF(t *testing.T) {
	folder := "/api/citations/20990000001/downloads/"
	for _, bad := range []string{"https://elsewhere.example/a.pdf",
		folder, folder + "..", folder + ".", folder + "a/b.pdf",
		folder + "a.pdf?x=1", folder + "a.pdf#x",
		"/api/citations/20990000001/downloadsa.pdf"} {
		text := `{"id": 20990000001, "downloads": [{"mimetype": ` +
			`"application/pdf", "links": {"pdf": "` + bad + `"}}]}`
		answer, _ := reply(t, http.StatusOK, "", text)
		cited, _, err := lookup(t, answer)
		if _, refused := cited.PDF(); err != nil || refused == nil {
			t.Errorf("%q: %v, %v", bad, err, refused)
		}
	}
	good, _ := reply(t, http.StatusOK, "", `{"id": 20990000001, `+
		`"downloads": [{"mimetype": "application/pdf", "links": `+
		`{"pdf": "`+folder+`a b.pdf"}}]}`)
	cited, _, _ := lookup(t, good)
	if _, refused := cited.PDF(); refused != nil {
		t.Errorf("refused: %v", refused)
	}
}

// A lookup of a citation NTRS does not have is ErrNotFound, one that is
// not JSON is an error, and neither is a citation.
//
// Anti-test: a 429 to a lookup is NTRS asking us to stop.
func TestALookupThatFailsGivesNoCitation(t *testing.T) {
	gone, _ := reply(t, http.StatusNotFound, "", "")
	cited, _, err := lookup(t, gone)
	if cited != nil || !errors.Is(err, ntrs.ErrNotFound) {
		t.Errorf("%v, %v", cited, err)
	}
	bad, _ := reply(t, http.StatusOK, "not-json.txt", "")
	cited, _, err = lookup(t, bad)
	if cited != nil || err == nil || askedToStop(err) ||
		!strings.Contains(err.Error(), "not JSON") {
		t.Errorf("%v, %v", cited, err)
	}
	stop, _ := reply(t, http.StatusTooManyRequests, "", "")
	_, _, err = lookup(t, stop)
	if !askedToStop(err) {
		t.Errorf("%v", err)
	}
}

// A lookup of what is not an id sends nothing.
//
// Anti-test: an id is digits, and sends.
func TestALookupOfWhatIsNotAnIDSendsNothing(t *testing.T) {
	doer := mocks.NewMockDoer(gomock.NewController(t))
	for _, bad := range []string{"", "2099x", "../1", "1 2", "-1",
		"٣", "1/downloads"} {
		if _, err := ntrs.Lookup(context.Background(), doer, bad,
			agent); !errors.Is(err, celestialdata.Refused()) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	answer, _ := reply(t, http.StatusOK, "citation.json", "")
	if cited, _, err := lookup(t, answer); err != nil || cited == nil {
		t.Errorf("%v, %v", cited, err)
	}
}

// ParseID gives back digits and refuses all else.
//
// Anti-test: a refusal is a refusal of this package's kind.
func TestParseIDTakesDigitsOnly(t *testing.T) {
	id, err := ntrs.ParseID("20120013496")
	if id != "20120013496" || err != nil {
		t.Errorf("%q, %v", id, err)
	}
	_, err = ntrs.ParseID("2012001349a")
	if !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "2012001349a") {
		t.Errorf("%v", err)
	}
}

// NTRS is an archive asked by name only, that holds nothing near a
// place.
//
// Anti-test: it is not one of the archives asked with no name.
func TestNTRSIsAnArchiveByNameThatHoldsNothingNearAPlace(t *testing.T) {
	archive := ntrs.New()
	found, err := archive.Find(nil)
	var asNamed celestialdata.ByName = archive
	if archive.Name() != "ntrs" || found != nil || err != nil ||
		!asNamed.ByNameOnly() {
		t.Errorf("%q, %v, %v", archive.Name(), found, err)
	}
}

// Stopped says how long NTRS asked, when it did.
func TestStoppedSaysTheWaitWhenThereIsOne(t *testing.T) {
	answer, _ := reply(t, http.StatusTooManyRequests, "", "")
	answer.Header.Set("Retry-After", "30")
	_, _, err := search(t, answer)
	if !strings.Contains(err.Error(), "wait 30") {
		t.Errorf("%v", err)
	}
}

// ids are digits, and what comes back as one is as it was asked.
func idsAreDigits(rt *rapid.T) {
	id := rapid.StringMatching(`[0-9]{1,20}`).Draw(rt, "id")
	got, err := ntrs.ParseID(id)
	if err != nil || got != id {
		rt.Fatalf("%q: %q, %v", id, got, err)
	}
	notOne := rapid.StringMatching(`[0-9]*[^0-9]+[0-9]*`).Draw(rt, "not")
	if _, err := ntrs.ParseID(notOne); err == nil {
		rt.Fatalf("%q was taken for an id", notOne)
	}
}

// Any run of digits is an id, and anything with a character that is not
// a digit is not.
func TestAnyDigitsAreAnIDAndNothingElseIs(t *testing.T) {
	rapid.Check(t, idsAreDigits)
}

// queryAndRows is the property: a query and a count of rows go into the
// request's body as they were.
func queryAndRows(rt *rapid.T) {
	asking := rapid.StringN(oneRune, unbounded, unbounded).Draw(rt,
		"query")
	rows := rapid.IntRange(oneCitation, theMost).Draw(rt, "rows")
	answer := &http.Response{StatusCode: http.StatusOK,
		Header: http.Header{},
		Body:   io.NopCloser(strings.NewReader(`{"results": []}`))}
	ctrl := gomock.NewController(rt)
	doer, kept := doerAnswering(ctrl, answer)
	_, err := ntrs.Search(context.Background(), doer, asking, rows,
		agent)
	var body struct {
		Q    string
		Page struct{ Size int }
	}
	raw, _ := io.ReadAll(kept.sent.Body)
	if err != nil || json.Unmarshal(raw, &body) != nil ||
		body.Q != asking || body.Page.Size != rows {
		rt.Fatalf("%q, %d: %v", asking, rows, err)
	}
	ctrl.Finish()
}

// Any query and any number of rows survive into the request as they
// were.
//
// Anti-test: the query is kept apart from the rest whatever it holds,
// such as quotes and braces.
func TestAnyQueryAndRowsSurviveIntoTheRequest(t *testing.T) {
	rapid.Check(t, queryAndRows)
}
