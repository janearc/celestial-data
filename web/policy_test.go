package web_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/janearc/celestial-data/web"
)

// The default policy is read from this package's numbers file and says
// it is the default.
func TestTheDefaultPolicyIsReadFromItsNumbers(t *testing.T) {
	policy, err := web.DefaultPolicy("example.org")
	if err != nil {
		t.Fatal(err)
	}
	want := web.Policy{Host: "example.org", Pace: time.Second,
		Rest: 5 * time.Minute, Tries: 3, MostTries: 10, Opens: 2,
		BackoffBase: time.Second, BackoffCap: time.Minute,
		OverridesKept: 20, LockWait: 2 * time.Second, Defaulted: true}
	if policy != want {
		t.Errorf("%+v, not %+v", policy, want)
	}
}

// Anti-test: defaults that will not read, or that lack a number, or hold
// one that makes no sense, are refused, saying which number.
func TestDefaultsThatCannotBeReadAreRefused(t *testing.T) {
	good := string(readFile(t, "numbers.yaml"))
	cases := map[string]string{
		"will not read": "numbers: [",
		"a number lacking": strings.Replace(good, "default-tries",
			"default-trys", 1),
		"a count that is not": strings.Replace(good,
			"value: 3\n    unit: attempts",
			"value: 1.5\n    unit: attempts", 1),
		"a backoff lacking": strings.Replace(good, "backoff-cap",
			"backoff-top", 1),
		"overrides kept not whole": strings.Replace(good,
			"value: 20\n    unit: overrides",
			"value: 2.5\n    unit: overrides", 1),
		"a pace no time holds": strings.Replace(good,
			"name: default-pace\n    value: 1",
			"name: default-pace\n    value: 1e12", 1),
	}
	for name, raw := range cases {
		restore := web.SwapDefaults([]byte(raw))
		_, err := web.DefaultPolicy("example.org")
		restore()
		if err == nil {
			t.Errorf("%s: read", name)
		}
	}
	restore := web.SwapDefaults([]byte(good))
	defer restore()
	if _, err := web.DefaultPolicy("example.org"); err != nil {
		t.Errorf("the built in file: %v", err)
	}
}

// Anti-test: when the built in defaults will not read, a default policy
// and a policy from a host's numbers both say so.
func TestADefaultPolicyOfDefaultsThatWillNotReadIsRefused(t *testing.T) {
	defer web.SwapDefaults([]byte("numbers: ["))()
	if _, err := web.DefaultPolicy("example.org"); err == nil {
		t.Error("a default policy from nothing")
	}
	if _, err := web.PolicyFrom("Example", "example",
		exampleNumbers(t)); err == nil {
		t.Error("a policy from nothing")
	}
}

// A host's own numbers take the place of the defaults, and one it does not
// name is the default.
func TestAHostsNumbersTakeThePlaceOfTheDefaults(t *testing.T) {
	policy := example(t)
	if policy.Pace != 3*time.Second || policy.Tries != 2 ||
		policy.Opens != 2 || !policy.RepeatsPosts ||
		policy.Rest != 5*time.Minute || policy.Defaulted ||
		policy.Host != "Example" {
		t.Errorf("%+v", policy)
	}
	quiet := policyOf(t, "Quiet", "quiet")
	if quiet.Tries != 0 || quiet.Rest != 5*time.Minute ||
		quiet.RepeatsPosts || quiet.BackoffCap != time.Minute {
		t.Errorf("%+v", quiet)
	}
}

// said is numbers a test says in a map, as web.Numbers.
type said map[string]float64

// Get is the number of a name, or why there is none.
func (numbers said) Get(name string) (float64, error) {
	if value, ok := numbers[name]; ok {
		return value, nil
	}
	return 0, errors.New("no number " + name)
}

// A host may name no opens, which is a circuit that never opens.
func TestAHostMayNameNoOpens(t *testing.T) {
	policy, err := web.PolicyFrom("Open", "open",
		said{"open-pace": 1, "open-opens": 0})
	if err != nil || policy.Opens != 0 {
		t.Errorf("%+v, %v", policy, err)
	}
}

// Anti-test: a host that names a negative number of opens is refused,
// saying which number.
func TestAHostMayNotNameNegativeOpens(t *testing.T) {
	_, err := web.PolicyFrom("Open", "open",
		said{"open-pace": 1, "open-opens": -1})
	if err == nil || !strings.Contains(err.Error(), "open-opens") {
		t.Errorf("%v", err)
	}
}

// Anti-test: a host with no pace is refused, and so is a number that is
// not a count or not yes or no, each saying which.
func TestAHostWhoseNumbersAreWrongIsRefusedSayingWhich(t *testing.T) {
	_, err := web.PolicyFrom("Nowhere", "nowhere", exampleNumbers(t))
	if err == nil || !strings.Contains(err.Error(), "Nowhere has no pace") {
		t.Errorf("no pace: %v", err)
	}
	_, err = web.PolicyFrom("Tied", "tied", exampleNumbers(t))
	if err == nil || !strings.Contains(err.Error(), "tied-tries") ||
		!strings.Contains(err.Error(), "tied-repeats-posts") {
		t.Errorf("wrong numbers: %v", err)
	}
}

// A gentle's policy is checked, and a pace quicker than the host's least
// is refused, with the least it allows.
func TestAPolicyThatMakesNoSenseIsRefused(t *testing.T) {
	good := example(t)
	cases := map[string]func(*web.Policy){
		"no host":         func(p *web.Policy) { p.Host = "" },
		"no pace":         func(p *web.Policy) { p.Pace = 0 },
		"a negative rest": func(p *web.Policy) { p.Rest = -1 },
		"negative tries":  func(p *web.Policy) { p.Tries = -1 },
		"negative opens":  func(p *web.Policy) { p.Opens = -1 },
		"negative kept":   func(p *web.Policy) { p.OverridesKept = -1 },
		"no backoff base": func(p *web.Policy) { p.BackoffBase = 0 },
		"a cap under base": func(p *web.Policy) {
			p.BackoffCap = p.BackoffBase - 1
		},
		"a quick pace": func(p *web.Policy) {
			p.LeastPace = p.Pace + time.Second
		},
	}
	clock, _ := clockAt(t, start)
	for name, spoil := range cases {
		policy := good
		spoil(&policy)
		_, err := web.NewGentle(policy, clock, randomOf(t, half))
		if err == nil {
			t.Errorf("%s: made", name)
		}
		if name == "a quick pace" && !strings.Contains(err.Error(),
			"quicker than Example allows: no more than one request "+
				"every 4s") {
			t.Errorf("%v", err)
		}
	}
}

// Anti-test: a gentle with no clock or no source of randomness is
// refused, and a policy at the least pace is not.
func TestAGentleWithoutAClockOrRandomIsRefused(t *testing.T) {
	clock, _ := clockAt(t, start)
	if _, err := web.NewGentle(example(t), nil, randomOf(t, half)); err == nil {
		t.Error("no clock")
	}
	if _, err := web.NewGentle(example(t), clock, nil); err == nil {
		t.Error("no random")
	}
	least := example(t)
	least.LeastPace = least.Pace
	if _, err := web.NewGentle(least, clock, randomOf(t, half)); err != nil {
		t.Error(err)
	}
}

// A host sets its least pace in its numbers file, and a pace under it is
// refused.
func TestAHostSetsItsLeastPaceInItsNumbers(t *testing.T) {
	policy := policyOf(t, "Least", "least")
	if policy.LeastPace != 5*time.Second {
		t.Fatalf("%+v", policy)
	}
	clock, _ := clockAt(t, start)
	_, err := web.NewGentle(policy, clock, randomOf(t, half))
	if err == nil || !strings.Contains(err.Error(), "quicker than Least "+
		"allows: no more than one request every 5s") {
		t.Errorf("%v", err)
	}
}

// Anti-test: more tries than the most a policy may ask for are refused,
// from a numbers file or by hand; the most itself is not; and a host's
// seconds that no time holds are refused, saying which.
func TestTriesPastTheMostAndSecondsNoTimeHoldsAreRefused(t *testing.T) {
	many, err := web.PolicyFrom("Many", "many", exampleNumbers(t))
	if err != nil {
		t.Fatal(err)
	}
	clock, _ := clockAt(t, start)
	if _, err := web.NewGentle(many, clock, randomOf(t, half)); err == nil ||
		!strings.Contains(err.Error(), "11 tries for Many is more than "+
			"the 10 allowed") {
		t.Errorf("%v", err)
	}
	many.Tries = many.MostTries
	if _, err := web.NewGentle(many, clock, randomOf(t, half)); err != nil {
		t.Errorf("the most: %v", err)
	}
	_, err = web.PolicyFrom("Huge", "huge", exampleNumbers(t))
	if err == nil || !strings.Contains(err.Error(), "huge-pace") {
		t.Errorf("%v", err)
	}
}
