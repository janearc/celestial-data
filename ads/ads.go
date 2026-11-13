// Package ads asks NASA's Astrophysics Data System for papers, one
// search a call.
//
// ADS wants a token with every request, and limits how many requests
// each endpoint answers in a day. When it asks us to stop, nothing more
// is asked: there are no retries here, and no loops. The token is sent
// in a header and is in no error, ever.
package ads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// api is the address of ADS's search endpoint, on the host ADS keeps
// for its API.
const api = "https://api.adsabs.harvard.edu/v1/search/query"

// What a search asks for, in the words ADS's search endpoint is asked in.
const (
	// fields are the fields of each paper that come back.
	fields = "bibcode,title,year,author,identifier"
	// newest sorts the papers found by date, latest first.
	newest = "date desc"
	// rateReset and rateRemaining are the headers ADS answers with to
	// say when its daily limit resets, and how much of it is left.
	rateReset     = "X-RateLimit-Reset"
	rateRemaining = "X-RateLimit-Remaining"
)

// The fewest rows a search asks, and where a list begins.
const (
	fewest = 1 // defined: a search of no rows asks nothing
	first  = 0 // defined: a list begins at its first
)

// Doer is what a search is sent through. An *http.Client is one.
type Doer interface {
	Do(request *http.Request) (*http.Response, error)
}

// Paper is a paper ADS found.
type Paper struct {
	bibcode string
	year    string
	titles  []string
	authors []string
	arxiv   string
}

// arxivSays is how ADS writes a paper's arXiv id among its identifiers.
const arxivSays = "arXiv:"

// ArXiv is the paper's arXiv id, as the papers verb fetches by, or none
// when ADS knows of no copy on arXiv.
func (paper Paper) ArXiv() string { return paper.arxiv }

// arxivOf is the arXiv id among a paper's identifiers, or none.
func arxivOf(identifiers []string) string {
	for _, one := range identifiers {
		if id, found := strings.CutPrefix(one, arxivSays); found {
			return id
		}
	}
	return ""
}

// Bibcode is ADS's name for the paper.
func (paper Paper) Bibcode() string { return paper.bibcode }

// Year is the year the paper is from, as ADS writes it.
func (paper Paper) Year() string { return paper.year }

// Title is the paper's first title, or none when ADS gave no title.
func (paper Paper) Title() string {
	if len(paper.titles) == first {
		return ""
	}
	return paper.titles[first]
}

// Authors are the paper's authors, in ADS's order. The list is a copy.
func (paper Paper) Authors() []string {
	return slices.Clone(paper.authors)
}

// Found is what a search found.
type Found struct {
	count     int
	papers    []Paper
	remaining string
}

// Count is how many papers ADS says match the query. It may be more
// than the papers it gave.
func (found *Found) Count() int { return found.count }

// Papers are the papers ADS gave, in its order. The list is a copy.
func (found *Found) Papers() []Paper { return slices.Clone(found.papers) }

// Remaining is how much of the day's limit ADS says is left, as it
// wrote it, or none when it did not say.
func (found *Found) Remaining() string { return found.remaining }

// wire is the shape of ADS's answer. The response is a pointer so an
// answer without one can be told from one that found nothing.
type wire struct {
	Response *struct {
		NumFound int `json:"numFound"`
		Docs     []struct {
			Bibcode string   `json:"bibcode"`
			Title   []string `json:"title"`
			Year    string   `json:"year"`
			Author  []string `json:"author"`
			Ids     []string `json:"identifier"`
		} `json:"docs"`
	} `json:"response"`
}

// Search asks ADS one question and gives what it found: at most so
// many papers, the newest first. It sends one request and does not
// retry. It refuses a missing token or query, and fewer than one paper,
// before it sends anything.
//
// The token goes in the Authorization header and in no error. The agent
// is how the request names itself.
//
// A 401 is ADS refusing the token. A 429 is ADS asking us to stop: its
// error is a *Stopped, for errors.As. Any other status but 200 is an
// error that names it.
func Search(ctx context.Context, doer Doer, token, query string,
	most int, agent string) (*Found, error) {
	switch {
	case token == "":
		return nil, errors.New("ads: a search needs a token")
	case query == "":
		return nil, errors.New("ads: a search needs a query")
	case most < fewest:
		return nil, fmt.Errorf("ads: a search asks at least %d paper, "+
			"not %d", fewest, most)
	}
	asked := url.Values{"q": {query}, "fl": {fields},
		"rows": {strconv.Itoa(most)}, "sort": {newest}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		api+"?"+asked.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("ads: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("User-Agent", agent)
	answer, err := doer.Do(request)
	if err != nil {
		return nil, safely("the search failed", err, token)
	}
	defer answer.Body.Close()
	switch answer.StatusCode {
	case http.StatusOK:
		return read(answer, token)
	case http.StatusUnauthorized:
		return nil, errors.New("ads: ADS refused the token")
	case http.StatusTooManyRequests:
		return nil, &Stopped{reset: heard(answer.Header.Get(rateReset),
			token)}
	}
	return nil, fmt.Errorf("ads: ADS answered with status %d",
		answer.StatusCode)
}

// safely is the error of a search that failed before ADS's answer was
// read: the request, or the read of its body, both of which carry what
// the network said.
//
// One the caller stopped says so, and errors.Is finds the context's
// reason in it. Any other is said in its own words with the token taken
// out, after what is said is the error of what failed, and is not
// wrapped, so nothing further down its chain can hold the token.
func safely(what string, err error, token string) error {
	for _, reason := range []error{context.Canceled,
		context.DeadlineExceeded} {
		if errors.Is(err, reason) {
			return fmt.Errorf("ads: the search was stopped: %w", reason)
		}
	}
	return fmt.Errorf("ads: %s: %s", what, heard(err.Error(), token))
}

// heard is something ADS or the network said, as it may be said again:
// what cannot be printed left out, then the token taken out, and cut to
// mostHeard characters. What cannot be printed goes first, so a token
// split by it cannot come back whole once it is gone.
func heard(said, token string) string {
	said = strings.Map(func(each rune) rune {
		if unicode.IsPrint(each) {
			return each
		}
		return dropped
	}, said)
	if token != "" {
		said = strings.ReplaceAll(said, token, "[the token]")
	}
	if kept := []rune(said); len(kept) > mostHeard {
		said = string(kept[:mostHeard])
	}
	return said
}

// How what is heard is said again, each defined.
const (
	// dropped is strings.Map's mark for a rune to leave out. Defined by
	// strings.Map.
	dropped = -1
	// mostHeard is the most characters of what is heard said again.
	// Defined: room for any reason a transport gives, and a reset time.
	mostHeard = 256
)

// read is what a 200 answer found. What ADS says remains is said again
// as heard.
func read(answer *http.Response, token string) (*Found, error) {
	var said wire
	if err := json.NewDecoder(answer.Body).Decode(&said); err != nil {
		return nil, safely("the answer is not JSON", err, token)
	}
	if said.Response == nil {
		return nil, errors.New("ads: the answer has no response")
	}
	found := &Found{count: said.Response.NumFound,
		remaining: heard(answer.Header.Get(rateRemaining), token)}
	for _, doc := range said.Response.Docs {
		found.papers = append(found.papers, Paper{bibcode: doc.Bibcode,
			year: doc.Year, titles: doc.Title, authors: doc.Author,
			arxiv: arxivOf(doc.Ids)})
	}
	return found, nil
}

// Stopped is ADS asking us to stop: it answered 429. It holds the time
// the limit resets as ADS wrote it, or none.
type Stopped struct {
	reset string
}

// Reset is when ADS says the limit resets, as it wrote it, or none.
func (stopped *Stopped) Reset() string { return stopped.reset }

// Error says ADS asked us to stop, and until when if it said.
func (stopped *Stopped) Error() string {
	if stopped.reset == "" {
		return "ads: ADS asked us to stop and gave no time"
	}
	return "ads: ADS asked us to stop until " + stopped.reset
}
