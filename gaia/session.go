package gaia

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
)

// sessionCookie is the cookie the archive knows a signed-in user by.
const sessionCookie = "JSESSIONID"

// onlyItsOwner is the mode of a saved session. Defined: read and written
// by the user who saved it, and by no one else.
const onlyItsOwner = 0o600

// isTheSession is whether a cookie is the archive's session.
func isTheSession(cookie *http.Cookie) bool {
	return cookie.Name == sessionCookie
}

// signedIn is whether cookies hold the archive's session.
func signedIn(cookies []*http.Cookie) bool {
	return slices.ContainsFunc(cookies, isTheSession)
}

// Save writes a session's cookies to path, readable by its owner alone,
// over a session saved there before. Whoever holds them is the user who
// signed in, until they expire. It refuses cookies with no session, and
// a file anyone else can read.
func Save(cookies []*http.Cookie, path string) error {
	if !signedIn(cookies) {
		return fmt.Errorf("the archive set no %s", sessionCookie)
	}
	text, err := json.Marshal(cookies)
	if err != nil {
		return fmt.Errorf("the session: %w", err)
	}
	about, err := os.Stat(path)
	if err == nil && about.Mode().Perm() != onlyItsOwner {
		return fmt.Errorf("%s is %v; a session is %v", path,
			about.Mode().Perm(), os.FileMode(onlyItsOwner))
	}
	return os.WriteFile(path, text, onlyItsOwner)
}

// Load reads the cookies Save wrote to path. It refuses a file anyone but
// its owner can read, and one with no session in it.
func Load(path string) ([]*http.Cookie, error) {
	about, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if about.Mode().Perm() != onlyItsOwner {
		return nil, fmt.Errorf("%s is %v; a session is %v", path,
			about.Mode().Perm(), os.FileMode(onlyItsOwner))
	}
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cookies []*http.Cookie
	if err := json.Unmarshal(text, &cookies); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if !signedIn(cookies) {
		return nil, fmt.Errorf("%s has no %s", path, sessionCookie)
	}
	return cookies, nil
}
