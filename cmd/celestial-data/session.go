package main

import (
	"net/http"

	"github.com/janearc/celestial-data/web"
)

//go:generate -command mock go tool mockgen -package=main
//go:generate mock -source=session.go -destination=session_mock_test.go

// gaiaSession is the variable that names where the gaia archive's
// session is kept: celestial-data-secrets gaia-login writes it, and
// fetch sends it.
const gaiaSession = "GAIA_SESSION"

// session is a web that keeps the cookies its answers set, and can be
// given them and asked for them.
type session interface {
	web.Web
	Cookies(address string) ([]*http.Cookie, error)
	Keep(address string, cookies []*http.Cookie) error
}
