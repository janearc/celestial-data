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

// opRefuses is the code the pretend op exits with when it has no such
// item. Picked: any code but none would do.
const opRefuses = 1

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

// signing is celestial-data with op holding the login, and a web that
// keeps a session, both mocks, with GAIA_SESSION as given.
func signing(t *testing.T, session string) (command, *Mocksecrets,
	*Mocksession) {
	t.Helper()
	controller := gomock.NewController(t)
	keys, signed := NewMocksecrets(controller), NewMocksession(controller)
	tool := command{picked: picked(t, numbersRaw), keys: keys,
		signed: signed, lookup: func(name string) string {
			if name == gaiaSession {
				return session
			}
			return ""
		}}
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

func TestGaiaLoginWithoutAnItemOrAPlaceIsRefused(t *testing.T) {
	tool, _, _ := signing(t, "") // neither mock takes a call
	for _, words := range [][]string{{"--op", item},
		{"--session", "s"}} {
		_, _, code := ran(tool, append([]string{"gaia-login"},
			words...)...)
		if code == exitOK {
			t.Errorf("gaia-login %v ran", words)
		}
	}
}

func TestGaiaLoginStopsWhenOpSaysNo(t *testing.T) {
	for _, field := range []string{"username", "password"} {
		tool, keys, _ := signing(t, "")
		if field == "password" {
			keys.EXPECT().Read(item+"/username").Return("ada", nil)
		}
		keys.EXPECT().Read(item+"/"+field).Return("",
			errors.New("locked"))
		_, errs, code := ran(tool, "gaia-login", "--op", item,
			"--session", filepath.Join(t.TempDir(), "s"))
		if code == exitOK || !strings.Contains(errs, "locked") {
			t.Errorf("%s: exit %d: %s", field, code, errs)
		}
	}
}

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

func TestGaiaLoginWithNoWaitToAskWithIsRefused(t *testing.T) {
	keys := NewMocksecrets(gomock.NewController(t))
	keys.EXPECT().Read(item+"/username").Return("ada", nil)
	keys.EXPECT().Read(item+"/password").Return("pw", nil)
	tool := command{picked: picked(t, numbersWith(nil)), keys: keys}
	if _, _, code := ran(tool, "gaia-login", "--op", item, "--session",
		filepath.Join(t.TempDir(), "s")); code == exitOK {
		t.Fatal("gaia-login signed in with no wait")
	}
}

func TestOnePasswordReadsWhatOpPrints(t *testing.T) {
	t.Setenv(asOp, "1")
	got, err := onePassword{os.Args[0]}.Read(item + "/username")
	if err != nil || got != "ada" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestOnePasswordSaysWhenOpRefuses(t *testing.T) {
	t.Setenv(asOp, "1")
	if _, err := (onePassword{os.Args[0]}).Read(item +
		"/nope"); err == nil {
		t.Fatal("op's refusal was taken as a secret")
	}
}

func TestTheWebSendsTheSessionGaiaSessionNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session")
	if err := gaia.Save(signedIn(), path); err != nil {
		t.Fatal(err)
	}
	tool, _, signed := signing(t, path)
	signed.EXPECT().Keep(gaia.Service, gomock.Any()).DoAndReturn(
		func(_ string, cookies []*http.Cookie) error {
			if len(cookies) != 1 || cookies[0].Value != "s1" {
				t.Errorf("kept %v", cookies)
			}
			return nil
		})
	if got, err := tool.web(); err != nil || got != signed {
		t.Fatalf("got %v, %v", got, err)
	}
}

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

func TestFetchRunsTheGaiaJobAndBringsItsAnswerHome(t *testing.T) {
	get := quiet(t)
	gomock.InOrder(
		get.EXPECT().Post("https://gea.esac.esa.int/tap-server/tap/async",
			gomock.Any()).DoAndReturn(func(_ string,
			form url.Values) (string, error) {
			if form.Get("PHASE") != "run" ||
				!strings.HasSuffix(form.Get("QUERY"), gaiaWhere) {
				t.Errorf("submitted %v", form)
			}
			return job, nil
		}),
		get.EXPECT().Get(job).Return(io.NopCloser(strings.NewReader(
			"<job><phase>COMPLETED</phase></job>")), int64(-1), nil),
		get.EXPECT().Get(job+"/results/result").Return(
			io.NopCloser(strings.NewReader("SIMPLE")), int64(6), nil))
	into := t.TempDir()
	out, errs, code := ran(through(t, get), "fetch", "--archive", "gaia",
		"--ra", "266.4168371", "--dec", "-29.0078106", "--radius", "0.1",
		"--into", into)
	got, err := os.ReadFile(filepath.Join(into, "gaia",
		"gaia_source_266.4168371_-29.0078106_r0.1.fits"))
	if code != exitOK || err != nil || string(got) != "SIMPLE" {
		t.Fatalf("exit %d, %q, %v: %s", code, got, err, errs)
	}
	if !strings.Contains(out, "job "+job+" submitted") ||
		!strings.Contains(out, "job "+job+" complete") {
		t.Fatalf("fetch did not say its job: %s", out)
	}
}

func TestJobsWithoutTheirNumbersAreRefused(t *testing.T) {
	for _, given := range []map[string]string{{"job-asks": "3"},
		{"job-every": "1"}} {
		tool := command{picked: picked(t, numbersWith(given))}
		if _, err := tool.jobs(quiet(t), io.Discard); err == nil {
			t.Errorf("with %v, jobs were made", given)
		}
	}
}

func TestFetchWithoutTheJobNumbersAsksNothing(t *testing.T) {
	cut := strings.Index(string(numbersRaw), "  - name: job-every")
	if cut < 0 {
		t.Fatal("numbers.yaml has no job-every")
	}
	tool := command{get: quiet(t), picked: picked(t, numbersRaw[:cut])}
	if _, _, code := ran(tool, "fetch", "--archive", "galcen_psc",
		"--ra", "266.4168371", "--dec", "-29.0078106", "--radius", "0.1",
		"--into", t.TempDir()); code == exitOK {
		t.Fatal("fetch ran with no numbers for its jobs")
	}
}
