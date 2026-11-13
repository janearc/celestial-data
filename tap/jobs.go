package tap

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/janearc/celestial-data/web"
)

// The parts of an asynchronous query, by TAP 1.1 and UWS 1.1.
const (
	asyncPath = "/async"          // defined: TAP's endpoint for jobs
	result    = "/results/result" // defined: TAP's address of an answer
	complete  = "COMPLETED"       // defined: UWS, the job is done
	errored   = "ERROR"           // defined: UWS, the job failed
	aborted   = "ABORTED"         // defined: UWS, the job was stopped
)

// unsaid is the length of an answer no one has asked for yet. It's
// defined by web.Web.
const unsaid = -1

// Jobs is a web that runs a query asked of a service's async endpoint as
// a job. Asked for such a query's address, as URL makes it, it submits
// the query, asks after the job every so often until it is complete, and
// opens its answer. It asks the web under it for anything else unchanged.
//
// It says each job's address when it is submitted and when it is
// complete, so a fetch says which job its file came from.
type Jobs struct {
	under web.Web
	every time.Duration
	asks  int
	told  io.Writer
}

// What a web of jobs is refused for: a wait of none or less, and fewer
// asks than one.
const (
	noWait    = 0 // defined: no time between asks
	leastAsks = 1 // defined: a job is asked after once at least
)

// NewJobs is a web of jobs over under, which asks after a job every so
// often and at most asks times, and says each job's address to told. It
// refuses no web under it, a wait that is not one, fewer than one ask,
// and no one to tell.
func NewJobs(under web.Web, every time.Duration, asks int,
	told io.Writer) (*Jobs, error) {
	if under == nil || every <= noWait || asks < leastAsks || told == nil {
		return nil, fmt.Errorf("jobs need a web, a wait, an ask "+
			"and someone to tell, not %v, %v, %d and %v", under,
			every, asks, told)
	}
	return &Jobs{under: under, every: every, asks: asks, told: told}, nil
}

// Get opens what an address holds. For a query asked of an async
// endpoint, that is the job's answer, once the job is complete.
func (jobs *Jobs) Get(address string) (io.ReadCloser, int64, error) {
	endpoint, form, isJob := asked(address)
	if !isJob {
		return jobs.under.Get(address)
	}
	form.Set("PHASE", "run")
	job, err := jobs.under.Post(endpoint, form)
	if err == nil && job == "" {
		err = fmt.Errorf("%s took the query and named no job", endpoint)
	}
	if err == nil {
		fmt.Fprintf(jobs.told, "job %s submitted\n", job)
		err = jobs.wait(job)
	}
	if err != nil {
		return nil, unsaid, err
	}
	fmt.Fprintf(jobs.told, "job %s complete\n", job)
	return jobs.under.Get(job + result)
}

// Head is the length of what an address holds. A job's answer has none
// until it has run, and asking would run it, so for a job it is unsaid.
func (jobs *Jobs) Head(address string) (int64, error) {
	if _, _, isJob := asked(address); isJob {
		return unsaid, nil
	}
	return jobs.under.Head(address)
}

// Post sends a form through the web under it.
func (jobs *Jobs) Post(address string, form url.Values) (string, error) {
	return jobs.under.Post(address, form)
}

// asked is the endpoint and the form of a query asked of an async
// endpoint, and whether the address is one.
func asked(address string) (string, url.Values, bool) {
	where, err := url.Parse(address)
	if err != nil || !strings.HasSuffix(where.Path, asyncPath) ||
		where.Query().Get("REQUEST") != "doQuery" {
		return "", nil, false
	}
	form := where.Query()
	where.RawQuery = ""
	return where.String(), form, true
}

// phase is what a job's description says of it: its phase, and when it
// failed, why.
type phase struct {
	//ninja:vow gate encoding/xml reads the job's description into it
	Phase string `xml:"phase"`
	//ninja:vow gate encoding/xml reads the job's description into it
	Message string `xml:"errorSummary>message"`
}

// wait asks after a job until it is complete. It says why when the job
// fails or is stopped, and when it is not complete after every ask.
func (jobs *Jobs) wait(job string) error {
	var last phase
	for range jobs.asks {
		now, err := jobs.state(job)
		if err != nil {
			return err
		}
		switch last = *now; now.Phase {
		case complete:
			return nil
		case errored, aborted:
			return fmt.Errorf("%s: %s: %s", job, now.Phase,
				strings.TrimSpace(now.Message))
		}
		time.Sleep(jobs.every)
	}
	return fmt.Errorf("%s is %s still, after %d asks", job, last.Phase,
		jobs.asks)
}

// state is a job's description as the service gives it now, or none
// when it cannot be read or says no phase.
func (jobs *Jobs) state(job string) (*phase, error) {
	body, _, err := jobs.under.Get(job)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	var now phase
	if err := xml.NewDecoder(body).Decode(&now); err != nil {
		return nil, fmt.Errorf("%s: %w", job, err)
	}
	if now.Phase == "" {
		return nil, fmt.Errorf("%s says no phase", job)
	}
	return &now, nil
}
