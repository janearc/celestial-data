package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/ads"
	"github.com/janearc/celestial-data/gaia"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/secrets"
	"go.uber.org/mock/gomock"
)

// The items the tests' configs place the secrets in.
const (
	adsItem  = "op://Vault/ads token"
	gaiaItem = "op://Vault/gaia login"
)

// What op gives the tests: the token, and the login. The password is
// plain enough to find in anything the command says.
const (
	token    = "tok-not-a-real-token"
	user     = "ada"
	password = "hunter2"
)

// configMode is the mode a test's config is written with. It's defined:
// read and written by its owner alone.
const configMode = 0o600

// configFolder is the mode of the folders above it. It's defined: read,
// written and entered by its owner alone.
const configFolder = 0o700

// homeWith is a home whose config holds these lines, and nothing when
// there are none.
func homeWith(t *testing.T, lines ...string) string {
	t.Helper()
	home := t.TempDir()
	if len(lines) == noneNamed {
		return home
	}
	path := secrets.Path(home)
	if err := os.MkdirAll(filepath.Dir(path), configFolder); err != nil {
		t.Fatal(err)
	}
	text := []byte(strings.Join(lines, "\n") + "\n")
	if err := os.WriteFile(path, text, configMode); err != nil {
		t.Fatal(err)
	}
	return home
}

// both is a config that places both secrets.
var both = []string{"secret ads :: " + adsItem, "secret gaia :: " + gaiaItem}

// environment answers HOME, GAIA_SESSION and ADS_SESSION with what it
// holds, and any other name with nothing.
type environment struct{ home, session, token string }

// lookup is the variable's value.
func (env environment) lookup(name string) string {
	switch name {
	case "HOME":
		return env.home
	case gaiaSession:
		return env.session
	case ads.Session:
		return env.token
	}
	return ""
}

// keeping is the command over a home and a GAIA_SESSION, with op and the
// web both mocks that take no call they are not told to expect.
func keeping(t *testing.T, env environment) (command, *mocks.MockReader,
	*Mocksession,
) {
	t.Helper()
	controller := gomock.NewController(t)
	keys, signed := mocks.NewMockReader(controller),
		NewMocksession(controller)
	tool := command{keys: keys, lookup: env.lookup, signed: signed,
		picked: celestialdata.NewMeasured(numbersFile, numbersRaw)}
	return tool, keys, signed
}

// ran runs a command with words, and gives what it said and its code.
func ran(tool command, words ...string) (string, string, int) {
	var out, errs bytes.Buffer
	code := tool.run(words, strings.NewReader(""), &out, &errs)
	return out.String(), errs.String(), code
}

// the test: list says the config, then each service, its fields and
// where its secret is kept; a service with no line says the line to add.
func TestListSaysEachServiceAndWhereItIs(t *testing.T) {
	home := homeWith(t, "secret gaia :: "+gaiaItem)
	tool, _, _ := keeping(t, environment{home: home})
	out, errs, code := ran(tool, "list")
	want := "the config: " + secrets.Path(home) + "\n" +
		"ads: an API token for NASA's Astrophysics Data System; its " +
		"fields: credential\n" +
		"  not placed: add the line \"secret ads :: op://vault/item\"\n" +
		"gaia: a login to the Gaia archive at ESA; its fields: " +
		"username, password\n" +
		"  kept in " + gaiaItem + "\n"
	if code != exitOK || out != want {
		t.Errorf("exit %d: %q%s", code, out, errs)
	}
}

// the anti-test: list asks op nothing, though both secrets are placed:
// op is a mock told to expect no call, which fails the test on one.
func TestListAsksOpNothing(t *testing.T) {
	tool, _, _ := keeping(t, environment{home: homeWith(t, both...)})
	out, errs, code := ran(tool, "list")
	if code != exitOK || strings.Count(out, "kept in ") != len(both) {
		t.Errorf("exit %d: %s%s", code, out, errs)
	}
}

// the anti-test: with no home, each verb is refused before op or the
// archive is asked, saying why.
func TestNoHomeIsRefused(t *testing.T) {
	tool, _, _ := keeping(t, environment{session: "s"})
	for _, verb := range []string{"list", "check", "gaia-login"} {
		_, errs, code := ran(tool, verb)
		if code == exitOK || !strings.Contains(errs, "HOME names none") {
			t.Errorf("%s: exit %d: %s", verb, code, errs)
		}
	}
}

// the anti-test: a config with a wrong line is refused by every verb,
// naming its line.
func TestAWrongConfigIsRefused(t *testing.T) {
	home := homeWith(t, "secret ads :: op://Vault")
	tool, _, _ := keeping(t, environment{home: home, session: "s"})
	for _, verb := range []string{"list", "check", "gaia-login"} {
		_, errs, code := ran(tool, verb)
		if code == exitOK || !strings.Contains(errs, ":1: secret: ") {
			t.Errorf("%s: exit %d: %s", verb, code, errs)
		}
	}
}

// read is op giving each secret, in the order check asks.
func read(keys *mocks.MockReader) {
	gomock.InOrder(
		keys.EXPECT().Read(adsItem+"/credential").Return(token, nil),
		keys.EXPECT().Read(gaiaItem+"/username").Return(user, nil),
		keys.EXPECT().Read(gaiaItem+"/password").Return(password, nil))
}

// the test: check reads every secret and says each was read.
func TestCheckReadsEverySecret(t *testing.T) {
	tool, keys, _ := keeping(t, environment{home: homeWith(t, both...)})
	read(keys)
	out, errs, code := ran(tool, "check")
	if code != exitOK || out != "ads: read\ngaia: read\n" {
		t.Errorf("exit %d: %q%s", code, out, errs)
	}
}

// the anti-test: check says no secret, whatever op gives.
func TestCheckSaysNoSecret(t *testing.T) {
	tool, keys, _ := keeping(t, environment{home: homeWith(t, both...)})
	read(keys)
	out, errs, _ := ran(tool, "check")
	for _, secret := range []string{token, user, password} {
		if strings.Contains(out+errs, secret) {
			t.Errorf("check said %q", secret)
		}
	}
}

// the test: check reads only the secrets named.
func TestCheckReadsOnlyWhatIsNamed(t *testing.T) {
	tool, keys, _ := keeping(t, environment{home: homeWith(t, both...)})
	keys.EXPECT().Read(adsItem+"/credential").Return(token, nil)
	out, errs, code := ran(tool, "check", "ads")
	if code != exitOK || out != "ads: read\n" {
		t.Errorf("exit %d: %q%s", code, out, errs)
	}
}

// the anti-test: a secret op will not give, one the config does not
// place, and a service there is not, are each said not read, why, and
// fail the check; the others are still read.
func TestCheckSaysWhatWasNotRead(t *testing.T) {
	home := homeWith(t, "secret gaia :: "+gaiaItem)
	tool, keys, _ := keeping(t, environment{home: home})
	keys.EXPECT().Read(gaiaItem+"/username").Return(user, nil)
	keys.EXPECT().Read(gaiaItem+"/password").Return("",
		errors.New("locked"))
	out, errs, code := ran(tool, "check", "ads", "gaia", "hubble")
	unread := []string{
		"ads: not read: secrets: the config gives no place",
		"gaia: not read: secrets: gaia's password: locked",
		`hubble: not read: secrets: no such service: "hubble"`,
	}
	for _, said := range unread {
		if !strings.Contains(out, said) {
			t.Errorf("did not say %q: %s", said, out)
		}
	}
	if code == exitOK || !strings.Contains(errs, "3 of 3 secrets were "+
		"not read") {
		t.Errorf("exit %d: %s", code, errs)
	}
}

// signedIn is the cookies the archive sets a user who signs in.
func signedIn() []*http.Cookie {
	return []*http.Cookie{{Name: "JSESSIONID", Value: "s1"}}
}

// login is op giving the login and the archive taking it.
func login(keys *mocks.MockReader, signed *Mocksession) {
	keys.EXPECT().Read(gaiaItem+"/username").Return(user, nil)
	keys.EXPECT().Read(gaiaItem+"/password").Return(password, nil)
	signed.EXPECT().Post("https://gea.esac.esa.int/tap-server/login",
		url.Values{"username": {user}, "password": {password}}).Return("",
		nil)
}

// the test: gaia-login signs in with the login the config places, keeps
// the session where --session says, and says where.
func TestGaiaLoginKeepsTheSessionWhereItIsTold(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session")
	tool, keys, signed := keeping(t, environment{home: homeWith(t,
		both...)})
	login(keys, signed)
	signed.EXPECT().Cookies(gaia.Service).Return(signedIn(), nil)
	out, errs, code := ran(tool, "gaia-login", "--session", path)
	if code != exitOK || !strings.Contains(out, "the session is in "+
		path) {
		t.Fatalf("exit %d: %s%s", code, out, errs)
	}
	if _, err := gaia.Load(path); err != nil {
		t.Fatal(err)
	}
}

// the anti-test: with no --session, gaia-login keeps the session where
// GAIA_SESSION says.
func TestGaiaLoginKeepsTheSessionWhereGaiaSessionSays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session")
	tool, keys, signed := keeping(t, environment{home: homeWith(t,
		both...), session: path})
	login(keys, signed)
	signed.EXPECT().Cookies(gaia.Service).Return(signedIn(), nil)
	if _, errs, code := ran(tool, "gaia-login"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errs)
	}
	if _, err := gaia.Load(path); err != nil {
		t.Fatal(err)
	}
}

// the anti-test: gaia-login with nowhere to keep the session, or with no
// login placed, is refused before op or the archive is asked.
func TestGaiaLoginWithoutAPlaceOrALoginIsRefused(t *testing.T) {
	for says, home := range map[string]string{
		"needs --session": homeWith(t, both...),
		"secret gaia :: ": homeWith(t, "secret ads :: "+adsItem),
	} {
		tool, _, _ := keeping(t, environment{home: home})
		_, errs, code := ran(tool, "gaia-login")
		if code == exitOK || !strings.Contains(errs, says) {
			t.Errorf("exit %d: %s", code, errs)
		}
	}
}

// the anti-test: a dry run says what gaia-login would do, and asks op
// and the archive nothing.
func TestGaiaLoginDryRunAsksNothing(t *testing.T) {
	tool, _, _ := keeping(t, environment{home: homeWith(t, both...)})
	out, errs, code := ran(tool, "gaia-login", "--session", "s",
		"--dry-run")
	if code != exitOK || !strings.Contains(out, "would read gaia's "+
		"username and password from "+gaiaItem) {
		t.Errorf("exit %d: %s%s", code, out, errs)
	}
}

// the anti-test: when op refuses the username or the password,
// gaia-login stops, says what op said, and asks the archive nothing.
func TestGaiaLoginStopsWhenOpSaysNo(t *testing.T) {
	for _, field := range []string{"username", "password"} {
		tool, keys, _ := keeping(t, environment{home: homeWith(t,
			both...)})
		if field == "password" {
			keys.EXPECT().Read(gaiaItem+"/username").Return(user, nil)
		}
		keys.EXPECT().Read(gaiaItem+"/"+field).Return("",
			errors.New("locked"))
		path := filepath.Join(t.TempDir(), "s")
		_, errs, code := ran(tool, "gaia-login", "--session", path)
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
		tool, keys, signed := keeping(t, environment{home: homeWith(t,
			both...)})
		keys.EXPECT().Read(gaiaItem+"/username").Return(user, nil)
		keys.EXPECT().Read(gaiaItem+"/password").Return(password, nil)
		posted := signed.EXPECT().Post(gomock.Any(), gomock.Any())
		says := map[string]string{"refused": "401 Unauthorized",
			"cookieless": "set no", "unheld": "no jar"}[why]
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
		_, errs, code := ran(tool, "gaia-login", "--session", path)
		_, err := os.Stat(path)
		if code == exitOK || err == nil || !strings.Contains(errs, says) {
			t.Errorf("%s: exit %d, kept %v: %s", why, code, err, errs)
		}
		if strings.Contains(errs, password) {
			t.Errorf("%s: said the password: %s", why, errs)
		}
	}
}

// the test: with no web given, the command signs in through the
// network's, with the wait its numbers pick.
func TestTheWebIsTheNetworksWithTheWait(t *testing.T) {
	tool := command{picked: celestialdata.NewMeasured(numbersFile,
		numbersRaw)}
	if signed, err := tool.client(); err != nil || signed == nil {
		t.Errorf("%v, %v", signed, err)
	}
}

// the anti-test: with no wait number to ask the archive with,
// gaia-login is refused after reading the login, and keeps nothing.
func TestGaiaLoginWithNoWaitIsRefused(t *testing.T) {
	keys := mocks.NewMockReader(gomock.NewController(t))
	keys.EXPECT().Read(gaiaItem+"/username").Return(user, nil)
	keys.EXPECT().Read(gaiaItem+"/password").Return(password, nil)
	env := environment{home: homeWith(t, both...)}
	tool := command{keys: keys, lookup: env.lookup,
		picked: celestialdata.NewMeasured("test.yaml",
			[]byte("numbers:\n"))}
	path := filepath.Join(t.TempDir(), "s")
	_, errs, code := ran(tool, "gaia-login", "--session", path)
	if _, err := os.Stat(path); code == exitOK || err == nil ||
		!strings.Contains(errs, "wait") {
		t.Errorf("exit %d, kept %v: %s", code, err, errs)
	}
}

// the test: ads-login reads the token once, keeps it where --session
// says, readable by its owner alone, and says where.
func TestADSLoginKeepsTheTokenWhereItIsTold(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ads-token")
	tool, keys, _ := keeping(t, environment{home: homeWith(t, both...)})
	keys.EXPECT().Read(adsItem+"/credential").Return(token, nil)
	out, errs, code := ran(tool, "ads-login", "--session", path)
	kept, err := ads.Load(path)
	if code != exitOK || err != nil || kept != token ||
		!strings.Contains(out, "the ads token is kept in "+path) {
		t.Errorf("exit %d, %q, %v: %s%s", code, kept, err, out, errs)
	}
}

// the anti-test: with no --session, ads-login keeps the token where
// ADS_SESSION says, and what it says never holds the token.
func TestADSLoginKeepsTheTokenWhereADSSessionSays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ads-token")
	tool, keys, _ := keeping(t, environment{home: homeWith(t, both...),
		token: path})
	keys.EXPECT().Read(adsItem+"/credential").Return(token, nil)
	out, errs, code := ran(tool, "ads-login")
	if kept, err := ads.Load(path); code != exitOK || err != nil ||
		kept != token || strings.Contains(out+errs, token) {
		t.Errorf("exit %d, %q, %v: %s%s", code, kept, err, out, errs)
	}
}

// the test: with no --session and no ADS_SESSION, ads-login keeps the
// token in ads-token beside the config.
func TestADSLoginKeepsTheTokenBesideTheConfig(t *testing.T) {
	home := homeWith(t, both...)
	tool, keys, _ := keeping(t, environment{home: home})
	keys.EXPECT().Read(adsItem+"/credential").Return(token, nil)
	_, errs, code := ran(tool, "ads-login")
	path := secrets.Beside(home, "ads-token")
	if kept, err := ads.Load(path); code != exitOK || err != nil ||
		kept != token {
		t.Errorf("exit %d, %q, %v: %s", code, kept, err, errs)
	}
}

// the anti-test: ads-login with no token placed is refused before op is
// asked, and keeps nothing beside the config.
func TestADSLoginWithoutATokenPlacedIsRefused(t *testing.T) {
	home := homeWith(t, "secret gaia :: "+gaiaItem)
	tool, _, _ := keeping(t, environment{home: home})
	_, errs, code := ran(tool, "ads-login")
	_, err := os.Stat(secrets.Beside(home, "ads-token"))
	if code == exitOK || err == nil ||
		!strings.Contains(errs, "secret ads :: ") {
		t.Errorf("exit %d, kept %v: %s", code, err, errs)
	}
}

// the anti-test: when op will not give the token, ads-login says what
// op said, and keeps nothing.
func TestADSLoginKeepsNothingOpWillNotGive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ads-token")
	tool, keys, _ := keeping(t, environment{home: homeWith(t, both...)})
	keys.EXPECT().Read(adsItem+"/credential").Return("",
		errors.New("locked"))
	_, errs, code := ran(tool, "ads-login", "--session", path)
	if _, err := os.Stat(path); code == exitOK || err == nil ||
		!strings.Contains(errs, "locked") {
		t.Errorf("exit %d, kept %v: %s", code, err, errs)
	}
}

// the anti-test: a token that will not keep, over a file others can
// read, is refused, and the file is left as it was.
func TestADSLoginKeepsNoTokenOthersCouldRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ads-token")
	if err := os.WriteFile(path, []byte("before\n"), readable); err != nil {
		t.Fatal(err)
	}
	tool, keys, _ := keeping(t, environment{home: homeWith(t, both...)})
	keys.EXPECT().Read(adsItem+"/credential").Return(token, nil)
	_, errs, code := ran(tool, "ads-login", "--session", path)
	left, _ := os.ReadFile(path)
	if code == exitOK || string(left) != "before\n" ||
		strings.Contains(errs, token) {
		t.Errorf("exit %d, %q: %s", code, left, errs)
	}
}

// readable is a mode anyone can read. It's defined: read by anyone,
// written by its owner.
const readable = 0o644
