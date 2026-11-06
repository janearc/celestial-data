package web_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/janearc/celestial-data/web"
	"github.com/janearc/puzzlemouth/numbers"
)

// numbersFile is where the picked numbers these tests use are kept.
const numbersFile = "testdata/numbers.yaml"

// unsaid is the length Web gives when the server says none. It's defined
// by Web.
const unsaid = -1

// agent is what the client here says it is.
const agent = "celestial-data-test"

// photons is what /file holds.
const photons = "photons"

// limitless is no limit on the whole request. It's defined by web.New.
const limitless time.Duration = 0

// seconds is a picked number of seconds from the numbers file, as a time.
func seconds(t *testing.T, name string) time.Duration {
	t.Helper()
	raw, err := os.ReadFile(numbersFile)
	if err != nil {
		t.Fatal(err)
	}
	held, err := numbers.Read(numbersFile, raw)
	if err != nil {
		t.Fatal(err)
	}
	number, err := held.Get(name)
	if err != nil {
		t.Fatal(err)
	}
	return web.Seconds(number.Value)
}

// patient is a client that waits long enough for a local server, with no
// limit on the whole request.
func patient(t *testing.T) web.Client {
	t.Helper()
	return web.New(seconds(t, "patient-wait"), limitless, agent)
}

// serve is the local archive. Its /file holds photons with its length, and
// /chunked holds the same without one. Its /agent says the user agent it
// was sent. Anything else is not found.
func serve(answer http.ResponseWriter, asked *http.Request) {
	switch asked.URL.Path {
	case "/file":
		length := strconv.Itoa(len(photons))
		answer.Header().Set("Content-Length", length)
		io.WriteString(answer, photons)
	case "/chunked":
		answer.(http.Flusher).Flush()
		io.WriteString(answer, photons)
	case "/choices":
		answer.WriteHeader(http.StatusMultipleChoices)
	case "/busy":
		answer.Header().Set("Retry-After", busyFor)
		answer.WriteHeader(http.StatusTooManyRequests)
	case "/agent":
		io.WriteString(answer, asked.UserAgent())
	case "/submit", "/sign", "/cookie", "/lost":
		served(answer, asked)
	default:
		http.NotFound(answer, asked)
	}
}

// archive is the local archive, running until the test ends.
func archive(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(serve))
	t.Cleanup(server.Close)
	return server
}

// read is all of a body, closed after.
func read(t *testing.T, body io.ReadCloser) string {
	t.Helper()
	defer body.Close()
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

func TestGetGivesTheBodyAndItsLength(t *testing.T) {
	server := archive(t)
	body, length, err := patient(t).Get(server.URL + "/file")
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, body); got != photons ||
		length != int64(len(photons)) {
		t.Errorf("%q, %d", got, length)
	}
}

func TestGetSaysWhenTheServerGaveNoLength(t *testing.T) {
	server := archive(t)
	body, length, err := patient(t).Get(server.URL + "/chunked")
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, body); got != photons || length != unsaid {
		t.Errorf("%q, length %d", got, length)
	}
}

func TestHeadGivesTheLengthAlone(t *testing.T) {
	server := archive(t)
	length, err := patient(t).Head(server.URL + "/file")
	if err != nil || length != int64(len(photons)) {
		t.Errorf("%d, %v", length, err)
	}
}

func TestEveryRequestSaysWhoIsAsking(t *testing.T) {
	server := archive(t)
	body, _, err := patient(t).Get(server.URL + "/agent")
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, body); got != agent {
		t.Errorf("the server was told %q", got)
	}
}

// Anti-tests: any answer but a success is an error naming the URL and the
// status. That holds for GET and HEAD alike.
// Answers that are not a success are errors that name the address and
// the status, with no length beside them. That includes 300, which says
// the server has more than one answer and gives none.
func TestAFailedAnswerIsAnError(t *testing.T) {
	server := archive(t)
	for _, path := range []string{"/missing", "/choices"} {
		url := server.URL + path
		_, length, got := patient(t).Get(url)
		if got == nil || !strings.Contains(got.Error(), url) ||
			length != nothingCame {
			t.Errorf("GET %s: %d, %v", path, length, got)
		}
		headed, err := patient(t).Head(url)
		if err == nil || headed != nothingCame {
			t.Errorf("HEAD %s: %d, %v", path, headed, err)
		}
	}
}

// nothingCame is the length beside an error. It's defined by Web.
const nothingCame = 0

// busyFor is the Retry-After /busy answers with, as a server writes it.
const busyFor = "120"

// A server that asks us to wait is refused as a Status that says so:
// its code, and the wait as the server wrote it, for GET and HEAD alike.
// Its message is the one a refusal always had: the method, the address
// and the status.
func TestARefusalSaysItsCodeAndTheWaitAsked(t *testing.T) {
	url := archive(t).URL + "/busy"
	_, _, got := patient(t).Get(url)
	_, headed := patient(t).Head(url)
	for method, err := range map[string]error{http.MethodGet: got,
		http.MethodHead: headed} {
		var status *web.Status
		if !errors.As(err, &status) ||
			status.Code() != http.StatusTooManyRequests ||
			status.RetryAfter() != busyFor ||
			err.Error() != method+" "+url+": 429 Too Many Requests" {
			t.Errorf("%s: %v", method, err)
		}
	}
}

// Anti-test: a refusal that asks no wait has none, and its code is its
// own.
func TestARefusalThatAsksNoWaitHasNone(t *testing.T) {
	_, _, err := patient(t).Get(archive(t).URL + "/missing")
	var status *web.Status
	if !errors.As(err, &status) || status.RetryAfter() != "" ||
		status.Code() != http.StatusNotFound {
		t.Errorf("%v", err)
	}
}

// Counts of seconds are that many seconds, parts of one too.
func TestSecondsAreSeconds(t *testing.T) {
	want := time.Second + time.Second/halves
	if got := web.Seconds(aSecondAndAHalf); got != want {
		t.Errorf("%v", got)
	}
}

// The second and a half, as a count and in halves. Each is defined.
const (
	aSecondAndAHalf = 1.5 // defined: a second and a half
	halves          = 2   // defined: what a second is cut in to halve it
)

func TestAServerThatIsNotThereIsAnError(t *testing.T) {
	server := archive(t)
	url := server.URL + "/file"
	server.Close()
	if _, _, err := patient(t).Get(url); err == nil {
		t.Error("GET reached a closed server")
	}
	if _, err := patient(t).Head(url); err == nil {
		t.Error("HEAD reached a closed server")
	}
}

func TestAnAddressThatIsNotOneIsAnError(t *testing.T) {
	if _, _, err := patient(t).Get("://nowhere"); err == nil {
		t.Error("asked for nothing")
	}
}

// lagging is a server that waits before it answers.
type lagging time.Duration

// serve waits, then answers with nothing.
func (delay lagging) serve(http.ResponseWriter, *http.Request) {
	time.Sleep(time.Duration(delay))
}

// The client gives up on a server that begins its answer too late.
func TestASlowServerIsGivenUpOn(t *testing.T) {
	slow := lagging(seconds(t, "slow-server"))
	server := httptest.NewServer(http.HandlerFunc(slow.serve))
	t.Cleanup(server.Close)
	hasty := web.New(seconds(t, "hasty-wait"), limitless, agent)
	if _, _, err := hasty.Get(server.URL); err == nil {
		t.Error("waited past its wait")
	}
}

// served is the archive's answers to a form and to a session. /submit
// takes a form whose QUERY is SELECT 1 and sends it on to /job/1. /sign
// sets a session, and /cookie says the session it was sent. /lost sends
// on to an address that is not one.
func served(answer http.ResponseWriter, asked *http.Request) {
	switch asked.URL.Path {
	case "/submit":
		if asked.FormValue("QUERY") != "SELECT 1" {
			http.Error(answer, "no", http.StatusBadRequest)
			return
		}
		http.Redirect(answer, asked, "/job/1", http.StatusSeeOther)
	case "/sign":
		http.SetCookie(answer, &http.Cookie{Name: "JSESSIONID",
			Value: "s1", Path: "/"})
	case "/cookie":
		session, _ := asked.Cookie("JSESSIONID")
		io.WriteString(answer, session.String())
	case "/lost":
		answer.Header().Set("Location", "%zz")
		answer.WriteHeader(http.StatusSeeOther)
	}
}

func TestPostGivesWhereTheAnswerSendsOn(t *testing.T) {
	server := archive(t)
	where, err := patient(t).Post(server.URL+"/submit",
		url.Values{"QUERY": {"SELECT 1"}})
	if err != nil || where != server.URL+"/job/1" {
		t.Fatalf("got %q, %v", where, err)
	}
}

func TestPostWithNowhereToGoGivesNone(t *testing.T) {
	where, err := patient(t).Post(archive(t).URL+"/sign", nil)
	if err != nil || where != "" {
		t.Fatalf("got %q, %v", where, err)
	}
}

func TestPostRefusedNamesTheAddressAndNeverTheForm(t *testing.T) {
	server := archive(t)
	for _, path := range []string{"/submit", "/lost"} {
		_, err := patient(t).Post(server.URL+path,
			url.Values{"password": {"hunter2"}})
		if err == nil || !strings.Contains(err.Error(), path) ||
			strings.Contains(err.Error(), "hunter2") {
			t.Errorf("%s: got %v", path, err)
		}
	}
}

func TestPostToNothingIsAnError(t *testing.T) {
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	for _, address := range []string{gone.URL + "/submit", "::"} {
		if _, err := patient(t).Post(address, nil); err == nil {
			t.Errorf("posted to %q", address)
		}
	}
}

func TestASessionSetIsSentAfter(t *testing.T) {
	server := archive(t)
	client := patient(t)
	if _, err := client.Post(server.URL+"/sign", nil); err != nil {
		t.Fatal(err)
	}
	body, _, err := client.Get(server.URL + "/cookie")
	if err != nil || read(t, body) != "JSESSIONID=s1" {
		t.Fatalf("the session was not sent: %v", err)
	}
	held, err := client.Cookies(server.URL)
	if err != nil || len(held) != 1 || held[0].Value != "s1" {
		t.Fatalf("held %v, %v", held, err)
	}
}

func TestASessionKeptIsSentAndNoneIsSentUnasked(t *testing.T) {
	server := archive(t)
	fresh := patient(t)
	body, _, err := fresh.Get(server.URL + "/cookie")
	if err != nil || read(t, body) != "" {
		t.Fatalf("a fresh client sent a session: %v", err)
	}
	kept := []*http.Cookie{{Name: "JSESSIONID", Value: "s2"}}
	if err := fresh.Keep(server.URL, kept); err != nil {
		t.Fatal(err)
	}
	body, _, err = fresh.Get(server.URL + "/cookie")
	if err != nil || read(t, body) != "JSESSIONID=s2" {
		t.Fatalf("the kept session was not sent: %v", err)
	}
}

func TestCookiesForWhatIsNotAnAddressIsAnError(t *testing.T) {
	client := patient(t)
	if _, err := client.Cookies("::"); err == nil {
		t.Error("cookies for no address")
	}
	if err := client.Keep("::", nil); err == nil {
		t.Error("kept cookies for no address")
	}
}
