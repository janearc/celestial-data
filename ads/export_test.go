package ads

import "github.com/janearc/celestial-data/web"

// PolicyFrom is the policy of numbers that are not the package's own, so
// a test can give numbers that are missing or make no sense.
func PolicyFrom(kept web.Numbers) (web.Policy, error) {
	return policyFrom(kept)
}

// GentleFrom is a gentle on numbers that are not the package's own.
func GentleFrom(kept web.Numbers, clock web.Clock,
	random web.Random) (*web.Gentle, error) {
	return gentleFrom(kept, clock, random)
}
