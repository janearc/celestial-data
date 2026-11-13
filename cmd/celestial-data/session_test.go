package main

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janearc/celestial-data/gaia"
	"go.uber.org/mock/gomock"
)

// The tests' own numbers, each defined here.
const (
	unknownSize int64 = -1 // defined: the size a web answers when it does not know
	notFound    int   = -1 // defined: strings.Index when the text is absent
)

// answerText is what the archive's job answers: the front of a file.
const answerText = "SIMPLE"

// signedIn is the cookies the archive sets a user who signs in.
func signedIn() []*http.Cookie {
	return []*http.Cookie{{Name: "JSESSIONID", Value: "s1"}}
}

// sessionAt answers a lookup of the session variable with a path, and
// any other with nothing.
type sessionAt string

// lookup is the path for the session variable, nothing for the rest.
func (path sessionAt) lookup(name string) string {
	if name == gaiaSession {
		return string(path)
	}
	return ""
}

// signing is celestial-data with a web that keeps a session, a mock,
// and GAIA_SESSION as given.
func signing(t *testing.T, session string) (command, *Mocksession) {
	t.Helper()
	signed := NewMocksession(gomock.NewController(t))
	tool := command{
		picked: picked(t, numbersRaw), signed: signed,
		lookup: sessionAt(session).lookup,
	}
	return tool, signed
}

// keptSession checks the cookies the web is given to keep: the saved
// session's one cookie.
type keptSession struct{ t *testing.T }

// keep is the web keeping the cookies, checked.
func (kept keptSession) keep(_ string, cookies []*http.Cookie) error {
	if len(cookies) != len(signedIn()) {
		kept.t.Errorf("kept %v", cookies)
	}
	for _, cookie := range cookies {
		if cookie.Value != "s1" {
			kept.t.Errorf("kept %v", cookie)
		}
	}
	return nil
}

// the web the tool fetches through is given the session GAIA_SESSION
// names.
func TestTheWebSendsTheSessionGaiaSessionNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session")
	if err := gaia.Save(signedIn(), path); err != nil {
		t.Fatal(err)
	}
	tool, signed := signing(t, path)
	signed.EXPECT().Keep(gaia.Service, gomock.Any()).DoAndReturn(
		keptSession{t}.keep)
	if got, err := tool.web(); err != nil || got != signed {
		t.Fatalf("got %v, %v", got, err)
	}
}

// the anti-test: a session file that is missing, or one the web will
// not keep, is an error, not a web.
func TestTheWebWithASessionItCannotSendIsAnError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	tool, _ := signing(t, missing)
	if _, err := tool.web(); err == nil {
		t.Error("a missing session was sent")
	}
	path := filepath.Join(t.TempDir(), "session")
	if err := gaia.Save(signedIn(), path); err != nil {
		t.Fatal(err)
	}
	tool, signed := signing(t, path)
	signed.EXPECT().Keep(gaia.Service, gomock.Any()).Return(
		errors.New("no jar"))
	if _, err := tool.web(); err == nil {
		t.Error("a session that was not kept was taken as sent")
	}
}

// job is the address of the gaia archive's job the fetch test submits.
const job = "https://gea.esac.esa.int/tap-server/tap/async/1730"

// submitting checks the job the fetch submits: a run of the gaia query.
type submitting struct{ t *testing.T }

// submit is the archive taking the job and answering its address.
func (asked submitting) submit(_ string, form url.Values) (string, error) {
	if form.Get("PHASE") != "run" ||
		!strings.HasSuffix(form.Get("QUERY"), gaiaWhere) {
		asked.t.Errorf("submitted %v", form)
	}
	return job, nil
}

// fetch of the gaia archive submits its job, waits for it, and brings
// its answer home under the archive's folder, saying the job's address.
func TestFetchRunsTheGaiaJobAndBringsItsAnswerHome(t *testing.T) {
	get := quiet(t)
	complete := io.NopCloser(strings.NewReader(
		"<job><phase>COMPLETED</phase></job>"))
	answer := io.NopCloser(strings.NewReader(answerText))
	size := int64(len(answerText))
	gomock.InOrder(
		get.EXPECT().Post("https://gea.esac.esa.int/tap-server/tap/async",
			gomock.Any()).DoAndReturn(submitting{t}.submit),
		get.EXPECT().Get(job).Return(complete, unknownSize, nil),
		get.EXPECT().Get(job+"/results/result").Return(answer, size, nil))
	into := t.TempDir()
	out, errs, code := ran(through(t, get), "fetch", "--archive", "gaia",
		"--ra", "266.4168371", "--dec", "-29.0078106", "--radius", "0.1",
		"--into", into)
	got, err := os.ReadFile(filepath.Join(into, "gaia",
		"gaia_source_266.4168371_-29.0078106_r0.1.fits"))
	if code != exitOK || err != nil || string(got) != answerText {
		t.Fatalf("exit %d, %q, %v: %s", code, got, err, errs)
	}
	if !strings.Contains(out, "job "+job+" submitted") ||
		!strings.Contains(out, "job "+job+" complete") {
		t.Fatalf("fetch did not say its job: %s", out)
	}
}

// the anti-test: jobs without their numbers, how often to ask and how
// long to wait, are refused.
func TestJobsWithoutTheirNumbersAreRefused(t *testing.T) {
	for _, given := range []map[string]string{
		{"job-asks": "3"},
		{"job-every": "1"},
	} {
		tool := command{picked: picked(t, numbersWith(given))}
		if _, err := tool.jobs(quiet(t), io.Discard); err == nil {
			t.Errorf("with %v, jobs were made", given)
		}
	}
}

// the anti-test: a fetch whose numbers file lacks the jobs' numbers asks
// the archive nothing.
func TestFetchWithoutTheJobNumbersAsksNothing(t *testing.T) {
	cut := strings.Index(string(numbersRaw), "  - name: job-every")
	if cut == notFound {
		t.Fatal("numbers.yaml has no job-every")
	}
	tool := command{get: quiet(t), picked: picked(t, numbersRaw[:cut])}
	if _, _, code := ran(tool, "fetch", "--archive", "galcen_psc",
		"--ra", "266.4168371", "--dec", "-29.0078106", "--radius", "0.1",
		"--into", t.TempDir()); code == exitOK {
		t.Fatal("fetch ran with no numbers for its jobs")
	}
}
