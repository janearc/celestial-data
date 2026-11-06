package arxiv

import (
	"context"
	_ "embed"
	"fmt"
	"os"
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

// leastPace is the least time from one request's start to the next's.
// It is three seconds, as arXiv's API terms ask.
func leastPace() time.Duration {
	return web.Seconds(asks.Value("least-pace"))
}

// Clock is the time a gate keeps. Its Sleep ends early when the context
// ends, and gives the context's error.
type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, wait time.Duration) error
}

// wall is the clock on the wall.
type wall struct{}

// Wall is the clock on the wall.
func Wall() Clock { return wall{} }

// Now is the time now.
func (wall) Now() time.Time { return time.Now() }

// Sleep waits so long, or until the context ends.
func (wall) Sleep(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// oneAtATime is how many requests a gate lets through at once. It's
// defined: arXiv asks for a single connection at a time.
const oneAtATime = 1

// noWait is a wait with nothing left of it. It's defined: none.
const noWait time.Duration = 0

// Gate lets one request at a time through to arXiv. Each starts at
// least its pace after the one before started. A request enters, is
// answered and leaves; the next waits at the gate.
//
// Held, a gate keeps its last start and any time arXiv asked it to wait
// until in a record, so the pace and the wait carry over between runs.
type Gate struct {
	turn   chan struct{}
	pace   time.Duration
	clock  Clock
	last   time.Time
	until  time.Time
	asked  int
	record string
	lock   *os.File
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
	return &Gate{turn: make(chan struct{}, oneAtATime), pace: pace,
		clock: clock}, nil
}

// enter waits until the request before has left and the pace has
// passed since it started, then notes the start in the record.
//
// When the context ends first, or the record will not write, it gives
// why and does not hold the gate. Before the time arXiv asked to be
// left until, it sends nothing and says so.
func (gate *Gate) enter(ctx context.Context) error {
	// an ended context sends nothing, even when the gate is free
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case gate.turn <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	now := gate.clock.Now()
	if gate.until.After(now) {
		<-gate.turn
		return &halt{status: fmt.Sprintf("it asked to be left until "+
			"%s, and it is %s", gate.until.Format(time.RFC3339),
			now.Format(time.RFC3339))}
	}
	// before the first request, last is the zero time, long past; a
	// last that a clock set wrong puts ahead is waited no more than the
	// pace
	wait := min(gate.last.Add(gate.pace).Sub(now), gate.pace)
	if wait > noWait {
		if err := gate.clock.Sleep(ctx, wait); err != nil {
			<-gate.turn
			return err
		}
	}
	gate.last = gate.clock.Now()
	if err := gate.note(); err != nil {
		<-gate.turn
		return err
	}
	gate.asked++
	return nil
}

// leave lets the next request through to wait out the pace.
func (gate *Gate) leave() { <-gate.turn }

// Asked is how many requests the gate has let through. Read it when none
// is waiting.
func (gate *Gate) Asked() int { return gate.asked }
