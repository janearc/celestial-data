package main

import (
	"errors"
	"fmt"
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

// asOp is the variable that makes this test binary answer as op does.
const asOp = "CELESTIAL_DATA_TEST_AS_OP"

// item is the op item the tests sign in with.
const item = "op://Personal/gaia"

// The tests' own numbers, each defined here.
const (
	opRefuses         = 1  // defined: the pretend op's exit with no such item
	unknownSize int64 = -1 // defined: the size a web answers when it does not know
	notFound          = -1 // defined: strings.Index when the text is absent
)

// answerText is what the archive's job answers: the front of a file.
const answerText = "SIMPLE"

// answerAsOp answers op read for the item's username, as op would, and
// refuses anything else. It gives the code to exit with.
func answerAsOp(args []string) int {
	if len(args) == 2 && args[0] == "read" && args[1] == item+"/username" {
		fmt.Println("ada")
		return exitOK
	}
	fmt.Fprintln(os.Stderr, "[ERROR] no such item")
	return opRefuses
}

// self is this test binary, which answers as op when asked to.
func self(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// Example_gaiaLogin says what gaia-login would do, and reads nothing.
func Example_gaiaLogin() {
	fmt.Print(said("gaia-login", "--op", item, "--session",
		"gaia.session", "--dry-run"))
	// Output:
	// would read op://Personal/gaia/username and op://Personal/gaia/password with op, sign in to https://gea.esac.esa.int/tap-server, and keep the session in gaia.session
}

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

// signing is celestial-data with op holding the login and a web that
// keeps a session, both mocks, with GAIA_SESSION as given.
func signing(t *testing.T, session string) (command, *Mocksecrets,
	*Mocksession,
) {
	t.Helper()
	controller := gomock.NewController(t)
	keys, signed := NewMocksecrets(controller), NewMocksession(controller)
	tool := command{
		picked: picked(t, numbersRaw), keys: keys,
		signed: signed, lookup: sessionAt(session).lookup,
	}
	return tool, keys, signed
}

// login is op giving the login and the archive taking it.
func login(keys *Mocksecrets, signed *Mocksession) {
	keys.EXPECT().Read(item+"/username").Return("ada", nil)
	keys.EXPECT().Read(item+"/password").Return("pw", nil)
	signed.EXPECT().Post("https://gea.esac.esa.int/tap-server/login",
		url.Values{"username": {"ada"}, "password": {"pw"}}).Return("",
		nil)
}

// gaia-login keeps the session where --session says, and says where.
func TestGaiaLoginKeepsTheSessionWhereItIsTold(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session")
	tool, keys, signed := signing(t, "")
	login(keys, signed)
	signed.EXPECT().Cookies(gaia.Service).Return(signedIn(), nil)
	out, errs, code := ran(tool, "gaia-login", "--op", item, "--session",
		path)
	if code != exitOK || !strings.Contains(out, "the session is in "+
		path) {
		t.Fatalf("exit %d: %s%s", code, out, errs)
	}
	if _, err := gaia.Load(path); err != nil {
		t.Fatal(err)
	}
}

// gaia-login keeps the session where GAIA_SESSION says when --session
// is not given.
func TestGaiaLoginKeepsTheSessionWhereGaiaSessionSays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session")
	tool, keys, signed := signing(t, path)
	login(keys, signed)
	signed.EXPECT().Cookies(gaia.Service).Return(signedIn(), nil)
	if _, errs, code := ran(tool, "gaia-login", "--op",
		item); code != exitOK {
		t.Fatalf("exit %d: %s", code, errs)
	}
	if _, err := gaia.Load(path); err != nil {
		t.Fatal(err)
	}
}

// the anti-test: gaia-login without an item, or without anywhere to
// keep the session, is refused before op or the archive is asked.
func TestGaiaLoginWithoutAnItemOrAPlaceIsRefused(t *testing.T) {
	tool, _, _ := signing(t, "") // neither mock takes a call
	for _, words := range [][]string{
		{"--op", item},
		{"--session", "s"},
	} {
		_, _, code := ran(tool, append([]string{"gaia-login"},
			words...)...)
		if code == exitOK {
			t.Errorf("gaia-login %v ran", words)
		}
	}
}

// the anti-test: when op refuses the username or the password,
// gaia-login stops and says what op said.
func TestGaiaLoginStopsWhenOpSaysNo(t *testing.T) {
	for _, field := range []string{"username", "password"} {
		tool, keys, _ := signing(t, "")
		if field == "password" {
			keys.EXPECT().Read(item+"/username").Return("ada", nil)
		}
		keys.EXPECT().Read(item+"/"+field).Return("",
			errors.New("locked"))
		path := filepath.Join(t.TempDir(), "s")
		_, errs, code := ran(tool, "gaia-login", "--op", item,
			"--session", path)
		if code == exitOK || !strings.Contains(errs, "locked") {
			t.Errorf("%s: exit %d: %s", field, code, errs)
		}
	}
}

// the anti-test: a login the archive refuses, or answers without a
// session, keeps no file and never says the password.
func TestGaiaLoginThatIsNotTakenKeepsNothing(t *testing.T) {
	for _, why := range []string{"refused", "cookieless", "unheld"} {
		path := filepath.Join(t.TempDir(), "session")
		tool, keys, signed := signing(t, "")
		keys.EXPECT().Read(item+"/username").Return("ada", nil)
		keys.EXPECT().Read(item+"/password").Return("hunter2", nil)
		posted := signed.EXPECT().Post(gomock.Any(), gomock.Any())
		switch why {
		case "refused":
			posted.Return("", errors.New("POST: 401 Unauthorized"))
		case "cookieless":
			posted.Return("", nil)
			signed.EXPECT().Cookies(gaia.Service).Return(nil, nil)
		case "unheld":
			posted.Return("", nil)
			signed.EXPECT().Cookies(gaia.Service).Return(nil,
				errors.New("no jar"))
		}
		_, errs, code := ran(tool, "gaia-login", "--op", item,
			"--session", path)
		if _, err := os.Stat(path); code == exitOK ||
			strings.Contains(errs, "hunter2") || err == nil {
			t.Errorf("%s: exit %d, kept %v: %s", why, code, err, errs)
		}
	}
}

// the anti-test: gaia-login with no wait number to ask the archive with
// is refused.
func TestGaiaLoginWithNoWaitToAskWithIsRefused(t *testing.T) {
	keys := NewMocksecrets(gomock.NewController(t))
	keys.EXPECT().Read(item+"/username").Return("ada", nil)
	keys.EXPECT().Read(item+"/password").Return("pw", nil)
	tool := command{picked: picked(t, numbersWith(nil)), keys: keys}
	path := filepath.Join(t.TempDir(), "s")
	if _, _, code := ran(tool, "gaia-login", "--op", item, "--session",
		path); code == exitOK {
		t.Fatal("gaia-login signed in with no wait")
	}
}

// op's answer is what onePassword reads: this binary, told to answer as
// op, prints the username.
func TestOnePasswordReadsWhatOpPrints(t *testing.T) {
	t.Setenv(asOp, "1")
	got, err := onePassword{os.Args[0]}.Read(item + "/username")
	if err != nil || got != "ada" {
		t.Fatalf("got %q, %v", got, err)
	}
}

// the anti-test: op's refusal is an error, not a secret.
func TestOnePasswordSaysWhenOpRefuses(t *testing.T) {
	t.Setenv(asOp, "1")
	if _, err := (onePassword{self(t)}).Read(item +
		"/nope"); err == nil {
		t.Fatal("op's refusal was taken as a secret")
	}
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
	tool, _, signed := signing(t, path)
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
	tool, _, _ := signing(t, missing)
	if _, err := tool.web(); err == nil {
		t.Error("a missing session was sent")
	}
	path := filepath.Join(t.TempDir(), "session")
	if err := gaia.Save(signedIn(), path); err != nil {
		t.Fatal(err)
	}
	tool, _, signed := signing(t, path)
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
