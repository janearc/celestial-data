package main

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/gaia"
	"github.com/janearc/celestial-data/web"
	"github.com/janearc/puzzlemouth/pzm"
	"github.com/janearc/puzzlemouth/verb"
)

//go:generate -command mock go tool mockgen -package=main
//go:generate mock -source=login.go -destination=login_mock_test.go

// gaiaSession is the variable that names where the gaia archive's
// session is kept: gaia-login writes it, and fetch sends it.
const gaiaSession = "GAIA_SESSION"

// session is a web that keeps the cookies its answers set, and can be
// given them and asked for them.
type session interface {
	web.Web
	Cookies(address string) ([]*http.Cookie, error)
	Keep(address string, cookies []*http.Cookie) error
}

// secrets reads a secret by its op:// reference.
type secrets interface {
	Read(reference string) (string, error)
}

// onePassword reads secrets with the 1Password command, op, found at
// command. Each read may ask the person at the keyboard to allow it.
type onePassword struct {
	command string
}

// Read runs op read for reference and gives what it printed, without
// the end of the line.
func (keys onePassword) Read(reference string) (string, error) {
	printed, err := exec.Command(keys.command, "read", reference).Output()
	if err != nil {
		return "", fmt.Errorf("%s read %s: %w", keys.command, reference,
			err)
	}
	return strings.TrimRight(string(printed), "\r\n"), nil
}

// signingIn is gaia-login's facts: it reads op, which asks the person at
// the keyboard, and writes the session.
var signingIn = verb.NewFacts(verb.WritesTo("the file --session or "+
	gaiaSession+" names"), verb.Changes, verb.Outside, verb.Confirmation,
	verb.Light)

// loginVerb signs in to the gaia archive with the login op holds under
// --op, and keeps the session where --session or GAIA_SESSION says.
func (tool command) loginVerb(call pzm.Call) error {
	item := call.Value("op")
	path := valueOr(call, "session", tool.env(gaiaSession))
	if item == "" || path == "" {
		return celestialdata.Refuse("gaia-login needs --op, and " +
			"--session or " + gaiaSession)
	}
	if call.DryRun() {
		_, err := fmt.Fprintf(call.Out(), "would read %[1]s/username "+
			"and %[1]s/password with op, sign in to %[2]s, and "+
			"keep the session in %[3]s\n", item, gaia.Service,
			path)
		return err
	}
	signed, err := tool.signIn(item)
	if err != nil {
		return err
	}
	cookies, err := signed.Cookies(gaia.Service)
	if err == nil {
		err = gaia.Save(cookies, path)
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(call.Out(), "signed in to %s; the session is "+
		"in %s\n", gaia.Service, path)
	return err
}

// signIn reads item's username and password and signs in with them,
// through a web that keeps the session.
func (tool command) signIn(item string) (session, error) {
	user, err := tool.keys.Read(item + "/username")
	if err != nil {
		return nil, err
	}
	password, err := tool.keys.Read(item + "/password")
	if err != nil {
		return nil, err
	}
	signed, err := tool.client()
	if err != nil {
		return nil, err
	}
	return signed, gaia.SignIn(signed, user, password)
}

const loginPage = "Signs in to the Gaia archive with a login from " +
	"1Password, read with op, and keeps the session in the file " +
	"--session names, or GAIA_SESSION, readable by its owner alone. " +
	"op asks the person at the keyboard to allow each read, so a " +
	"person runs this. fetch sends the session when GAIA_SESSION " +
	"names it, so the archive runs the gaia archive's jobs as that " +
	"person's. --op names the login item; its username and password " +
	"fields are read."
