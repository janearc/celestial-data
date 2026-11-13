package ads_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/janearc/celestial-data/ads"
	"github.com/janearc/celestial-data/mocks"
	"go.uber.org/mock/gomock"
	"pgregory.net/rapid"
)

// The token, agent and query these tests search with. They're defined:
// any will do. The token is made up, and plain enough to find in a line.
const (
	token = "tok-4f9c2a-not-a-real-token"
	agent = "celestial-data-tests"
	query = `author:"Quill, A." year:2099`
	// fields are the fields of a paper a search asks ADS for, as its
	// documentation names them.
	fields = "bibcode,title,year,author,identifier"
)

// The counts these tests expect. The papers and authors are counted
// from the fixtures in testdata, which are crafted.
const (
	noPapers  = 0       // defined: none
	onePaper  = 1       // defined: one
	numFound  = 12      // defined: numFound in two-papers.json
	aFew      = 5       // defined: a few rows asked for
	noAuthors = 0       // defined: none
	allThree  = 3       // defined: the first paper's authors there
	params    = 4       // defined: ADS's q, fl, rows and sort
	aMillion  = 1000000 // defined: the most rows the property asks for
	oneRune   = 1       // defined: the least a query holds
	unbounded = -1      // defined: rapid's word for no bound
)

// remaining is the X-RateLimit-Remaining the answers here give. It's
// defined: any count will do.
const remaining = "4321"

// reset is the X-RateLimit-Reset the 429 answers here give. It's
// defined: any time will do.
const reset = "2026-11-07T00:00:00Z"

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
func search(t *testing.T, answer *http.Response) (*ads.Found,
	*http.Request, error) {
	t.Helper()
	doer, kept := doerAnswering(gomock.NewController(t), answer)
	found, err := ads.Search(context.Background(), doer, token, query,
		aFew, agent)
	return found, kept.sent, err
}

// asksNothing is a search that must send nothing: its mock Doer expects
// no call, so a request fails the test.
func asksNothing(t *testing.T, key, words string, most int) (*ads.Found,
	error) {
	t.Helper()
	doer := mocks.NewMockDoer(gomock.NewController(t))
	return ads.Search(context.Background(), doer, key, words, most, agent)
}

// endpoint is ADS's search endpoint, as its developer documentation
// gives it.
const endpoint = "https://api.adsabs.harvard.edu/v1/search/query"

// askedToStop is whether an error is ADS asking us to stop.
func askedToStop(err error) bool {
	var stopped *ads.Stopped
	return errors.As(err, &stopped)
}

// A search is sent to api.adsabs.harvard.edu, over https: the host ADS
// keeps for its API.
//
// Anti-test: a plain http address, or another host, does not pass.
func TestTheAPIIsTheHostADSKeepsForItsAPI(t *testing.T) {
	answer, _ := reply(t, http.StatusOK, "two-papers.json", "")
	_, sent, err := search(t, answer)
	if err != nil || sent.URL.Scheme != "https" ||
		sent.URL.Host != "api.adsabs.harvard.edu" {
		t.Errorf("asked %s, %v", sent.URL, err)
	}
}

// A search is one GET to the API with the query, the fields, the rows
// and the sort in its parameters, the token as a bearer token, and the
// agent as the user agent.
//
// Anti-test: no other parameter, and no token in the address.
func TestASearchAsksTheAPIAsADSDocumentsIt(t *testing.T) {
	answer, _ := reply(t, http.StatusOK, "two-papers.json", "")
	_, sent, err := search(t, answer)
	asked := sent.URL.Query()
	got := []string{sent.Method, sent.URL.Scheme + "://" + sent.URL.Host +
		sent.URL.Path, asked.Get("q"), asked.Get("fl"),
		asked.Get("rows"), asked.Get("sort"),
		sent.Header.Get("Authorization"), sent.Header.Get("User-Agent")}
	want := []string{http.MethodGet, endpoint, query,
		fields, strconv.Itoa(aFew), "date desc",
		"Bearer " + token, agent}
	if err != nil || !slices.Equal(got, want) || len(asked) != params ||
		strings.Contains(sent.URL.String(), token) {
		t.Errorf("%q, want %q, %v", got, want, err)
	}
}

// A search gives how many papers ADS says match, the papers it gave in
// its order, each with its bibcode, year, first title, authors and
// arXiv id, and how much of the limit ADS says is left.
//
// Anti-test: the second paper's title is the first of its two, its one
// author is its own, not the first paper's, and it has no arXiv id; the
// first's id is the one ADS marks arXiv, not its arXiv-shaped bibcode.
func TestASearchGivesWhatADSFound(t *testing.T) {
	answer, _ := reply(t, http.StatusOK, "two-papers.json", "")
	answer.Header.Set("X-RateLimit-Remaining", remaining)
	found, _, err := search(t, answer)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, paper := range found.Papers() {
		lines = append(lines, described(paper))
	}
	want := []string{"2099ApJ...999..111A 2099 A crafted paper about " +
		"nothing: Quill, A.; Marsh, B.; Thorn, C. [9909.00001]",
		"2098MNRAS.888..222B 2098 A second crafted paper: Fen, D. []"}
	if found.Count() != numFound || !slices.Equal(lines, want) ||
		found.Remaining() != remaining {
		t.Errorf("%d, %q, %q", found.Count(), lines, found.Remaining())
	}
}

// described is a paper's every field on a line: its bibcode, year,
// title, authors, and its arXiv id in brackets.
func described(paper ads.Paper) string {
	return paper.Bibcode() + " " + paper.Year() + " " + paper.Title() +
		": " + strings.Join(paper.Authors(), "; ") + " [" +
		paper.ArXiv() + "]"
}

// A paper ADS gives with no title, no author and no identifiers has
// none of them, and no arXiv id though its bibcode looks like one; the
// rest of it as given.
//
// Anti-test: a paper with all three has all three (the test above).
func TestAPaperWithNoTitleOrAuthorHasNone(t *testing.T) {
	answer, _ := reply(t, http.StatusOK, "bare-paper.json", "")
	found, _, err := search(t, answer)
	if err != nil {
		t.Fatal(err)
	}
	for _, bare := range found.Papers() {
		if described(bare) != "2097arXiv....7777C 2097 :  []" ||
			bare.Title() != "" || len(bare.Authors()) != noAuthors ||
			found.Remaining() != "" {
			t.Errorf("%q, %q", described(bare), found.Remaining())
		}
	}
}

// An answer that found nothing is no papers and a count of none.
//
// Anti-test: an answer that found papers does not have a count of none.
func TestAnAnswerThatFoundNothingHasNoPapers(t *testing.T) {
	answer, _ := reply(t, http.StatusOK, "none-found.json", "")
	found, _, err := search(t, answer)
	if err != nil || found.Count() != noPapers ||
		len(found.Papers()) != noPapers {
		t.Errorf("%v, %v", found, err)
	}
	full, _ := reply(t, http.StatusOK, "two-papers.json", "")
	some, _, _ := search(t, full)
	if some.Count() == noPapers {
		t.Errorf("a count of none")
	}
}

// The papers and authors a search gives are copies: changing them does
// not change what the search found.
//
// Anti-test: what is given again is as it was.
func TestWhatASearchGivesIsACopy(t *testing.T) {
	answer, _ := reply(t, http.StatusOK, "two-papers.json", "")
	found, _, err := search(t, answer)
	if err != nil {
		t.Fatal(err)
	}
	papers := found.Papers()
	authors := papers[noPapers].Authors()
	papers[noPapers] = ads.Paper{}
	authors[noPapers] = "changed"
	again := found.Papers()[noPapers]
	if again.Bibcode() == "" || again.Authors()[noPapers] == "changed" ||
		len(again.Authors()) != allThree {
		t.Errorf("%q, %q", again.Bibcode(), again.Authors())
	}
}

// A search with no token, no query or fewer than one row asks nothing:
// the mock Doer expects no call.
//
// Anti-test: a search with all three is sent (every test above).
func TestASearchMissingWhatItNeedsAsksNothing(t *testing.T) {
	for name, miss := range map[string]struct {
		key, words string
		most       int
	}{
		"token":    {"", query, aFew},
		"query":    {token, "", aFew},
		"rows":     {token, query, noPapers},
		"negative": {token, query, -aFew},
	} {
		found, err := asksNothing(t, miss.key, miss.words, miss.most)
		if found != nil || err == nil || !strings.HasPrefix(err.Error(),
			"ads: a search ") {
			t.Errorf("%s: %v, %v", name, found, err)
		}
	}
}

// A search with one row, the fewest, is sent.
//
// Anti-test: a search with none is not (the test above).
func TestASearchOfOneRowIsSent(t *testing.T) {
	answer, _ := reply(t, http.StatusOK, "none-found.json", "")
	doer, kept := doerAnswering(gomock.NewController(t), answer)
	_, err := ads.Search(context.Background(), doer, token, query,
		onePaper, agent)
	if err != nil || kept.sent.URL.Query().Get("rows") !=
		strconv.Itoa(onePaper) {
		t.Errorf("%v", err)
	}
}

// A 401 is ADS refusing the token, and says so.
//
// Anti-test: it is not ADS asking us to stop.
func TestA401IsADSRefusingTheToken(t *testing.T) {
	answer, _ := reply(t, http.StatusUnauthorized, "", "no")
	found, _, err := search(t, answer)
	if found != nil || err == nil ||
		err.Error() != "ads: ADS refused the token" ||
		askedToStop(err) {
		t.Errorf("%v, %v", found, err)
	}
}

// A 429 is ADS asking us to stop until the time it gave, which is kept
// as written.
//
// Anti-test: a 429 without the header says ADS gave no time, and the
// reset it gives is empty.
func TestA429IsADSAskingUsToStop(t *testing.T) {
	answer, _ := reply(t, http.StatusTooManyRequests, "", "")
	answer.Header.Set("X-RateLimit-Reset", reset)
	_, _, err := search(t, answer)
	var stopped *ads.Stopped
	if !errors.As(err, &stopped) || !askedToStop(err) ||
		stopped.Reset() != reset ||
		err.Error() != "ads: ADS asked us to stop until "+reset {
		t.Fatalf("%v", err)
	}
	unsaid, _ := reply(t, http.StatusTooManyRequests, "", "")
	_, _, err = search(t, unsaid)
	if !errors.As(err, &stopped) || !askedToStop(err) ||
		stopped.Reset() != "" || err.Error() !=
		"ads: ADS asked us to stop and gave no time" {
		t.Errorf("%v", err)
	}
}

// An error that wraps ADS asking us to stop is known as it.
//
// Anti-test: no error, and errors that are not ADS asking, are not.
func TestOnlyADSAskingUsToStopIsAskedToStop(t *testing.T) {
	wrapped := fmt.Errorf("while searching: %w", &ads.Stopped{})
	if !askedToStop(wrapped) {
		t.Errorf("%v is not known", wrapped)
	}
	if askedToStop(errors.New("another error")) ||
		askedToStop(nil) {
		t.Errorf("an error that is not ADS asking is known")
	}
}

// Any other status is an error that names it, and is not ADS asking us
// to stop.
//
// Anti-test: the status is in the error, not the answer's body.
func TestAnyOtherStatusIsAnErrorNamingIt(t *testing.T) {
	for _, code := range []int{http.StatusForbidden,
		http.StatusInternalServerError, http.StatusNoContent} {
		answer, _ := reply(t, code, "", "a body that is not named")
		_, _, err := search(t, answer)
		want := "ads: ADS answered with status " + strconv.Itoa(code)
		if err == nil || err.Error() != want || askedToStop(err) {
			t.Errorf("%d: %v", code, err)
		}
	}
}

// An answer that is not JSON, or has no response, is an error, and
// finds nothing.
//
// Anti-test: an answer of JSON with a response that found nothing is
// not.
func TestAnAnswerNotInADSsShapeIsAnError(t *testing.T) {
	for file, want := range map[string]string{
		"not-json.txt":     "ads: the answer is not JSON: ",
		"no-response.json": "ads: the answer has no response",
	} {
		answer, _ := reply(t, http.StatusOK, file, "")
		found, _, err := search(t, answer)
		if found != nil || err == nil ||
			!strings.HasPrefix(err.Error(), want) {
			t.Errorf("%s: %v, %v", file, found, err)
		}
	}
	answer, _ := reply(t, http.StatusOK, "none-found.json", "")
	if _, _, err := search(t, answer); err != nil {
		t.Errorf("%v", err)
	}
}

// The body of every answer is closed once, whatever the status.
//
// Anti-test: the count is one, not none and not two.
func TestEveryAnswersBodyIsClosedOnce(t *testing.T) {
	for _, code := range []int{http.StatusOK, http.StatusUnauthorized,
		http.StatusTooManyRequests, http.StatusInternalServerError} {
		answer, body := reply(t, code, "", "{}")
		search(t, answer)
		if body.closed != onePaper {
			t.Errorf("%d: closed %d times", code, body.closed)
		}
	}
}

// A Doer that fails is an error that says the Doer's own words, and
// finds nothing. A missing context is an error, and sends nothing.
//
// Anti-test: neither is ADS asking us to stop.
func TestADoerThatFailsOrNoContextIsAnError(t *testing.T) {
	broke := errors.New("the line went dead")
	doer := mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).Return(nil, broke)
	found, err := ads.Search(context.Background(), doer, token, query,
		aFew, agent)
	if found != nil || err == nil ||
		!strings.Contains(err.Error(), broke.Error()) || askedToStop(err) {
		t.Errorf("%v, %v", found, err)
	}
	var none context.Context
	doer = mocks.NewMockDoer(gomock.NewController(t))
	found, err = ads.Search(none, doer, token, query, aFew, agent)
	if found != nil || err == nil || askedToStop(err) {
		t.Errorf("%v, %v", found, err)
	}
}

// Anti-test: the token is in no error by the two ways round the body:
// a Doer whose own error holds it, where it was wrapped and said whole,
// and a reset header that holds it, where it was said as ADS wrote it.
// What cannot be printed in a header is left out.
//
// A search the caller stopped is still found as stopped by errors.Is,
// though the Doer's error is no longer wrapped.
func TestTheTokenIsInNoErrorRoundTheBody(t *testing.T) {
	doer := mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).Return(nil,
		errors.New("failed with "+token))
	_, err := ads.Search(context.Background(), doer, token, query, aFew,
		agent)
	if err == nil || strings.Contains(err.Error(), token) ||
		!strings.Contains(err.Error(), "failed with [the token]") {
		t.Errorf("a Doer's error said %v", err)
	}
	answer := &http.Response{StatusCode: http.StatusTooManyRequests,
		Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}
	answer.Header.Set("X-RateLimit-Reset", "soon\x1b[2J"+token)
	doer = mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).Return(answer, nil)
	_, err = ads.Search(context.Background(), doer, token, query, aFew,
		agent)
	if !askedToStop(err) || strings.Contains(err.Error(), token) ||
		!strings.Contains(err.Error(), "soon[2J[the token]") {
		t.Errorf("a reset header said %q: the escape left out and "+
			"nothing in its place", err)
	}
	doer = mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).Return(nil,
		fmt.Errorf("the dial: %w", context.Canceled))
	_, err = ads.Search(context.Background(), doer, token, query, aFew,
		agent)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("a stopped search said %v", err)
	}
}

// heldNowhere fails the test when an error, or any error its chain
// unwraps to, holds the token.
func heldNowhere(t *testing.T, err error, how string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s gave no error", how)
	}
	for each := err; each != nil; each = errors.Unwrap(each) {
		if strings.Contains(each.Error(), token) {
			t.Errorf("%s said %q", how, each)
		}
	}
}

// Anti-test: the token is in no error by two more ways. A body whose
// read fails, with the token in the reader's own error, where the read's
// error was wrapped whole; and a token split by a character that cannot
// be printed, where it was taken out first and so came back whole when
// the character was left out.
func TestTheTokenIsInNoErrorOfTheBodyOrSplit(t *testing.T) {
	broken := &http.Response{StatusCode: http.StatusOK,
		Header: http.Header{}, Body: io.NopCloser(iotest.ErrReader(
			errors.New("failed with " + token)))}
	doer := mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).Return(broken, nil)
	_, err := ads.Search(context.Background(), doer, token, query, aFew,
		agent)
	heldNowhere(t, err, "a body that would not read")
	half := len(token) / halves
	split := token[:half] + "\n" + token[half:]
	doer = mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).Return(nil,
		errors.New("failed with "+split))
	_, err = ads.Search(context.Background(), doer, token, query, aFew,
		agent)
	heldNowhere(t, err, "a token split by a newline")
}

// halves is how many parts the split token is cut into. It's defined.
const halves = 2

// No error the package makes holds the token, though ADS's own words
// might: each kind of error is searched for it.
//
// Anti-test: the errors are all there, each different, so the search is
// not of nothing; and the token is in what ADS said.
func TestNoErrorHoldsTheToken(t *testing.T) {
	echoed := "you said " + token
	var errs []error
	for _, code := range []int{http.StatusUnauthorized,
		http.StatusTooManyRequests, http.StatusForbidden} {
		answer, _ := reply(t, code, "", echoed)
		_, _, err := search(t, answer)
		errs = append(errs, err)
	}
	for _, text := range []string{echoed, "{}"} {
		answer, _ := reply(t, http.StatusOK, "", text)
		_, _, err := search(t, answer)
		errs = append(errs, err)
	}
	_, noToken := asksNothing(t, "", query, aFew)
	_, noQuery := asksNothing(t, token, "", aFew)
	_, noRows := asksNothing(t, token, query, noPapers)
	errs = append(errs, noToken, noQuery, noRows)
	seen := map[string]bool{}
	for _, err := range errs {
		if err == nil || strings.Contains(err.Error(), token) {
			t.Fatalf("%v", err)
		}
		seen[err.Error()] = true
	}
	if len(seen) != len(errs) || !strings.Contains(echoed, token) {
		t.Errorf("%d different errors of %d", len(seen), len(errs))
	}
}

// queryAndRows is the property: a query and a count of rows go into the
// request as they were, and the token into the header.
func queryAndRows(rt *rapid.T) {
	asking := rapid.StringN(oneRune, unbounded, unbounded).Draw(rt,
		"query")
	rows := rapid.IntRange(onePaper, aMillion).Draw(rt, "rows")
	answer := &http.Response{StatusCode: http.StatusOK,
		Header: http.Header{},
		Body:   io.NopCloser(strings.NewReader(`{"response": {}}`))}
	ctrl := gomock.NewController(rt)
	doer, kept := doerAnswering(ctrl, answer)
	_, err := ads.Search(context.Background(), doer, token, asking, rows,
		agent)
	asked := kept.sent.URL.Query()
	if err != nil || asked.Get("q") != asking ||
		asked.Get("rows") != strconv.Itoa(rows) ||
		asked.Get("fl") != fields ||
		kept.sent.Header.Get("Authorization") != "Bearer "+token {
		rt.Fatalf("%q, %d: %v", asking, rows, err)
	}
	ctrl.Finish()
}

// Any query and any number of rows survive into the request as they
// were.
//
// Anti-test: the query is kept apart from the other parameters whatever
// it holds, such as ampersands and equals signs.
func TestAnyQueryAndRowsSurviveIntoTheRequest(t *testing.T) {
	rapid.Check(t, queryAndRows)
}

// The longest reset kept, and how far past it the long one here runs.
const (
	mostHeard = 256 // defined: the most characters ads keeps of a reset
	pastMost  = 44  // defined: any number past it will do
)

// twoBytes is a character UTF-8 writes in two bytes.
const twoBytes = "é"

// A reset longer than ADS would write is kept to its first 256
// characters, counted as characters, not bytes: each here is two bytes.
//
// Anti-test: a reset of exactly 256 characters is kept whole.
func TestALongResetIsKeptToItsFirstCharacters(t *testing.T) {
	for _, length := range []int{mostHeard, mostHeard + pastMost} {
		answer, _ := reply(t, http.StatusTooManyRequests, "", "")
		long := strings.Repeat(twoBytes, length)
		answer.Header.Set("X-RateLimit-Reset", long)
		_, _, err := search(t, answer)
		var stopped *ads.Stopped
		if !errors.As(err, &stopped) {
			t.Fatalf("a reset of %d said %v", length, err)
		}
		want := strings.Repeat(twoBytes, min(length, mostHeard))
		if stopped.Reset() != want {
			t.Errorf("a reset of %d characters kept %d bytes",
				length, len(stopped.Reset()))
		}
	}
}
