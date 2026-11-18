package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// oneAtATime is how many requests a gentle lets through at once. It's
// defined: one host is asked by one request at a time.
const oneAtATime = 1

// Gentle is the one core of asking a host without being a nuisance: one
// request at a time, each at least a pace after the one before began,
// nothing sent while the host has asked us to wait, and no more asking
// once the host has failed too often with no word.
//
// A request enters, is answered, is judged and leaves; the next waits at
// the gate. For a policy that holds until a body is closed, a request
// leaves when its body is closed, not when the answer arrives.
//
// Held, a gentle keeps its last start, any time the host asked it to wait
// until, what the host's Retry-After said, and its overrides in a record,
// so the pace and the wait carry over between runs.
//
// Within and Through are its faces for a Web and a Doer; Enter, Leave,
// Judge and GiveUp are for a library whose requests we do not make.
//
// Insist makes it fail soft: it says what the host asked for and asks
// anyway.
//
// A Gentle may be used from many goroutines: requests go one at a time,
// and Insist, Hold and Release may be called while one is in flight.
type Gentle struct {
	turn   chan struct{}
	policy Policy
	clock  Clock
	random Random
	// guard keeps the fields below, which a request changes while it holds
	// the turn and Insist, Hold, Release and Asked read or change at any
	// time. It is never held across a sleep.
	guard     sync.Mutex
	last      time.Time
	until     time.Time
	askedFor  string
	asked     int
	giveUps   int
	open      bool
	insisting io.Writer
	overrides []override
	// what was overridden already this run, so each is said and kept
	// once, not once a request
	overrode     time.Time
	overrodeOpen bool
	record       string
	lock         *os.File
	// sharing is the path of a shared record, which no lock is held on
	// between requests; unwritten is whether a stop or an override is in
	// memory that the record does not hold yet; speaks is where the
	// gentle says what it cannot keep, and told what it said already.
	sharing   string
	unwritten bool
	speaks    io.Writer
	told      map[string]bool
}

// NewGentle is a gentle at a policy, on a clock, with a source of
// randomness for jitter. It refuses a policy that cannot be asked by,
// such as a pace quicker than the host's least, and a missing clock or
// source.
func NewGentle(policy Policy, clock Clock, random Random) (*Gentle,
	error) {
	if err := policy.check(); err != nil {
		return nil, err
	}
	if clock == nil || random == nil {
		return nil, fmt.Errorf("a gentle for %s needs a clock and a "+
			"source of randomness", policy.Host)
	}
	return &Gentle{turn: make(chan struct{}, oneAtATime), policy: policy,
		clock: clock, random: random}, nil
}

// Insist has the gentle ask anyway, for this run. A wait the host asked
// for, here or in an earlier run, and an open circuit, are asked through
// after it writes what the host asked for, and the override is kept in
// the record.
//
// A request the host refuses with a fresh stop is asked once more, after
// the pace, and only once. A stop that comes back after that is given,
// and says the override was tried.
//
// It is an error to insist with no writer to say what is overridden: the
// override would go unsaid, so it is not set.
func (gentle *Gentle) Insist(say io.Writer) error {
	if say == nil {
		return fmt.Errorf(
			"asking %s anyway needs a writer "+
				"to say what it overrides",
			gentle.policy.Host)
	}
	gentle.guard.Lock()
	defer gentle.guard.Unlock()
	gentle.insisting = say
	return nil
}

// Say gives the gentle a writer for saying what it cannot keep, such as a
// record that will not write. With none, and none to insist by, it stays
// quiet and goes on.
func (gentle *Gentle) Say(say io.Writer) {
	gentle.guard.Lock()
	defer gentle.guard.Unlock()
	gentle.speaks = say
}

// Asked is how many requests the gentle has let through.
func (gentle *Gentle) Asked() int {
	gentle.guard.Lock()
	defer gentle.guard.Unlock()
	return gentle.asked
}

// For is the gentle itself, for any address: a gentle is the Chooser of a
// host that is asked by none other.
func (gentle *Gentle) For(string) (*Gentle, error) { return gentle, nil }

// Enter waits until the request before has left and the pace has
// passed since it started, then notes the start in the record. A
// request that is let through must be Left.
//
// When the context ends first, or the record will not write, it gives
// why and does not hold the gentle. Before the time the host asked to be
// left until, or when the circuit is open, it sends nothing and says
// so, unless the gentle insists.
func (gentle *Gentle) Enter(ctx context.Context) error {
	// an ended context sends nothing, even when the gentle is free
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case gentle.turn <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := gentle.begin(ctx); err != nil {
		<-gentle.turn
		return err
	}
	return nil
}

// Leave lets the next request through to wait out the pace. A Leave with
// nothing held does nothing, so a second one cannot wait forever.
func (gentle *Gentle) Leave() {
	select {
	case <-gentle.turn:
	default:
	}
}

// begin is a request's start, with the turn held: the stops, the wait out
// of the pace, and the note in the record. The pace is slept without the
// guard, so Insist and Release are not kept waiting for it.
func (gentle *Gentle) begin(ctx context.Context) error {
	if gentle.shares() {
		return gentle.beginShared(ctx)
	}
	gentle.guard.Lock()
	now := gentle.clock.Now()
	refused := gentle.refused(now)
	// before the first request, last is the zero time, long past; a
	// last that a clock set wrong puts ahead is waited no more than the
	// pace
	wait := min(gentle.last.Add(gentle.policy.Pace).Sub(now),
		gentle.policy.Pace)
	gentle.guard.Unlock()
	if refused != nil {
		return refused
	}
	if wait > noWait {
		if err := gentle.clock.Sleep(ctx, wait); err != nil {
			return err
		}
	}
	gentle.guard.Lock()
	defer gentle.guard.Unlock()
	gentle.last = gentle.clock.Now()
	if err := gentle.note(); err != nil {
		return err
	}
	gentle.asked++
	return nil
}

// refused is why a request may not begin at a time, or nothing. A gentle
// that insists says what it is overriding, keeps it, and refuses nothing.
// The guard is held.
func (gentle *Gentle) refused(now time.Time) error {
	switch {
	case gentle.until.After(now):
		until := gentle.until
		why := fmt.Sprintf("it asked to be left until %s, and it is %s",
			until.Format(time.RFC3339), now.Format(time.RFC3339))
		stopped := gentle.stopped(why, gentle.askedFor, until)
		return gentle.insistedOn(now, stopped, !gentle.overrode.Equal(
			until), func() { gentle.overrode = until })
	case gentle.open:
		why := "it failed too often with no word from it"
		stopped := gentle.stopped(why, "", time.Time{})
		stopped.Ours = true
		return gentle.insistedOn(now, stopped, !gentle.overrodeOpen,
			func() { gentle.overrodeOpen = true })
	}
	return nil
}

// stopped is a Stopped for this host. The guard is held.
func (gentle *Gentle) stopped(why, retryAfter string,
	until time.Time) *Stopped {
	return &Stopped{Host: gentle.policy.Host, Why: why,
		RetryAfter: retryAfter, Until: until,
		Defaulted: gentle.policy.Defaulted,
		Insisting: gentle.insisting != nil}
}

// insistedOn is the stop itself when the gentle does not insist. When it
// does, it is nothing: the first time, it says what was overridden and
// what the host asked for, keeps it in the record's overrides, and marks
// it done. The guard is held.
func (gentle *Gentle) insistedOn(now time.Time, stopped *Stopped, first bool,
	mark func()) error {
	if gentle.insisting == nil {
		return stopped
	}
	if first {
		mark()
		asked := noneAsked
		switch {
		case stopped.Ours:
			asked = circuitAsked
		case stopped.RetryAfter != "":
			asked = stopped.RetryAfter
		}
		fmt.Fprintf(gentle.insisting, "%s: %s; %s; asking anyway, "+
			"as told to\n", stopped.Host, stopped.Why,
			askedText(asked))
		gentle.keep(override{when: now, until: stopped.Until,
			asked: asked})
	}
	return nil
}

// askedText says what a host asked for, as the record holds it.
func askedText(asked string) string {
	switch asked {
	case noneAsked:
		return "it named no Retry-After"
	case circuitAsked:
		return "this is our own circuit, not its word"
	}
	return "its Retry-After was " + asked
}

// keep adds an override to those kept, and drops the oldest past as many
// as the policy keeps. The guard is held.
func (gentle *Gentle) keep(one override) {
	gentle.overrides = append(gentle.overrides, one)
	gentle.trim()
}

// trim drops the oldest overrides past as many as the policy keeps. The
// guard is held.
func (gentle *Gentle) trim() {
	most := gentle.policy.OverridesKept
	if extra := len(gentle.overrides) - most; extra > 0 {
		gentle.overrides = gentle.overrides[extra:]
	}
}

// neverOpens is the Opens of a policy whose circuit never opens. It's
// defined: none.
const neverOpens = 0

// The statuses a host says stop with.
const (
	forbidden   = http.StatusForbidden
	tooMany     = http.StatusTooManyRequests
	unavailable = http.StatusServiceUnavailable
)

// noAnswer is the code of a request that was given no answer. It's
// defined by Judge: zero.
const noAnswer = 0

// Judge says what a request's answer means to the host, once the request
// is done, with the gentle held. The code is the status's, or zero when
// no answer came; retryAfter is the host's Retry-After as written, or
// empty; failure is nil for a success.
//
// A 403 or 429, or a 503 with a Retry-After, is the host asking us to
// stop: the wait is kept, and err is a *Stopped. So is a 503 with none
// when the policy allows no retries, since it may not be asked again.
//
// Silent is a failure with no word from the host that is worth asking
// again: a timeout, a reset connection, a connection that ended early,
// or a 503 with no Retry-After. The caller may ask again after a wait
// of its own, and calls GiveUp when it will not.
//
// Within and Through do both.
//
// Any other error, a malformed address or a refused connection among
// them, comes back as it was, and is not silent and not counted.
//
// Any answer from the host but a silent one, whatever its status, shows
// it is talking, and starts the count of failures again; a success or
// any other answer closes an open circuit.
func (gentle *Gentle) Judge(code int, retryAfter string,
	failure error) (silent bool, err error) {
	gentle.guard.Lock()
	silent, err = gentle.judged(code, retryAfter, failure)
	var stopped *Stopped
	noted := errors.As(err, &stopped)
	shared := noted && gentle.sharing != ""
	if noted && gentle.record != "" {
		if writeErr := gentle.note(); writeErr != nil {
			err = errors.Join(err, writeErr)
		}
	}
	gentle.unwritten = gentle.unwritten || shared
	gentle.guard.Unlock()
	if shared {
		gentle.noteShared()
	}
	return silent, err
}

// judged is what Judge says, with the guard held. A stop is in memory
// when it gives.
func (gentle *Gentle) judged(code int, retryAfter string,
	failure error) (silent bool, err error) {
	switch {
	case failure == nil:
		gentle.answered()
		return false, nil
	case code == forbidden, code == tooMany, code == unavailable &&
		(retryAfter != "" || gentle.policy.Tries == 0):
		until := gentle.stopUntil(retryAfter)
		gentle.answered()
		return false, gentle.stopped(failure.Error(), retryAfter, until)
	case silentFailure(code, retryAfter, failure):
		return true, failure
	case code != noAnswer:
		gentle.answered()
	}
	return false, failure
}

// answered is the host showing it is talking: the count of failures
// starts again and the circuit closes. The guard is held.
func (gentle *Gentle) answered() {
	gentle.giveUps, gentle.open = 0, false
}

// silentFailure is whether a failure is the host saying nothing: a 503
// with no Retry-After, or no answer at all for a reason that is the
// network's and may pass: a timeout, a connection reset, or one that
// ended early.
//
// A malformed address, a refused connection, a name not found, a
// certificate that fails and too many redirects are ours or lasting, and
// are not.
func silentFailure(code int, retryAfter string, failure error) bool {
	if code == unavailable {
		return retryAfter == ""
	}
	if code != noAnswer {
		return false
	}
	var network net.Error
	return errors.Is(failure, io.EOF) ||
		errors.Is(failure, io.ErrUnexpectedEOF) ||
		errors.Is(failure, syscall.ECONNRESET) ||
		errors.As(failure, &network) && network.Timeout()
}

// GiveUp is a request that has ended with no word from the host, after
// all the tries it was allowed. The gentle counts them in a row, and when
// the policy's opens have passed it leaves the host alone for the rest of
// the run.
func (gentle *Gentle) GiveUp() {
	gentle.guard.Lock()
	defer gentle.guard.Unlock()
	gentle.giveUps++
	// a policy of no opens has no circuit: the host is always asked
	if gentle.policy.Opens != neverOpens &&
		gentle.giveUps >= gentle.policy.Opens {
		gentle.open = true
		gentle.overrodeOpen = false
	}
}

// stopUntil has the gentle leave the host alone as long as a Retry-After
// asks: so many seconds, or until a date. A number that fits but is longer
// than a duration holds is kept as the longest wait there is.
//
// A date already past is the server saying the time has gone, so it
// stops nothing. With nothing it can read, a number too large for a whole
// number among it, the gentle rests as long as the policy's rest says.
// The guard is held.
func (gentle *Gentle) stopUntil(retryAfter string) time.Time {
	now := gentle.clock.Now()
	until := now.Add(gentle.policy.Rest)
	seconds, err := strconv.ParseInt(retryAfter, 10, 64)
	switch {
	case err == nil && seconds >= noSeconds:
		until = now.Add(time.Duration(min(seconds, longestSeconds)) *
			time.Second)
	default:
		if at, err := http.ParseTime(retryAfter); err == nil {
			until = at
		}
	}
	// no request goes out while an earlier wait is left, unless an
	// override sends it, and then the wait just asked is the one
	gentle.until, gentle.askedFor = until, retryAfter
	return until
}

// noSeconds is a Retry-After of no wait. It's defined: none.
const noSeconds = 0

// longestSeconds is the longest Retry-After the gentle keeps, in seconds.
// It's defined by time.Duration: the most seconds it holds, some 292
// years; a longer one is kept as this.
const longestSeconds = math.MaxInt64 / int64(time.Second)

// ask is a request's life at the gentle of its address's host: the
// gentle the chooser gives, one request after another while the host says
// nothing, then the answer.
//
// With hold, and when the gentle's policy holds until a body is closed,
// the gentle is held for the caller to Leave when its answer is done
// with; otherwise it is let go as the answer arrives.
func ask(ctx context.Context, choose Chooser, address string,
	repeats func(*Gentle) bool, hold bool,
	call func() (code int, retryAfter string, failure error)) (*Gentle,
	error) {
	gentle, err := choose.For(address)
	if err != nil {
		return nil, err
	}
	if err := gentle.try(ctx, repeats(gentle), call); err != nil {
		return nil, err
	}
	if !hold || !gentle.policy.HoldsUntilClosed {
		gentle.Leave()
	}
	return gentle, nil
}

// try is a request's whole life at the gentle: it enters, calls, judges,
// and when the host said nothing asks again after a backoff, as many
// times as the policy allows, only when repeats is true.
//
// When the host refuses it with a fresh stop and the gentle insists, it
// is asked once more, only when repeats is true.
//
// It gives nil with the gentle still held, for the caller to Leave when
// its answer is done with, or the error with the gentle left. Context is
// checked between attempts, and a failure while the context has ended is
// the context's, not the host's.
func (gentle *Gentle) try(ctx context.Context, repeats bool,
	call func() (code int, retryAfter string, failure error)) error {
	if err := gentle.Enter(ctx); err != nil {
		return err
	}
	reasked := false
	for attempt := 0; ; attempt++ {
		code, retryAfter, failure := call()
		if failure != nil && ctx.Err() != nil {
			gentle.Leave()
			return failure
		}
		silent, err := gentle.Judge(code, retryAfter, failure)
		if err == nil {
			return nil
		}
		err = gentle.another(ctx, attempt, repeats, &reasked, silent,
			err)
		if err != nil {
			gentle.Leave()
			return err
		}
	}
}

// another is what follows a request that was not answered: nil when it
// is to be asked again, the pause or the pace done, or the error to give.
// Only a request that was allowed its tries and used them all is counted
// against the host.
func (gentle *Gentle) another(ctx context.Context, attempt int,
	repeats bool, reasked *bool, silent bool, err error) error {
	var stopped *Stopped
	switch {
	case silent && repeats && attempt < gentle.policy.Tries:
		return gentle.pause(ctx, attempt)
	case silent && repeats:
		gentle.GiveUp()
	case errors.As(err, &stopped):
		return gentle.afterStop(ctx, stopped, repeats, reasked, err)
	}
	return err
}

// afterStop is what follows a fresh stop. When the gentle insists, and
// the request may be asked again, and has not been for a stop, it writes
// what the host asked for, keeps the override in the record, waits the
// pace and gives nil to ask once more.
//
// Otherwise it gives the stop, which says the override was tried when it
// was.
func (gentle *Gentle) afterStop(ctx context.Context, stopped *Stopped,
	repeats bool, reasked *bool, stopErr error) error {
	gentle.guard.Lock()
	if gentle.insisting == nil || !repeats || *reasked {
		gentle.guard.Unlock()
		stopped.Tried = *reasked
		return stopErr
	}
	*reasked = true
	asked := noneAsked
	if stopped.RetryAfter != "" {
		asked = stopped.RetryAfter
	}
	fmt.Fprintf(gentle.insisting, "%s: %s; %s; asking once more, as told "+
		"to\n", stopped.Host, stopped.Why, askedText(asked))
	gentle.keep(override{when: gentle.clock.Now(), until: stopped.Until,
		asked: asked})
	// the stop is said and kept here, so the pace's start does not say it
	// again
	gentle.overrode = stopped.Until
	gentle.guard.Unlock()
	return gentle.begin(ctx)
}

// Reask is what follows a fresh stop, for a request made with Enter, Leave
// and Judge, as Within does it for a Web. Give it the error Judge gave and
// a flag that starts false and is the caller's to keep for the request.
//
// When the gentle insists, and has not asked again for this request, it
// says what the host asked for, keeps the override, waits the pace and
// gives nil: ask once more, with the gentle still held. Otherwise it
// gives the stop, which says the override was tried when it was.
//
// An error that is no stop is given as it came.
func (gentle *Gentle) Reask(ctx context.Context, err error,
	reasked *bool) error {
	var stopped *Stopped
	if !errors.As(err, &stopped) {
		return err
	}
	return gentle.afterStop(ctx, stopped, true, reasked, err)
}

// backoff waits the jittered wait before a retry, with the turn held, so
// no other request goes to the host meanwhile, and gives why when the
// context ends first.
func (gentle *Gentle) backoff(ctx context.Context, attempt int) error {
	wait := Jitter(gentle.random, Ceiling(gentle.policy, attempt))
	return gentle.clock.Sleep(ctx, wait)
}

// pause is the backoff and the start of the next attempt, which waits out
// the pace. It gives why when either cannot be done.
func (gentle *Gentle) pause(ctx context.Context, attempt int) error {
	if err := gentle.backoff(ctx, attempt); err != nil {
		return err
	}
	return gentle.begin(ctx)
}
