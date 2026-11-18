package dasch_test

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"go.uber.org/mock/gomock"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/dasch"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
)

// The star the recorded light curve is of, and the circle it is asked in.
// The star is Gaia DR3 4081136684118965760; DASCH answered with the light
// curve on 2026-11-08. All are defined by that answer.
const (
	starRA     = 287.50086              // defined: the star's right ascension, degrees
	starDec    = -22.23481              // defined: its declination
	starWide   = 0.0014                 // defined: the circle's radius, degrees
	starNumber = "419100022221405"      // defined: its APASS ref_number
	starBin    = "52510421"             // defined: its gsc_bin_index
	agent      = "celestial-data-tests" // defined: any agent will do
	curveName  = "dasch-dr7-apass-419100022221405-lightcurve.csv"
	curveURL   = "https://api.starglass.cfa.harvard.edu/public/dasch/" +
		"dr7/lightcurve?gsc_bin_index=52510421&ref_number=" +
		"419100022221405&refcat=apass"
	nearer  = "dasch-dr7-apass-419100022221405-lightcurve.csv"
	farther = "dasch-dr7-apass-419100022221777-lightcurve.csv"
)

// The counts the tests expect. All are defined by the fixtures.
const (
	noFiles     = 0  // defined: none
	twoSources  = 2  // defined: querycat.json has three, one beyond the circle
	oneSource   = 1  // defined: the least a search names
	curveLines  = 3  // defined: the lines of lightcurve.json
	threeFields = 3  // defined: a light curve's body: refcat and two numbers
	first       = 0  // defined: the first of a list
	second      = 1  // defined: the second of a list
	unsaid      = -1 // defined: a length no one has asked for
)

// star is the recorded circle, or the test stops.
func star(t *testing.T, radius float64) *celestialdata.Near {
	t.Helper()
	centre, err := celestialdata.NewPosition(starRA, starDec)
	if err != nil {
		t.Fatal(err)
	}
	near, err := celestialdata.NewNear(centre, radius)
	if err != nil {
		t.Fatal(err)
	}
	return near
}

// recorded is the text of a file in testdata, or the test stops.
func recorded(t *testing.T, file string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// reply is an answer of a status with a body. closed counts how many
// times the body was closed.
type reply struct {
	response *http.Response
	closed   *int
}

// closing is a body that counts its closes.
type closing struct {
	io.Reader
	closed *int
}

// Close counts a close.
func (body closing) Close() error {
	*body.closed++
	return nil
}

// answer is an answer of a status with a body, and headers as pairs.
func answer(code int, body string, headers ...string) reply {
	closed := new(int)
	response := &http.Response{StatusCode: code, Header: http.Header{},
		Body: closing{Reader: strings.NewReader(body), closed: closed}}
	for at := 0; at < len(headers); at += 2 {
		response.Header.Set(headers[at], headers[at+1])
	}
	return reply{response: response, closed: closed}
}

// sent is a request as it was sent: the request, with its body read.
type sent struct {
	*http.Request
	text string
}

// doerAnswering is a mock Doer that answers each request in turn with
// the next reply, and the requests it was sent. It expects as many
// requests as it has replies.
func doerAnswering(t *testing.T, replies ...reply) (*mocks.MockDoer,
	*[]sent) {
	t.Helper()
	var requests []sent
	doer := mocks.NewMockDoer(gomock.NewController(t))
	for _, each := range replies {
		doer.EXPECT().Do(gomock.Any()).DoAndReturn(
			func(request *http.Request) (*http.Response, error) {
				raw, err := io.ReadAll(request.Body)
				if err != nil {
					t.Error(err)
				}
				requests = append(requests, sent{request, string(raw)})
				return each.response, nil
			})
	}
	return doer, &requests
}

// archive is the archive asked through a doer, naming at most so many
// sources, or the test stops.
func archive(t *testing.T, doer dasch.Doer, most int) *dasch.Archive {
	t.Helper()
	found, err := dasch.New(doer, agent, most)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// the test: the archive is named dasch, and asked only by name.
func TestTheArchiveIsNamedAndAskedByNameOnly(t *testing.T) {
	doer, _ := doerAnswering(t)
	found := archive(t, doer, oneSource)
	var named celestialdata.ByName = found
	if found.Name() != "dasch" || !named.ByNameOnly() {
		t.Errorf("%q asked by name only: %v", found.Name(),
			named.ByNameOnly())
	}
}

// Anti-tests: no doer, no agent, and fewer than one source are refused.
func TestTheArchiveRefusesWhatItCannotAskWith(t *testing.T) {
	doer, _ := doerAnswering(t)
	for name, each := range map[string]struct {
		doer  dasch.Doer
		agent string
		most  int
	}{
		"no doer":    {nil, agent, oneSource},
		"no agent":   {doer, "", oneSource},
		"no source":  {doer, agent, noFiles},
		"below none": {doer, agent, -oneSource},
	} {
		found, err := dasch.New(each.doer, each.agent, each.most)
		if found != nil || !errors.Is(err, celestialdata.Refused()) ||
			!strings.HasPrefix(err.Error(), "refused: dasch ") {
			t.Errorf("%s: %v, %v", name, found, err)
		}
	}
}

// decoded is a request's JSON body, read, or the test stops.
func decoded(t *testing.T, request sent) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(request.text), &body); err != nil {
		t.Fatalf("%q: %v", request.text, err)
	}
	return body
}

// the test: a search is one POST of JSON to the catalogue search of the
// public tier, over https, naming the agent, asking APASS for a box
// round the circle's centre with the circle's radius in arcseconds.
func TestTheSearchIsOnePostToTheCatalogueSearch(t *testing.T) {
	doer, requests := doerAnswering(t,
		answer(http.StatusOK, recorded(t, "querycat.json")))
	_, err := archive(t, doer, twoSources).Find(star(t, starWide))
	if err != nil || len(*requests) != oneSource {
		t.Fatalf("%d requests, %v", len(*requests), err)
	}
	request := (*requests)[first]
	body := decoded(t, request)
	if request.Method != http.MethodPost ||
		request.URL.String() != "https://api.starglass.cfa.harvard."+
			"edu/public/dasch/dr7/querycat" ||
		request.Header.Get("Content-Type") != "application/json" ||
		request.Header.Get("User-Agent") != agent ||
		request.Header.Get("x-api-key") != "" ||
		body["refcat"] != "apass" || body["ra_deg"] != starRA ||
		body["dec_deg"] != starDec ||
		fmt.Sprintf("%.4f", body["radius_arcsec"]) != "5.0400" ||
		len(body) != len([]string{"refcat", "ra_deg", "dec_deg",
			"radius_arcsec"}) {
		t.Errorf("%s %s %v %v", request.Method, request.URL,
			request.Header, request.text)
	}
}

// the test: the products are the sources in the circle, the nearest
// first, each a light curve with the question in its address. The
// source in the box's corner, beyond the circle, is left out.
func TestFindNamesTheSourcesInTheCircleNearestFirst(t *testing.T) {
	reply := answer(http.StatusOK, recorded(t, "querycat.json"))
	doer, _ := doerAnswering(t, reply)
	products, err := archive(t, doer, twoSources).Find(star(t, starWide))
	if err != nil || len(products) != twoSources {
		t.Fatalf("%d products, %v", len(products), err)
	}
	near, far := products[first], products[second]
	if near.Archive() != "dasch" || near.Name() != nearer ||
		near.ID() != "apass-"+starNumber || near.URL() != curveURL ||
		far.Name() != farther ||
		!strings.Contains(near.About(), "DASCH DR7 light curve of "+
			"apass source "+starNumber) ||
		!strings.Contains(near.About(), "0.1 arcseconds") ||
		!strings.Contains(near.About(), "number 1 of the ") ||
		!strings.Contains(far.About(), "number 2 of the ") ||
		*reply.closed != oneSource {
		t.Errorf("%q %q %q %q %q, closed %d", near.Archive(),
			near.Name(), near.ID(), near.URL(), near.About(),
			*reply.closed)
	}
}

// the test: no more sources are named than the archive was made to
// name, the nearest of them.
func TestFindNamesNoMoreThanItWasMadeTo(t *testing.T) {
	doer, _ := doerAnswering(t,
		answer(http.StatusOK, recorded(t, "querycat.json")))
	products, err := archive(t, doer, oneSource).Find(star(t, starWide))
	if err != nil || len(products) != oneSource ||
		products[first].Name() != nearer {
		t.Errorf("%v, %v", products, err)
	}
}

// the test: a circle with no source in it names none, and is no error.
func TestFindNamesNoneForAnEmptyCircle(t *testing.T) {
	doer, _ := doerAnswering(t, answer(http.StatusOK,
		`["ref_number,gsc_bin_index,ra_deg,dec_deg"]`))
	products, err := archive(t, doer, twoSources).Find(star(t, starWide))
	if err != nil || len(products) != noFiles {
		t.Errorf("%v, %v", products, err)
	}
}

// Anti-test: a circle wider than the box the API allows, an hour of arc,
// is refused before anything is asked. One of exactly that is not.
func TestFindRefusesACircleWiderThanTheAPIAllows(t *testing.T) {
	doer, _ := doerAnswering(t)
	products, err := archive(t, doer, twoSources).Find(star(t, 1.0001))
	if products != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "at most 3600 arcseconds") {
		t.Errorf("%v, %v", products, err)
	}
	doer, requests := doerAnswering(t, answer(http.StatusOK,
		`["ref_number,gsc_bin_index,ra_deg,dec_deg"]`))
	_, err = archive(t, doer, twoSources).Find(star(t, 1))
	if err != nil || len(*requests) != oneSource {
		t.Errorf("an hour of arc: %d requests, %v", len(*requests), err)
	}
}

// Anti-tests: what the API answers that is not a catalogue is an error
// that begins with the archive's name, and no product.
func TestFindRefusesWhatIsNotACatalogue(t *testing.T) {
	const columns = "ref_number,gsc_bin_index,ra_deg,dec_deg"
	for name, each := range map[string]struct {
		reply reply
		want  string
	}{
		"a server error": {answer(http.StatusInternalServerError, ""),
			"answered with status 500"},
		"a column named both ways": {answer(http.StatusOK,
			`["ref_number,refNumber,gsc_bin_index,ra_deg,dec_deg"]`),
			"twice"},
		"an object": {answer(http.StatusOK, `{"message":"no"}`),
			"not a list of lines"},
		"not JSON": {answer(http.StatusOK, "<html>"),
			"not a list of lines"},
		"an empty list": {answer(http.StatusOK, "[]"),
			"answered with no lines"},
		"no columns": {answer(http.StatusOK, `["a,b"]`),
			"no [ref_number gsc_bin_index ra_deg dec_deg]; its " +
				"columns are [a b]"},
		"a column twice, in both spellings": {answer(http.StatusOK,
			`["ref_number,refNumber,gsc_bin_index,ra_deg,dec_deg"]`),
			"names the column ref_number twice"},
		"a short line": {answer(http.StatusOK,
			`["`+columns+`","1,2,3"]`), "3 fields, not the 4"},
		"a long line": {answer(http.StatusOK,
			`["`+columns+`","1,2,3,4,5"]`), "5 fields, not the 4"},
		"a number that is none": {answer(http.StatusOK,
			`["`+columns+`","one,2,3,4"]`), "one"},
		"a bin that is none": {answer(http.StatusOK,
			`["`+columns+`","1,two,3,4"]`), "two"},
		"a position that is none": {answer(http.StatusOK,
			`["`+columns+`","1,2,,4"]`), `""`},
		"a negative number": {answer(http.StatusOK,
			`["`+columns+`","-1,2,3,4"]`), "-1"},
		"a bin too large": {answer(http.StatusOK,
			`["`+columns+`","1,4294967296,3,4"]`), "4294967296"},
	} {
		doer, _ := doerAnswering(t, each.reply)
		products, err := archive(t, doer, twoSources).Find(
			star(t, starWide))
		if products != nil || err == nil ||
			!strings.HasPrefix(err.Error(), "dasch: ") ||
			!strings.Contains(err.Error(), each.want) ||
			*each.reply.closed != oneSource {
			t.Errorf("%s: %v, %v", name, products, err)
		}
	}
}

// Anti-tests: a stop of the API, asked through the gentle, is an error
// that begins with the archive's name, is ErrStopped, says that the API
// asked, and what it asked for when it said, and no product.
func TestFindSaysWhenTheAPIAsksUsToStop(t *testing.T) {
	for name, each := range map[string]struct {
		reply reply
		want  string
	}{
		"a stop without a time": {answer(http.StatusTooManyRequests, ""),
			"DASCH asked us to stop"},
		"a stop with one": {answer(http.StatusTooManyRequests, "",
			"Retry-After", "120"), "its Retry-After is 120"},
	} {
		doer, _ := doerAnswering(t, each.reply)
		products, err := archive(t, gentled(t, doer), twoSources).Find(
			star(t, starWide))
		if products != nil || !errors.Is(err, dasch.ErrStopped) ||
			!strings.HasPrefix(err.Error(), "dasch: ") ||
			!strings.Contains(err.Error(), each.want) ||
			*each.reply.closed != oneSource {
			t.Errorf("%s: %v, %v", name, products, err)
		}
	}
}

// Anti-test: a request that cannot be sent is an error that begins with
// the archive's name and says which endpoint, and is not retried.
func TestFindSaysWhenTheRequestFails(t *testing.T) {
	doer := mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).Return(nil, errors.New("no route"))
	products, err := archive(t, doer, twoSources).Find(star(t, starWide))
	if products != nil || err == nil ||
		!strings.HasPrefix(err.Error(), "dasch: POST /dasch/dr7/"+
			"querycat failed: no route") {
		t.Errorf("%v, %v", products, err)
	}
}

// the test: the light curve's address holds the question, and is the
// one thing Lightcurves answers by a POST.
func TestALightCurveIsAskedByAPostOfItsTwoNumbers(t *testing.T) {
	curve := answer(http.StatusOK, recorded(t, "lightcurve.json"))
	doer, requests := doerAnswering(t, curve)
	under := mocks.NewMockWeb(gomock.NewController(t))
	curves, err := dasch.NewLightcurves(under, doer, agent)
	if err != nil {
		t.Fatal(err)
	}
	body, length, err := curves.Get(curveURL)
	if err != nil {
		t.Fatal(err)
	}
	text, readErr := io.ReadAll(body)
	body.Close()
	var listed []string
	if err := json.Unmarshal([]byte(recorded(t, "lightcurve.json")),
		&listed); err != nil {
		t.Fatal(err)
	}
	want := strings.Join(listed, "\n") + "\n"
	request := (*requests)[first]
	asked := decoded(t, request)
	if readErr != nil || string(text) != want ||
		length != int64(len(want)) || len(listed) != curveLines ||
		len(*requests) != oneSource ||
		request.Method != http.MethodPost ||
		request.URL.String() != "https://api.starglass.cfa.harvard."+
			"edu/public/dasch/dr7/lightcurve" ||
		request.Header.Get("User-Agent") != agent ||
		asked["refcat"] != "apass" ||
		fmt.Sprintf("%.0f", asked["ref_number"]) != starNumber ||
		fmt.Sprintf("%.0f", asked["gsc_bin_index"]) != starBin ||
		len(asked) != threeFields || *curve.closed != oneSource {
		t.Errorf("%q %d %s %v", text, length, request.text, readErr)
	}
}

// the test: fetch brings the light curve home as a file of CSV, whose
// first line names the columns, and keeps nothing it was not given.
func TestFetchBringsALightCurveHomeAsCSV(t *testing.T) {
	doer, _ := doerAnswering(t,
		answer(http.StatusOK, recorded(t, "querycat.json")),
		answer(http.StatusOK, recorded(t, "lightcurve.json")))
	products, err := archive(t, doer, oneSource).Find(star(t, starWide))
	if err != nil {
		t.Fatal(err)
	}
	curves, err := dasch.NewLightcurves(
		mocks.NewMockWeb(gomock.NewController(t)), doer, agent)
	if err != nil {
		t.Fatal(err)
	}
	into := t.TempDir()
	path, err := celestialdata.Fetch(curves, products[first], into)
	text, readErr := os.ReadFile(path)
	if err != nil || readErr != nil || filepath.Base(path) != curveName ||
		!strings.HasPrefix(string(text), "date_jd,series,plate_number,") ||
		strings.Count(string(text), "\n") != curveLines {
		t.Errorf("%s: %v, %v, %q", path, err, readErr, text)
	}
}

// Anti-test: a fetch of a light curve the API will not give leaves no
// file, and no .part.
func TestFetchOfALightCurveTheAPIRefusesKeepsNothing(t *testing.T) {
	doer, _ := doerAnswering(t,
		answer(http.StatusOK, recorded(t, "querycat.json")),
		answer(http.StatusInternalServerError, ""))
	products, err := archive(t, doer, oneSource).Find(star(t, starWide))
	if err != nil {
		t.Fatal(err)
	}
	curves, err := dasch.NewLightcurves(
		mocks.NewMockWeb(gomock.NewController(t)), doer, agent)
	if err != nil {
		t.Fatal(err)
	}
	into := t.TempDir()
	_, err = celestialdata.Fetch(curves, products[first], into)
	left, listErr := os.ReadDir(into)
	if err == nil || listErr != nil || len(left) != noFiles {
		t.Errorf("%v, %v, %d files", err, listErr, len(left))
	}
}

// Anti-tests: no web under, no doer and no agent are refused.
func TestLightCurvesRefuseWhatTheyCannotAskWith(t *testing.T) {
	doer, _ := doerAnswering(t)
	under := mocks.NewMockWeb(gomock.NewController(t))
	for name, each := range map[string]struct {
		under *mocks.MockWeb
		doer  dasch.Doer
		agent string
	}{
		"no web":   {nil, doer, agent},
		"no doer":  {under, nil, agent},
		"no agent": {under, doer, ""},
	} {
		got, err := dasch.NewLightcurves(webOf(each.under), each.doer,
			each.agent)
		if got != nil || err == nil {
			t.Errorf("%s: %v, %v", name, got, err)
		}
	}
}

// webOf is a mock web as a web, or none for none: a nil *MockWeb in an
// interface is not nil.
func webOf(under *mocks.MockWeb) web.Web {
	if under == nil {
		return nil
	}
	return under
}

// Anti-tests: an address that is the light curve's but whose numbers are
// not whole, or whose catalogue is not APASS, is an error, and nothing
// is sent to DASCH.
func TestALightCurveAddressThatAsksNonsenseSendsNothing(t *testing.T) {
	doer, requests := doerAnswering(t)
	curves, err := dasch.NewLightcurves(
		mocks.NewMockWeb(gomock.NewController(t)), doer, agent)
	if err != nil {
		t.Fatal(err)
	}
	base := "https://api.starglass.cfa.harvard.edu/public/dasch/dr7/" +
		"lightcurve?"
	for name, query := range map[string]string{
		"no number":         "refcat=apass&gsc_bin_index=1",
		"a negative":        "refcat=apass&ref_number=-1&gsc_bin_index=1",
		"a fraction":        "refcat=apass&ref_number=1&gsc_bin_index=1.5",
		"a bin too large":   "refcat=apass&ref_number=1&gsc_bin_index=4294967296",
		"another catalogue": "refcat=atlas&ref_number=1&gsc_bin_index=1",
		"no catalogue":      "ref_number=1&gsc_bin_index=1",
	} {
		body, length, err := curves.Get(base + query)
		if body != nil || length != unsaid || err == nil ||
			!strings.HasPrefix(err.Error(), base) {
			t.Errorf("%s: %v, %d, %v", name, body, length, err)
		}
	}
	if len(*requests) != noFiles {
		t.Errorf("%d requests sent", len(*requests))
	}
}

// Anti-test: a 200 that holds no light curve is an error with no body.
func TestAnAnswerThatIsNoLightCurveIsAnError(t *testing.T) {
	doer, _ := doerAnswering(t, answer(http.StatusOK, `["no such source"]`))
	curves, err := dasch.NewLightcurves(
		mocks.NewMockWeb(gomock.NewController(t)), doer, agent)
	if err != nil {
		t.Fatal(err)
	}
	body, length, err := curves.Get(curveURL)
	if body != nil || length != unsaid || err == nil ||
		!strings.Contains(err.Error(), "not a light curve") {
		t.Errorf("%v, %d, %v", body, length, err)
	}
}

// gentled is a doer asked through the gentle of this package's policy,
// as a run on the network asks. Its one question goes at once, so it
// never waits.
func gentled(t *testing.T, under dasch.Doer) dasch.Doer {
	t.Helper()
	policy, err := dasch.Policy()
	if err != nil {
		t.Fatal(err)
	}
	gentle, err := web.NewGentle(policy, web.Wall(), web.Unix())
	if err != nil {
		t.Fatal(err)
	}
	through, err := web.Through(under, gentle)
	if err != nil {
		t.Fatal(err)
	}
	return through
}

// Anti-test: the API asking us to stop is an error with no body, that is
// ErrStopped, and says what it asked for.
func TestALightCurveTheAPIRefusesIsAnError(t *testing.T) {
	doer, _ := doerAnswering(t, answer(http.StatusTooManyRequests, "",
		"Retry-After", "30"))
	curves, err := dasch.NewLightcurves(
		mocks.NewMockWeb(gomock.NewController(t)), gentled(t, doer), agent)
	if err != nil {
		t.Fatal(err)
	}
	body, length, err := curves.Get(curveURL)
	if body != nil || length != unsaid || !errors.Is(err,
		dasch.ErrStopped) || !strings.Contains(err.Error(),
		"its Retry-After is 30") {
		t.Errorf("%v, %d, %v", body, length, err)
	}
}

// Anti-test: an answer that is not a light curve's shape is an error
// that names the address, opens nothing and says what is wrong: its
// first line names a column twice, or begins with another column, or a
// line after it has the wrong number of fields. The line is counted from
// one with the header first, so the first line after it is line 2.
func TestALightCurveOfAnotherShapeIsAnError(t *testing.T) {
	for name, each := range map[string]struct{ listed, want string }{
		"a column twice": {`["date_jd,dateJd"]`,
			"names the column date_jd twice"},
		"another first column": {`["mag,date_jd","1,2"]`,
			`its first line begins "mag", not "date_jd"`},
		"a short first line": {`["date_jd,mag","1","1,2"]`,
			"line 2 has 1 fields, not the 2 of its columns"},
		"a long third line": {`["date_jd,mag","1,2","3,4","5,6,7"]`,
			"line 4 has 3 fields, not the 2 of its columns"},
	} {
		doer, _ := doerAnswering(t, answer(http.StatusOK, each.listed))
		curves, err := dasch.NewLightcurves(
			mocks.NewMockWeb(gomock.NewController(t)), doer, agent)
		if err != nil {
			t.Fatal(err)
		}
		body, length, err := curves.Get(curveURL)
		if body != nil || length != unsaid || err == nil ||
			!strings.HasPrefix(err.Error(), curveURL+": ") ||
			!strings.Contains(err.Error(), each.want) {
			t.Errorf("%s: %v, %d, %v", name, body, length, err)
		}
	}
}

// Anti-test: an answer whose body cannot be read is an error that says
// so and names what was asked, and opens nothing.
func TestALightCurveThatCannotBeReadIsAnError(t *testing.T) {
	closed := new(int)
	broken := reply{closed: closed, response: &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{},
		Body: closing{Reader: iotest.ErrReader(errors.New("cut off")),
			closed: closed}}}
	doer, _ := doerAnswering(t, broken)
	curves, err := dasch.NewLightcurves(
		mocks.NewMockWeb(gomock.NewController(t)), doer, agent)
	if err != nil {
		t.Fatal(err)
	}
	body, length, err := curves.Get(curveURL)
	if body != nil || length != unsaid || err == nil ||
		!strings.Contains(err.Error(), "reading the answer: cut off") ||
		*closed != oneSource {
		t.Errorf("%v, %d, %v, closed %d", body, length, err, *closed)
	}
}

// the test: anything but a light curve goes to the web under, unchanged:
// a Get, a Head and a Post, and an address that is no address at all.
func TestAnythingElseGoesToTheWebUnder(t *testing.T) {
	doer, requests := doerAnswering(t)
	under := mocks.NewMockWeb(gomock.NewController(t))
	curves, err := dasch.NewLightcurves(under, doer, agent)
	if err != nil {
		t.Fatal(err)
	}
	const other = "https://example.org/file.fits"
	const broken = "%zz"
	const same = "https://api.starglass.cfa.harvard.edu/public/dasch/" +
		"dr7/querycat?refcat=apass"
	form := url.Values{"a": {"b"}}
	for _, address := range []string{other, broken, same} {
		under.EXPECT().Get(address).Return(io.NopCloser(
			strings.NewReader("held")), int64(4), nil)
		body, length, err := curves.Get(address)
		text, _ := io.ReadAll(body)
		if err != nil || length != 4 || string(text) != "held" {
			t.Errorf("%s: %q, %d, %v", address, text, length, err)
		}
	}
	under.EXPECT().Head(other).Return(int64(7), nil)
	under.EXPECT().Post(other, form).Return("elsewhere", nil)
	length, headErr := curves.Head(other)
	where, postErr := curves.Post(other, form)
	if headErr != nil || length != 7 || postErr != nil ||
		where != "elsewhere" || len(*requests) != noFiles {
		t.Errorf("%d %v %q %v", length, headErr, where, postErr)
	}
}

// the test: a light curve has no length until it is asked for, and
// asking would be the whole question, so a Head says none and asks no one.
func TestAHeadOfALightCurveAsksNoOne(t *testing.T) {
	doer, requests := doerAnswering(t)
	curves, err := dasch.NewLightcurves(
		mocks.NewMockWeb(gomock.NewController(t)), doer, agent)
	if err != nil {
		t.Fatal(err)
	}
	length, err := curves.Head(curveURL)
	if err != nil || length != unsaid || len(*requests) != noFiles {
		t.Errorf("%d, %v, %d requests", length, err, len(*requests))
	}
}
