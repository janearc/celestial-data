package arxiv

import (
	"context"
	"errors"
	"fmt"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
)

// Paced is a web whose every request goes through the gate, waiting
// there while ctx lasts. One goes at a time, each begun at least the
// gate's pace after the last began. A Get holds the gate until its body
// is closed, so a slow download is still the only request.
//
// A 403, 429 or 503 is arXiv asking us to stop, and the request says so,
// with the wait asked.
//
// It refuses a missing context, web or gate.
func Paced(ctx context.Context, get web.Web, gate *Gate) (web.Web,
	error) {
	if ctx == nil || get == nil || gate == nil {
		return nil, celestialdata.Refuse("a paced web needs a " +
			"context, a web and a gate")
	}
	within, err := web.Within(ctx, get, gate.gentle)
	if err != nil {
		return nil, celestialdata.Refuse("%v", err)
	}
	return web.Rewriting(within, inWords), nil
}

// halt is arXiv asking us to stop, in the words this adapter has always
// said it in: which host, why, and the wait it asked, as written. The
// web package's stop, which it holds, says more; this is the part arXiv's
// users have been told.
type halt struct {
	stopped *web.Stopped
	kept    error
}

// inWords is an error as this adapter says it: a stop, as a halt, and any
// other error as it is. arXiv's circuit never opens, so every stop is
// its own asking.
func inWords(err error) error {
	var stopped *web.Stopped
	if !errors.As(err, &stopped) {
		return err
	}
	return &halt{stopped: stopped, kept: otherThanStop(err)}
}

// otherThanStop is what a stop was joined with, such as an error writing
// the wait to arXiv's run record.
func otherThanStop(err error) error {
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return nil
	}
	var others []error
	for _, cause := range joined.Unwrap() {
		var stopped *web.Stopped
		if errors.As(cause, &stopped) {
			if other := otherThanStop(cause); other != nil {
				others = append(others, other)
			}
			continue
		}
		others = append(others, cause)
	}
	return errors.Join(others...)
}

// Error says arXiv asked us to stop, why, and how long when it said. When
// the override was tried, it says that too.
func (asking *halt) Error() string {
	text := host + " asked us to stop: " + asking.stopped.Why
	if asking.stopped.RetryAfter != "" {
		text += fmt.Sprintf("; its Retry-After is %s",
			asking.stopped.RetryAfter)
	}
	if asking.stopped.Tried {
		text += "; the override was tried, and it refused again"
	}
	if asking.kept != nil {
		text += "; could not keep arXiv's wait: " + asking.kept.Error()
	}
	return text
}

// Unwrap keeps the stop and the error beside it, so a caller may read both.
func (asking *halt) Unwrap() []error {
	if asking.kept == nil {
		return []error{asking.stopped}
	}
	return []error{asking.stopped, asking.kept}
}
