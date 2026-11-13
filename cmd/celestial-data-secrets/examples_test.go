package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/secrets"
	"go.uber.org/mock/gomock"
)

// stopping is what a mock in an example reports to. Examples have no
// test to fail, so it stops the example with what went wrong.
type stopping struct{}

// Errorf stops the example with what went wrong.
func (stopping) Errorf(format string, args ...any) {
	panic(fmt.Sprintf(format, args...))
}

// Fatalf stops the example with what went wrong.
func (stopping) Fatalf(format string, args ...any) {
	panic(fmt.Sprintf(format, args...))
}

// exampleHome is a home for an example whose config holds these lines,
// in a folder made for it, which clean removes.
type exampleHome struct{ path string }

// newExampleHome makes the home and its config.
func newExampleHome(lines ...string) exampleHome {
	home := must(os.MkdirTemp("", "home"))
	path := secrets.Path(home)
	folder := filepath.Dir(path)
	must(nothing, os.MkdirAll(folder, configFolder))
	text := []byte(strings.Join(lines, "\n") + "\n")
	must(nothing, os.WriteFile(path, text, configMode))
	return exampleHome{path: home}
}

// clean removes the home.
func (home exampleHome) clean() { must(nothing, os.RemoveAll(home.path)) }

// nothing is no value, for must of a call that gives only an error.
var nothing struct{}

// must is the value made, or stops the example with its error.
func must[Made any](made Made, err error) Made {
	if err != nil {
		panic(err)
	}
	return made
}

// said runs celestial-data-secrets over a home, reading through keys,
// and gives what it said. It stops the example when the run fails.
func said(home string, keys *mocks.MockReader, words ...string) string {
	env := environment{home: home}
	tool := command{keys: keys, lookup: env.lookup,
		picked: celestialdata.NewMeasured(numbersFile, numbersRaw)}
	var out, errs bytes.Buffer
	if code := tool.run(words, strings.NewReader(""), &out,
		&errs); code != exitOK {
		panic(errs.String())
	}
	return out.String()
}

// Example_list lists the services, here with a home that has no config:
// neither is placed, and each says the line that would place it.
func Example_list() {
	fmt.Print(said("a-home", nil, "list"))
	// Output:
	// the config: a-home/.config/celestial-data-secrets/config
	// ads: an API token for NASA's Astrophysics Data System; its fields: credential
	//   not placed: add the line "secret ads :: op://vault/item"
	// gaia: a login to the Gaia archive at ESA; its fields: username, password
	//   not placed: add the line "secret gaia :: op://vault/item"
}

// Example_check reads each secret through op, here a mock, and says
// only that each was read.
func Example_check() {
	home := newExampleHome(both...)
	defer home.clean()
	keys := mocks.NewMockReader(gomock.NewController(stopping{}))
	read(keys)
	fmt.Print(said(home.path, keys, "check"))
	// Output:
	// ads: read
	// gaia: read
}

// Example_gaiaLogin says what gaia-login would do, and reads nothing.
func Example_gaiaLogin() {
	home := newExampleHome(both...)
	defer home.clean()
	fmt.Print(said(home.path, nil, "gaia-login", "--session",
		"gaia.session", "--dry-run"))
	// Output:
	// would read gaia's username and password from op://Vault/gaia login with op, sign in to https://gea.esac.esa.int/tap-server, and keep the session in gaia.session
}

// Example_adsLogin says what ads-login would do, and reads nothing.
func Example_adsLogin() {
	home := newExampleHome(both...)
	defer home.clean()
	fmt.Print(said(home.path, nil, "ads-login", "--session",
		"ads.token", "--dry-run"))
	// Output:
	// would read the ads token from op://Vault/ads token with op, and keep it in ads.token, readable by you alone
}

// Every verb has its example, and pzm holds each to what it prints.
func TestEveryVerbHasItsExample(t *testing.T) {
	var tool command
	if err := tool.declared().Examples(os.DirFS(".")); err != nil {
		t.Error(err)
	}
}
