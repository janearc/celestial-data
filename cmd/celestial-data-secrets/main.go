// Command celestial-data-secrets keeps the secrets celestial-data signs
// in with, in one place. It lists the services and where each secret is
// kept in 1Password, checks that op gives each, and signs in to the Gaia
// archive.
//
// It never says a password or a token it read. When op fails, op's own
// words are passed on as op gave them.
//
// pzm parses its words and writes its help and its errors.
package main

import (
	"os"
)

// The stamps game writes into the binary when it builds. Plain go build
// leaves them as they are here, and --age says so.
var (
	build    = "dev"
	built    = ""
	who      = ""
	sessions = ""
)

// The codes celestial-data-secrets exits with.
const (
	exitOK    = 0 // defined by the shell: success
	exitUsage = 2 // defined by pzm: nothing ran
)

// afterName is where the words after the command's name begin. It's
// defined by os.Args.
const afterName = 1

// main runs celestial-data-secrets and exits with its code.
func main() {
	os.Exit(run(os.Args[afterName:], os.Stdin, os.Stdout, os.Stderr))
}
