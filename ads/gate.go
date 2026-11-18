package ads

import (
	_ "embed"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
)

// numbersFile is the numbers file that holds how ADS is asked.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// asks is how ADS is asked, read from numbers.yaml.
var asks = celestialdata.NewMeasured(numbersFile, numbersRaw)

// host is ADS as a person says it, in every message about it.
const host = "ADS"

// key is what ADS's numbers are named from, as ads-pace.
const key = "ads"

// Policy is how ADS is asked: its own numbers, and the web package's
// default for any it does not name.
//
// ADS limits requests by the day and publishes no pace, so the pace is
// one we picked. A request is never asked again after a failure with no
// word, since ADS counts each one against the day, and a search is a GET
// with no body, so no POST is repeated either.
func Policy() (web.Policy, error) { return policyFrom(asks) }

// policyFrom is a policy read from numbers named as ADS's are.
func policyFrom(kept web.Numbers) (web.Policy, error) {
	return web.PolicyFrom(host, key, kept)
}

// NewGentle is the gentle that asks ADS, on a clock, with a source of
// randomness for the jitter a policy with tries would use. Wrap the Doer
// of a search in web.Through with it, and ADS's stops and paces are the
// web package's, so nothing here paces or reads a Retry-After.
func NewGentle(clock web.Clock, random web.Random) (*web.Gentle, error) {
	return gentleFrom(asks, clock, random)
}

// gentleFrom is a gentle for the policy numbers give.
func gentleFrom(kept web.Numbers, clock web.Clock,
	random web.Random) (*web.Gentle, error) {
	policy, err := policyFrom(kept)
	if err != nil {
		return nil, err
	}
	return web.NewGentle(policy, clock, random)
}
