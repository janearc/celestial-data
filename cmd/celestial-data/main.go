// Command celestial-data finds and fetches what astronomy archives hold near a
// place on the sky. It names each archive's files for a circle. It brings
// them home and never replaces a file already there. pzm parses its words
// and writes its help and its errors.
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

// The codes celestial-data exits with.
const (
	exitOK     = 0 // defined by the shell: success
	exitFailed = 1 // defined by pzm: a verb failed
	exitUsage  = 2 // defined by pzm: nothing ran
)

// afterName is where the words after the command's name begin. It's
// defined by os.Args.
const afterName = 1

// main runs celestial-data and exits with its code.
func main() {
	os.Exit(run(os.Args[afterName:], os.Stdin, os.Stdout, os.Stderr))
}
