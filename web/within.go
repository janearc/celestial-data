package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
)

// Chooser says which Gentle asks the host of an address. A *Gentle is the
// Chooser of the one host it asks; *Hosts has one for each.
type Chooser interface {
	For(address string) (*Gentle, error)
}

// held is a body that holds a gentle until it is closed, once.
type held struct {
	body   io.ReadCloser
	gentle *Gentle
	once   sync.Once
}

// Read reads the body.
func (holding *held) Read(into []byte) (int, error) {
	return holding.body.Read(into)
}

// Close closes the body, then lets the gentle go, so the next request
// begins after this one's connection is done. Closing again closes the
// body again and lets go of nothing.
func (holding *held) Close() error {
	err := holding.body.Close()
	holding.once.Do(holding.gentle.Leave)
	return err
}

// within is a Web whose every request goes through a gentle.
type within struct {
	ctx    context.Context
	inner  Web
	choose Chooser
}

// Within is a Web whose every request goes through the Gentle the
// Chooser gives for its host, waiting there while ctx lasts. It is a Web
// as it stands, so no adapter changes to use it.
//
// A Get holds the gentle until its body is closed only for a host whose
// policy says HoldsUntilClosed, so a slow download is still the only
// request. For such a host the body MUST be closed before the host is
// asked again, or the next request waits for it for as long as ctx lasts.
//
// For any other the gentle is let go as the answer arrives, and the pace
// is still measured from the start of each request.
//
// A Get or a Head that fails with no word from the host is asked again
// after a wait, as the policy says. A Post is paced and judged and never
// asked again unless the policy says a POST to the host is safe to
// repeat.
//
// The context stops the waits and any attempt that has not begun. The
// Web interface carries none into a request, so a request in the air is
// bounded only by the timeouts of the Web inside, as New sets them.
//
// Those are the wait for the server to connect and begin its answer, and
// the whole of the request when one is given.
//
// It refuses a missing context, web or chooser.
func Within(ctx context.Context, inner Web, choose Chooser) (Web, error) {
	if ctx == nil || inner == nil || choose == nil {
		return nil, errors.New("a web within a gentle needs a " +
			"context, a web and a chooser")
	}
	return within{ctx: ctx, inner: inner, choose: choose}, nil
}

// outcome is what a request's error says to a gentle: the status's code
// and Retry-After, or no code for an error that is no status.
func outcome(failure error) (int, string, error) {
	var status *Status
	if errors.As(failure, &status) {
		return status.Code(), status.RetryAfter(), failure
	}
	return noAnswer, "", failure
}

// repeating is the rule of a Get or a Head: it may be asked again.
func repeating(*Gentle) bool { return true }

// posting is the rule of a Post: it may be asked again only when the
// host's policy says a POST to it is safe to repeat.
func posting(gentle *Gentle) bool { return gentle.policy.RepeatsPosts }

// Get is the Web's Get, through the gentle. When the host's policy holds
// until a body is closed, the gentle is held until then. With an error,
// the length is the inner web's, or none when the inner web was not asked.
func (wrapped within) Get(address string) (io.ReadCloser, int64, error) {
	var body io.ReadCloser
	var length int64
	gentle, err := ask(wrapped.ctx, wrapped.choose, address, repeating,
		true, func() (int, string, error) {
			var failure error
			body, length, failure = wrapped.inner.Get(address)
			return outcome(failure)
		})
	if err != nil {
		return nil, length, err
	}
	if !gentle.policy.HoldsUntilClosed {
		return body, length, nil
	}
	return &held{body: body, gentle: gentle}, length, nil
}

// Head is the Web's Head, through the gentle. With an error, the length
// is the inner web's, or none when the inner web was not asked.
func (wrapped within) Head(address string) (int64, error) {
	var length int64
	_, err := ask(wrapped.ctx, wrapped.choose, address, repeating, false,
		func() (int, string, error) {
			var failure error
			length, failure = wrapped.inner.Head(address)
			return outcome(failure)
		})
	return length, err
}

// Post is the Web's Post, through the gentle.
func (wrapped within) Post(address string, form url.Values) (string,
	error) {
	var where string
	_, err := ask(wrapped.ctx, wrapped.choose, address, posting, false,
		func() (int, string, error) {
			var failure error
			where, failure = wrapped.inner.Post(address, form)
			return outcome(failure)
		})
	if err != nil {
		return "", err
	}
	return where, nil
}

// Hosts has a Gentle for each host an address names, made when the host
// is first asked, so one Web can ask many hosts and each is asked in its
// own way. A host with no policy of its own is asked by the default
// policy, and its stops say so: it fails gentle, not open.
//
// A host is named in lower case with no dot at its end, in the policies,
// in its record and wherever an address spells it otherwise.
type Hosts struct {
	folder    string
	clock     Clock
	random    Random
	policies  map[string]Policy
	lock      sync.Mutex
	gentles   map[string]*Gentle
	insisting io.Writer
	speaks    io.Writer
	// unkept is why there is no folder to keep records in, when the caller
	// knows; saidUnkept is whether it was said.
	unkept     error
	saidUnkept bool
}

// NewHosts is the Gentles of many hosts. Each host's record is kept in
// the folder, as RecordPath gives; with no folder none is kept. The
// policies are by the host's name in lower case with no dot at its end.
func NewHosts(folder string, clock Clock, random Random,
	policies map[string]Policy) *Hosts {
	return &Hosts{folder: folder, clock: clock, random: random,
		policies: policies, gentles: map[string]*Gentle{}}
}

// Insist has every host's gentle insist, those made already and those
// made later. It is an error to insist with no writer. See Gentle.Insist.
func (hosts *Hosts) Insist(say io.Writer) error {
	if say == nil {
		return errors.New("asking hosts anyway needs a writer to say " +
			"what it overrides")
	}
	hosts.lock.Lock()
	defer hosts.lock.Unlock()
	hosts.insisting = say
	for _, gentle := range hosts.gentles {
		// the writer is here, which is all it refuses
		_ = gentle.Insist(say)
	}
	return nil
}

// Say has every host's gentle say what it cannot keep, such as a record
// that will not write, to a writer, those made already and those made
// later. See Gentle.Say.
func (hosts *Hosts) Say(say io.Writer) {
	hosts.lock.Lock()
	defer hosts.lock.Unlock()
	hosts.speaks = say
	for _, gentle := range hosts.gentles {
		gentle.Say(say)
	}
}

// Unkept says why there is no folder to keep records in, when Hosts was
// made with none. When the first host is asked, it is said once, to the
// writer given to Say, that no wait will be kept between runs.
func (hosts *Hosts) Unkept(why error) {
	hosts.lock.Lock()
	defer hosts.lock.Unlock()
	hosts.unkept = why
}

// For is the Gentle of the host an address names, made if it is the
// first time the host is asked. Example.ORG and example.org. are the same
// host.
func (hosts *Hosts) For(address string) (*Gentle, error) {
	where, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSuffix(strings.ToLower(where.Hostname()), ".")
	if name == "" {
		return nil, fmt.Errorf("%q names no host to ask", address)
	}
	hosts.lock.Lock()
	defer hosts.lock.Unlock()
	if gentle, ok := hosts.gentles[name]; ok {
		return gentle, nil
	}
	gentle, err := hosts.made(name)
	if err != nil {
		return nil, err
	}
	hosts.gentles[name] = gentle
	return gentle, nil
}

// made is a new Gentle for a host, holding its record when there is a
// folder for it.
func (hosts *Hosts) made(name string) (*Gentle, error) {
	policy, ok := hosts.policies[name]
	if !ok {
		var err error
		if policy, err = DefaultPolicy(name); err != nil {
			return nil, err
		}
	}
	gentle, err := NewGentle(policy, hosts.clock, hosts.random)
	if err != nil {
		return nil, err
	}
	if hosts.insisting != nil {
		// the writer is here, which is all it refuses
		_ = gentle.Insist(hosts.insisting)
	}
	gentle.Say(hosts.speaks)
	if hosts.folder == "" {
		hosts.sayNoFolder()
		return gentle, nil
	}
	if err := gentle.Hold(RecordPath(hosts.folder, name)); err != nil {
		return nil, err
	}
	return gentle, nil
}

// sayNoFolder says once, when there is a writer and a reason, that there
// is no folder to keep records in, so no wait is kept between runs. The
// lock is held.
func (hosts *Hosts) sayNoFolder() {
	if hosts.unkept == nil || hosts.speaks == nil || hosts.saidUnkept {
		return
	}
	hosts.saidUnkept = true
	fmt.Fprintf(hosts.speaks, "there is no folder to keep a record of "+
		"waits in (%v); each host is paced for this run only, and no "+
		"wait will be kept between runs\n", hosts.unkept)
}

// Release releases every host's record, so the next run keeps each pace,
// wait and override.
func (hosts *Hosts) Release() error {
	hosts.lock.Lock()
	defer hosts.lock.Unlock()
	var problems []error
	for _, gentle := range hosts.gentles {
		problems = append(problems, gentle.Release())
	}
	return errors.Join(problems...)
}
