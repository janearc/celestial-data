package arxiv

import (
	"context"
	_ "embed"
	"io"
	"time"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
)

// numbersFile is the numbers file that holds what arXiv asks.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// asks is what arXiv asks of a program, read from numbers.yaml.
var asks = celestialdata.NewMeasured(numbersFile, numbersRaw)

// host is arXiv as a person says it, in every message about it.
const host = "arXiv"

// Clock is the time a gate keeps. It is the web package's, so a clock
// made for either is a clock for both.
type Clock = web.Clock

// Wall is the clock on the wall.
func Wall() Clock { return web.Wall() }

// Gate lets one request at a time through to arXiv, each at least its
// pace after the one before began, and none while arXiv has asked to be
// left alone. It keeps the web package's gentle, asked for arXiv by its
// numbers. The gentle is not exported: a caller asks through the gate.
type Gate struct {
	gentle *web.Gentle
}

// leastPace is the least time from one request's start to the next's.
// It is three seconds, as arXiv's API terms ask.
func leastPace() time.Duration {
	return web.Seconds(asks.Value("least-pace"))
}

// policy is how arXiv is asked at a pace: arXiv's own numbers, and the
// pace a caller picked. The rest of a policy is the web package's
// default.
//
// arXiv is asked once and no more: a request that fails with no word
// from arXiv is not asked again. That is what this adapter has always
// done, and arXiv's numbers keep it so on purpose, as tries. Nor is arXiv
// left alone for the run after failures with no word, as opens says.
func policy(pace time.Duration) web.Policy {
	// the defaults are the web package's own numbers, which its tests
	// read, so they always read
	made, _ := web.DefaultPolicy(host)
	made.Defaulted = false
	made.Pace = pace
	made.LeastPace = web.Seconds(asks.Value("least-pace"))
	made.Rest = web.Seconds(asks.Value("rest-unsaid"))
	made.Tries = int(asks.Value("tries"))
	made.Opens = int(asks.Value("opens"))
	// a download is still the only request until it is closed
	made.HoldsUntilClosed = true
	// the record is held for the run, as arXiv's always was: a second run
	// is refused while one is asking
	made.Keeping = web.KeepsForTheRun
	return made
}

// NewGate is a gate at a pace, on a clock. It refuses a pace quicker
// than arXiv's least, so a mistake in a numbers file cannot ask faster
// than arXiv allows. It refuses a missing clock too.
func NewGate(pace time.Duration, clock Clock) (*Gate, error) {
	if pace < leastPace() {
		return nil, celestialdata.Refuse("a pace of %v is quicker "+
			"than arXiv allows: no more than one request every %v",
			pace, leastPace())
	}
	if clock == nil {
		return nil, celestialdata.Refuse("a gate needs a clock")
	}
	gentle, err := web.NewGentle(policy(pace), clock, web.Unix())
	if err != nil {
		return nil, celestialdata.Refuse("%v", err)
	}
	return &Gate{gentle: gentle}, nil
}

// Insist has the gate ask arXiv anyway, for this run. What arXiv asked
// for is written to say before the request goes. It refuses a missing
// writer, since what is overridden must be said.
func (gate *Gate) Insist(say io.Writer) error {
	return gate.gentle.Insist(say)
}

// Say gives the gate a writer for saying what it cannot keep, such as a
// record that will not write. With none it stays quiet and goes on.
func (gate *Gate) Say(say io.Writer) { gate.gentle.Say(say) }

// Asked is how many requests the gate has let through. Read it when none
// is waiting.
func (gate *Gate) Asked() int { return gate.gentle.Asked() }

// enter waits until the request before has left and the pace has
// passed, as the web package's gentle does it. A request let through
// must be left.
func (gate *Gate) enter(ctx context.Context) error {
	return gate.gentle.Enter(ctx)
}

// leave lets the next request through to wait at the gate.
func (gate *Gate) leave() { gate.gentle.Leave() }

// judge says what arXiv's answer means, once the request is done, with
// the gate held. See web.Gentle.Judge.
func (gate *Gate) judge(code int, retryAfter string,
	failure error) (bool, error) {
	return gate.gentle.Judge(code, retryAfter, failure)
}

// reask is what follows a fresh stop from arXiv when the gate insists:
// asked once more, with the gate held. See web.Gentle.Reask.
func (gate *Gate) reask(ctx context.Context, err error,
	reasked *bool) error {
	return gate.gentle.Reask(ctx, err, reasked)
}
