package web_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"github.com/janearc/puzzlemouth/numbers"
	"go.uber.org/mock/gomock"
)

// start is the time the tests begin at. It's defined: any time will do,
// and this is a plain one.
var start = time.Date(2026, time.November, 11, 12, 0, 0, 0, time.UTC)

// moment is the time a mock clock keeps. Each Sleep passes the time it
// is asked to sleep and is written down, so a test sees what the gentle
// waited without waiting. Sleeps from the one numbered refuseFrom on
// fail, as an ended context fails them; zero refuses none. Goroutines
// may share it.
type moment struct {
	lock       sync.Mutex
	now        time.Time
	slept      []time.Duration
	refuseFrom int
}

// read is the time now.
func (at *moment) read() time.Time {
	at.lock.Lock()
	defer at.lock.Unlock()
	return at.now
}

// set is the time now, put by a test.
func (at *moment) set(now time.Time) {
	at.lock.Lock()
	defer at.lock.Unlock()
	at.now = now
}

// sleep passes the time, unless it is refused.
func (at *moment) sleep(_ context.Context, wait time.Duration) error {
	at.lock.Lock()
	defer at.lock.Unlock()
	at.slept = append(at.slept, wait)
	if at.refuseFrom > 0 && len(at.slept) >= at.refuseFrom {
		return context.Canceled
	}
	at.now = at.now.Add(wait)
	return nil
}

// total is the time all the sleeps passed.
func (at *moment) total() time.Duration {
	at.lock.Lock()
	defer at.lock.Unlock()
	var all time.Duration
	for _, wait := range at.slept {
		all += wait
	}
	return all
}

// clockAt is a mock clock that starts at the time given and passes only
// when it sleeps, and the moment it keeps.
func clockAt(t *testing.T, at time.Time) (*mocks.MockClock, *moment) {
	t.Helper()
	keeping := &moment{now: at}
	clock := mocks.NewMockClock(gomock.NewController(t))
	clock.EXPECT().Now().DoAndReturn(keeping.read).AnyTimes()
	clock.EXPECT().Sleep(gomock.Any(), gomock.Any()).
		DoAndReturn(keeping.sleep).AnyTimes()
	return clock, keeping
}

// half is the fraction a mock random gives, so a jittered wait is half
// its ceiling. It's defined: the middle.
const half = 0.5

// randomOf is a mock random that always gives one number.
func randomOf(t *testing.T, fraction float64) *mocks.MockRandom {
	t.Helper()
	random := mocks.NewMockRandom(gomock.NewController(t))
	random.EXPECT().Float64().Return(fraction).AnyTimes()
	return random
}

// kept is numbers read from a file, as web.Numbers.
type kept struct {
	read numbers.Numbers
}

// Get is the value of a number of a name, or why there is none.
func (held kept) Get(name string) (float64, error) {
	number, err := held.read.Get(name)
	if err != nil {
		return 0, err
	}
	return number.Value, nil
}

// exampleNumbers is the tests' numbers file, as web.Numbers.
func exampleNumbers(t *testing.T) web.Numbers {
	t.Helper()
	raw := readFile(t, numbersFile)
	read, err := numbers.Read(numbersFile, raw)
	if err != nil {
		t.Fatal(err)
	}
	return kept{read: read}
}

// policyOf is the policy of a host the tests' numbers file names by key.
func policyOf(t *testing.T, name, key string) web.Policy {
	t.Helper()
	policy, err := web.PolicyFrom(name, key, exampleNumbers(t))
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

// example is the example host's policy.
func example(t *testing.T) web.Policy {
	t.Helper()
	return policyOf(t, "Example", "example")
}

// heldExample is the example host's policy with its record held for the run.
func heldExample(t *testing.T) web.Policy {
	t.Helper()
	policy := example(t)
	policy.Keeping = web.KeepsForTheRun
	return policy
}

// gentleOn is a gentle for a policy on a clock, with half for jitter.
func gentleOn(t *testing.T, policy web.Policy,
	clock web.Clock) *web.Gentle {
	t.Helper()
	gentle, err := web.NewGentle(policy, clock, randomOf(t, half))
	if err != nil {
		t.Fatal(err)
	}
	return gentle
}

// exampleAt is the example host's gentle, on a clock at the start, and
// the moment the clock keeps.
func exampleAt(t *testing.T) (*web.Gentle, *moment) {
	t.Helper()
	clock, keeping := clockAt(t, start)
	return gentleOn(t, example(t), clock), keeping
}

// answer is a body with something in it.
func answer() io.ReadCloser { return io.NopCloser(strings.NewReader("x")) }

// refusal is an answer with a status and a Retry-After, when there is
// one.
func refusal(code int, retryAfter string) *http.Response {
	header := http.Header{}
	if retryAfter != "" {
		header.Set("Retry-After", retryAfter)
	}
	return &http.Response{StatusCode: code, Status: http.StatusText(code),
		Header: header, Body: answer()}
}

// statusError is the error a client gives for an answer of a status and
// a Retry-After.
func statusError(code int, retryAfter string) error {
	return web.NewStatus(http.MethodGet, address, http.StatusText(code),
		code, retryAfter)
}

// The failures a client gives with no answer. Three are the host saying
// nothing, and are worth asking again; the rest are ours or lasting.
var (
	timedOut = fmt.Errorf("Get: %w", &net.OpError{Op: "read",
		Err: os.ErrDeadlineExceeded})
	resetBy    = fmt.Errorf("read: %w", syscall.ECONNRESET)
	endedEarly = fmt.Errorf("read: %w", io.ErrUnexpectedEOF)
	endedClean = fmt.Errorf("read: %w", io.EOF)
	malformed  = &url.Error{Op: "Get", URL: "://",
		Err: errors.New("missing protocol scheme")}
	refusedTo = &net.OpError{Op: "dial", Err: &os.SyscallError{
		Syscall: "connect", Err: syscall.ECONNREFUSED}}
	notFound   = &net.DNSError{Err: "no such host", IsNotFound: true}
	redirected = &url.Error{Op: "Get", URL: address,
		Err: errors.New("stopped after 10 redirects")}
)

// silentKinds are the failures that are worth asking again.
func silentKinds() map[string]error {
	return map[string]error{"a timeout": timedOut, "a reset": resetBy,
		"an end too early": endedEarly, "an end": endedClean}
}

// lastingKinds are the failures that are ours or lasting, and are given
// at once.
func lastingKinds() map[string]error {
	return map[string]error{"a malformed address": malformed,
		"a refused connection": refusedTo, "a name not found": notFound,
		"too many redirects": redirected}
}
