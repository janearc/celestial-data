package web

import (
	_ "embed"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/janearc/puzzlemouth/numbers"
)

// numbersFile is the numbers file that holds what this package asks of
// a host that has said nothing of its own.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// Numbers is where a host's numbers are kept, by name. A
// celestialdata.Measured is one.
type Numbers interface {
	Get(name string) (float64, error)
}

// Keeping is how a gentle keeps its record, when it has one.
type Keeping int

const (
	// KeepsShared is the default: the record is shared between runs. A
	// request takes the record's lock only for the moment it begins, to
	// read what any run wrote and write its own start, and again for the
	// moment a stop or an override is noted.
	//
	// Two runs asking one host at once keep one pace and one stop between
	// them, and neither is ever refused.
	KeepsShared Keeping = iota
	// KeepsForTheRun holds the record's lock from the first Hold until
	// Release, or until the run ends. A second run is refused while one
	// is asking. It is for a host whose adapter has always asked that way.
	KeepsForTheRun
)

// Policy is how one host is asked: how often, how long to wait when it
// says stop, how often to try again when it says nothing. Each of its
// numbers is kept in a numbers file, where it says whether it was picked
// or given by a source.
type Policy struct {
	// Host is the host as a person says it, such as arXiv. It is in
	// every message about the host.
	Host string
	// Pace is the least time from the start of one request to the start
	// of the next.
	Pace time.Duration
	// LeastPace, when not zero, is the quickest pace the host allows. A
	// pace under it is refused, so a mistake in a numbers file cannot
	// ask faster than the host's terms.
	LeastPace time.Duration
	// Rest is how long the host is left alone after it asked us to
	// stop and did not say how long.
	Rest time.Duration
	// Tries is how many more times a request is asked after a failure
	// with no word from the host. None means it is asked once. It is
	// never more than MostTries.
	Tries int
	// MostTries is the most Tries a policy may have. It is this
	// package's number, and a policy is refused with more.
	MostTries int
	// Opens is how many requests in a row may use up their tries before
	// the host is left alone for the rest of the run.
	//
	// Only a request that was allowed its tries and used every one
	// counts. A request that may not be asked again, a POST the host
	// does not say is safe to repeat, does not count.
	//
	// Nor does any request the host answered with a status, whatever it
	// was: an answer shows the host is talking, and starts the count
	// again.
	//
	// Zero means the host's circuit never opens: it is always asked.
	Opens int
	// BackoffBase and BackoffCap bound the wait before a retry: the
	// base for the first, doubled for each after, never over the cap.
	BackoffBase, BackoffCap time.Duration
	// OverridesKept is how many overrides the host's record keeps.
	OverridesKept int
	// Keeping is how the record is kept: shared between runs, which is
	// the default, or held for the run.
	Keeping Keeping
	// LockWait is the longest a request waits for another run to let go
	// of a shared record's lock, which it holds only for a moment. When
	// it passes, the request goes ahead on what this run knows. It is
	// not used for a record held for the run.
	LockWait time.Duration
	// RepeatsPosts is whether a request with a body may be sent again
	// after a failure. It is false unless the host's own numbers say a
	// POST to it is safe to repeat, and why.
	RepeatsPosts bool
	// HoldsUntilClosed is whether the gate is held until the body of an
	// answer is closed, so a slow download is still the only request to
	// the host. It is false unless a host's own code says so.
	//
	// When false, the gate is let go as the answer arrives, and the pace
	// is still measured from the start of each request. A host that
	// holds MUST have each body closed before it is asked again, or the
	// next request waits for it.
	HoldsUntilClosed bool
	// Defaulted is true when no numbers were picked for this host, so
	// the pace is the default. A stop says so.
	Defaulted bool
}

// setting is one number of a policy: the end of its name, and how it is
// put in a policy.
type setting struct {
	suffix string
	set    func(policy *Policy, value float64) error
}

// inSeconds is how a number of seconds is put in a duration of a policy.
func inSeconds(field func(*Policy) *time.Duration) func(*Policy,
	float64) error {
	return func(policy *Policy, value float64) (err error) {
		*field(policy), err = duration(value)
		return err
	}
}

// asCount is how a count is put in a number of a policy.
func asCount(field func(*Policy) *int) func(*Policy, float64) error {
	return func(policy *Policy, value float64) (err error) {
		*field(policy), err = whole(value)
		return err
	}
}

// asYesOrNo puts 1 or 0, and no other number, in a flag of a policy.
func asYesOrNo(policy *Policy, value float64) error {
	if value != 0 && value != 1 {
		return fmt.Errorf("%v is not 1 for yes or 0 for no", value)
	}
	policy.RepeatsPosts = value == 1
	return nil
}

// The settings that are a duration or a count in a policy, by the field
// each is put in.
var (
	pace      = inSeconds(func(p *Policy) *time.Duration { return &p.Pace })
	rest      = inSeconds(func(p *Policy) *time.Duration { return &p.Rest })
	leastPace = inSeconds(func(p *Policy) *time.Duration {
		return &p.LeastPace
	})
	tries = asCount(func(p *Policy) *int { return &p.Tries })
	opens = asCount(func(p *Policy) *int { return &p.Opens })
)

// settings are the numbers a host may set for itself. This package keeps
// a default for each, named the same after "default-". A host names them
// after its own key, as arxiv-pace.
var settings = []setting{{"pace", pace}, {"rest-unsaid", rest},
	{"tries", tries}, {"opens", opens}, {"repeats-posts", asYesOrNo}}

// hostSettings are the settings and one more that only a host sets: the
// quickest pace its terms allow. There is no default for it.
var hostSettings = append(settings[:len(settings):len(settings)],
	setting{"least-pace", leastPace})

// whole is a number that is a count: not negative, with no fraction, and
// one an int holds.
func whole(value float64) (int, error) {
	if value < 0 || value != math.Trunc(value) || value > math.MaxInt32 {
		return 0, fmt.Errorf("%v is not a count", value)
	}
	return int(value), nil
}

// duration is a number of seconds as a time, refused when it is not a
// number, is negative, or is longer than a time holds.
func duration(value float64) (time.Duration, error) {
	if math.IsNaN(value) || value < 0 || value >= float64(longestSeconds) {
		return 0, fmt.Errorf("%v is not a number of seconds a time "+
			"holds", value)
	}
	return Seconds(value), nil
}

// DefaultPolicy is the policy for a host that has none of its own, from
// this package's numbers file.
func DefaultPolicy(host string) (Policy, error) {
	policy, err := readDefaults(numbersRaw)
	policy.Host = host
	policy.Defaulted = true
	return policy, err
}

// unprefixed are the defaults that are named as they are, with no
// "default-" before: the backoff, the most tries, the overrides kept and
// how long a shared record's lock is waited for.
var unprefixed = []setting{
	{"backoff-base", inSeconds(func(p *Policy) *time.Duration {
		return &p.BackoffBase
	})},
	{"backoff-cap", inSeconds(func(p *Policy) *time.Duration {
		return &p.BackoffCap
	})},
	{"most-tries", asCount(func(p *Policy) *int { return &p.MostTries })},
	{"record-overrides-kept", asCount(func(p *Policy) *int {
		return &p.OverridesKept
	})},
	{"record-lock-wait", inSeconds(func(p *Policy) *time.Duration {
		return &p.LockWait
	})},
}

// readDefaults reads the default policy from the bytes of a numbers
// file, giving why when it will not read or a number is missing or makes
// no sense. The numbers of the settings are named with "default-" before.
func readDefaults(raw []byte) (Policy, error) {
	read, err := numbers.Read(numbersFile, raw)
	if err != nil {
		return Policy{}, err
	}
	var policy Policy
	var problems []error
	for _, one := range settings {
		problems = append(problems, apply(read, "default-"+one.suffix,
			one.set, &policy))
	}
	for _, one := range unprefixed {
		problems = append(problems, apply(read, one.suffix, one.set,
			&policy))
	}
	return policy, errors.Join(problems...)
}

// apply puts the number of a name, from numbers read, in a policy, and
// says which number it was when it is missing or makes no sense.
func apply(read numbers.Numbers, name string,
	set func(*Policy, float64) error, policy *Policy) error {
	number, err := read.Get(name)
	if err == nil {
		err = set(policy, number.Value)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// PolicyFrom is the policy for a host, read from the numbers it names
// with its key and a dash before, as arxiv-pace. Its pace must be there.
// A number it does not name is this package's default.
//
// It may also name its least pace, as arxiv-least-pace. A number it names
// that is not a number the policy can use is refused, saying which.
func PolicyFrom(host, key string, kept Numbers) (Policy, error) {
	policy, err := DefaultPolicy(host)
	if err != nil {
		return Policy{}, err
	}
	policy.Defaulted = false
	if _, err := kept.Get(key + "-pace"); err != nil {
		return Policy{}, fmt.Errorf("%s has no pace: %w", host, err)
	}
	var problems []error
	for _, one := range hostSettings {
		name := key + "-" + one.suffix
		value, err := kept.Get(name)
		if err != nil {
			continue // not named: the default stands
		}
		if err := one.set(&policy, value); err != nil {
			problems = append(problems,
				fmt.Errorf("%s: %w", name, err))
		}
	}
	return policy, errors.Join(problems...)
}

// check refuses a policy that cannot be asked by: no host to name, a
// pace under the host's least, more tries than may be, or a number that
// does not make sense, such as a negative count. No opens is a circuit
// that never opens, and is allowed.
func (policy Policy) check() error {
	switch {
	case policy.Host == "":
		return errors.New("a policy needs a host's name")
	case policy.Pace <= noWait:
		return fmt.Errorf("a pace of %v for %s is no pace at all",
			policy.Pace, policy.Host)
	case policy.Pace < policy.LeastPace:
		return fmt.Errorf("a pace of %v is quicker than %s allows: no "+
			"more than one request every %v", policy.Pace,
			policy.Host, policy.LeastPace)
	case policy.Tries > policy.MostTries:
		return fmt.Errorf("%d tries for %s is more than the %d "+
			"allowed", policy.Tries, policy.Host, policy.MostTries)
	case policy.Rest < noWait || policy.Tries < 0 || policy.Opens < 0 ||
		policy.OverridesKept < 0 || policy.LockWait < noWait:
		return fmt.Errorf("the rest, tries, opens, overrides kept or "+
			"lock wait for %s are not numbers that make sense",
			policy.Host)
	case policy.BackoffBase <= noWait ||
		policy.BackoffCap < policy.BackoffBase:
		return fmt.Errorf("the backoff for %s is not a base above "+
			"nothing and a cap no less", policy.Host)
	}
	return nil
}
