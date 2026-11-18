package ntrs_test

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/janearc/celestial-data/ntrs"
	"github.com/janearc/celestial-data/web"
)

// spaces is a body that never ends: every read fills the buffer with
// spaces, which JSON allows anywhere. It shows a read is cut off.
type spaces struct{}

// Read fills the buffer.
func (spaces) Read(into []byte) (int, error) {
	for at := range into {
		into[at] = ' '
	}
	return len(into), nil
}

// padded is a JSON answer of exactly this many bytes: the text, then
// spaces.
func padded(text string, size int64) string {
	return text + strings.Repeat(" ", int(size)-len(text))
}

// An answer to a search or a lookup that goes on past what a page of
// citations could hold is refused, and says how much was allowed. The
// body that never ends proves the read stops.
func TestAnAnswerThatNeverEndsIsRefused(t *testing.T) {
	for name, ask := range askers(t) {
		answer, _ := reply(t, http.StatusOK, "", "")
		body := &closing{Reader: spaces{}}
		answer.Body = body
		err := ask(answer)
		if err == nil || !strings.Contains(err.Error(), "is over") ||
			!strings.Contains(err.Error(), "ntrs-answer-most") ||
			body.closed != oneCitation {
			t.Errorf("%s: %v, %d closes", name, err, body.closed)
		}
	}
}

// askers are the two ways of asking NTRS that read an answer, each
// given the answer its mock Doer gives and giving the error.
func askers(t *testing.T) map[string]func(*http.Response) error {
	return map[string]func(*http.Response) error{
		"search": func(answer *http.Response) error {
			_, _, err := search(t, answer)
			return err
		},
		"lookup": func(answer *http.Response) error {
			_, _, err := lookup(t, answer)
			return err
		},
	}
}

// broken is a body that fails as it is read.
type broken struct{}

// Read fails.
func (broken) Read([]byte) (int, error) { return 0, errors.New("cut") }

// An answer whose body fails part way is an error that says it was cut
// off, and is not read as JSON that is not.
//
// Anti-test: it is not the error of a body that read but was not JSON.
func TestAnAnswerCutOffIsSaidSo(t *testing.T) {
	for name, ask := range askers(t) {
		answer, _ := reply(t, http.StatusOK, "", "")
		body := &closing{Reader: broken{}}
		answer.Body = body
		err := ask(answer)
		if err == nil || !strings.Contains(err.Error(), "cut off") ||
			strings.Contains(err.Error(), "not JSON") ||
			body.closed != oneCitation {
			t.Errorf("%s: %v, %d closes", name, err, body.closed)
		}
	}
}

// Anti-test: an answer of exactly the limit is read, and the body is
// closed.
func TestAnAnswerAtTheLimitIsRead(t *testing.T) {
	size := ntrs.AnswerLimit()
	answer, body := reply(t, http.StatusOK, "", padded(`{"results": [], `+
		`"stats": {"total": 0}}`, size))
	found, _, err := search(t, answer)
	if err != nil || found == nil || found.Count() != noCitations ||
		body.closed != oneCitation {
		t.Errorf("%v, %v, %d closes", found, err, body.closed)
	}
	answer, _ = reply(t, http.StatusOK, "", padded(`{"id": 20990000001}`,
		size))
	if cited, _, err := lookup(t, answer); err != nil || cited == nil {
		t.Errorf("%v, %v", cited, err)
	}
}

// A PDF address whose file name hides a slash or a dot name behind
// percent signs is refused, since the server may read it as one.
//
// Anti-test: a name with an escaped space is a plain file.
func TestAnEscapedNameThatLeavesItsFolderIsRefused(t *testing.T) {
	folder := "/api/citations/20990000001/downloads/"
	for _, bad := range []string{folder + "%2e%2e", folder + "%2E",
		folder + "a%2fb.pdf", folder + "a%2Fb.pdf", folder + "a%5cb.pdf",
		folder + "%zz.pdf"} {
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
		`{"pdf": "`+folder+`a%20b.pdf"}}]}`)
	cited, _, _ := lookup(t, good)
	if _, refused := cited.PDF(); refused != nil {
		t.Errorf("refused: %v", refused)
	}
}

// NTRS is asked in a way of its own: the host is the one the search is
// sent to, the numbers are its own and not the default, and the gentle
// accepts them.
func TestNTRSHasAPolicyOfItsOwn(t *testing.T) {
	where, err := url.Parse(endpoint)
	if err != nil || where.Hostname() != ntrs.Host {
		t.Fatalf("%v: %q is not %q", err, where.Hostname(), ntrs.Host)
	}
	policy := ntrs.Policy()
	if policy.Defaulted || policy.Host != "NTRS" ||
		policy.Pace != 2*time.Second || policy.Tries != 2 ||
		policy.Opens != 2 || !policy.HoldsUntilClosed {
		t.Errorf("%+v", policy)
	}
	clock := web.Wall()
	if _, err := web.NewGentle(policy, clock, web.Unix()); err != nil {
		t.Error(err)
	}
}

// A search only asks a question, so it is the one POST the policy says
// is safe to repeat. Anti-test: the default policy says no to every
// POST, and NTRS's is not the default.
func TestASearchMayBeRepeatedAndTheDefaultMayNot(t *testing.T) {
	standard, err := web.DefaultPolicy("any")
	if err != nil || standard.RepeatsPosts || !ntrs.Policy().RepeatsPosts {
		t.Errorf("%v: default %v, ntrs %v", err, standard.RepeatsPosts,
			ntrs.Policy().RepeatsPosts)
	}
}
