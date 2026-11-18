package web

import (
	"errors"
	"net/http"
)

// Doer is what a request is sent through. An *http.Client is one, and so
// is what Through gives.
type Doer interface {
	Do(request *http.Request) (*http.Response, error)
}

// through is a Doer whose every request goes through a gentle.
type through struct {
	doer   Doer
	choose Chooser
}

// Through is a Doer whose every request goes through the Gentle the
// Chooser gives for its host. The request's own context stops the waits.
//
// An answer of 403, 429, or 503 is read as Within reads it, and is given
// as an error, a *Stopped when the host asked us to stop; any other
// answer is given as it came, for the adapter to read, and shows the
// host is talking.
//
// A request that carries a body is asked again only when the policy says
// a POST to the host is safe to repeat and the request can give its body
// again.
//
// For a host whose policy says HoldsUntilClosed, the answer's body MUST
// be closed before the host is asked again. For any other the gentle is
// let go as the answer arrives.
//
// It refuses a missing doer or chooser.
func Through(doer Doer, choose Chooser) (Doer, error) {
	if doer == nil || choose == nil {
		return nil, errors.New("a doer within a gentle needs a doer " +
			"and a chooser")
	}
	return through{doer: doer, choose: choose}, nil
}

// Do sends the request through the gentle. When the host's policy holds
// until a body is closed, the gentle is held until the answer's is.
func (wrapped through) Do(request *http.Request) (*http.Response, error) {
	var answer *http.Response
	gentle, err := ask(request.Context(), wrapped.choose,
		request.URL.String(), func(gentle *Gentle) bool {
			return repeatable(request, gentle)
		}, true, func() (int, string, error) {
			var failure error
			answer, failure = wrapped.send(request)
			return judgement(request, &answer, failure)
		})
	if err != nil {
		return nil, err
	}
	if gentle.policy.HoldsUntilClosed {
		answer.Body = &held{body: answer.Body, gentle: gentle}
	}
	return answer, nil
}

// repeatable is whether a request may be sent again after a failure with
// no word: a GET or a HEAD may, and one with a body only when the policy
// says it is safe and the request can give its body again.
func repeatable(request *http.Request, gentle *Gentle) bool {
	method := request.Method
	if method == http.MethodGet || method == http.MethodHead {
		return true
	}
	empty := request.Body == nil || request.Body == http.NoBody
	return posting(gentle) && (empty || request.GetBody != nil)
}

// send sends a copy of the request, with its body given again when it
// can be, so the same request may be sent more than once.
func (wrapped through) send(request *http.Request) (*http.Response,
	error) {
	attempt := request.Clone(request.Context())
	if request.GetBody != nil {
		body, err := request.GetBody()
		if err != nil {
			return nil, err
		}
		attempt.Body = body
	}
	return wrapped.doer.Do(attempt)
}

// judgement is what an answer says to a gentle. A failure to send is
// itself. An answer of 403, 429 or 503 is closed and made a *Status, so
// it is judged as Within judges one; any other is given with its code, and
// is kept as the answer.
func judgement(request *http.Request, answer **http.Response,
	failure error) (int, string, error) {
	if failure != nil {
		*answer = nil
		return outcome(failure)
	}
	code := (*answer).StatusCode
	if code != forbidden && code != tooMany && code != unavailable {
		return code, "", nil
	}
	refused := &Status{method: request.Method, url: request.URL.String(),
		status: (*answer).Status, code: code,
		retryAfter: (*answer).Header.Get("Retry-After")}
	(*answer).Body.Close()
	*answer = nil
	return outcome(refused)
}
