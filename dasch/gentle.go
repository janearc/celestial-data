package dasch

import (
	_ "embed"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
)

// numbersFile is the numbers file that holds how Starglass is asked.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// asks is how Starglass is asked, read from numbers.yaml.
var asks = celestialdata.NewMeasured(numbersFile, numbersRaw)

// Host is the host the questions go to, in lower case with no dot at its
// end, as the web package names a host. It is root's host; a test holds
// the two together.
const Host = "api.starglass.cfa.harvard.edu"

// hostName is the archive as a person says it, in every message about
// the host.
const hostName = "DASCH"

// Policy is how Starglass is asked: this package's own numbers, and the
// web package's defaults for the rest. Hand it to web.NewHosts under
// Host, and every request to Starglass goes at that pace, stops when
// Starglass asks it to, and is retried when Starglass says nothing.
func Policy() (web.Policy, error) {
	return policyFrom(asks)
}

// policyFrom is a policy read from numbers, so a test may give it numbers
// that are wrong.
func policyFrom(kept web.Numbers) (web.Policy, error) {
	return web.PolicyFrom(hostName, "dasch", kept)
}
