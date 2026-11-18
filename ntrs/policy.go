package ntrs

import (
	_ "embed"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
)

// numbersFile is the numbers file that holds how NTRS is asked.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// asks is how NTRS is asked, read from numbers.yaml.
var asks = celestialdata.NewMeasured(numbersFile, numbersRaw)

// Host is NTRS's host, as the web package names a host: in lower case,
// with no dot at its end. A search, a citation and a PDF are all asked
// of it.
const Host = "ntrs.nasa.gov"

// policyKey is what the numbers of NTRS's policy are named after.
const policyKey = "ntrs"

// Policy is how NTRS is asked: the pace, the rest, the tries and the
// opens its numbers say, and the rest the web package's default.
//
// A search is a POST, and the policy says it may be sent again after a
// failure with no word from NTRS, because it only asks a question. See
// ntrs-repeats-posts in numbers.yaml for why.
//
// A PDF is a large answer, so the gate is held until its body is closed:
// a slow download is still the only request to NTRS. That is this
// package's code saying so, as arXiv's does.
func Policy() web.Policy {
	// the numbers are this package's own, which its tests read, so they
	// always read
	made, _ := web.PolicyFrom("NTRS", policyKey, asks)
	made.HoldsUntilClosed = true
	return made
}

// AnswerLimit is the most bytes the answer to a search or a lookup may
// hold, as ntrs-answer-most says.
func AnswerLimit() int64 { return int64(asks.Value("ntrs-answer-most")) }
