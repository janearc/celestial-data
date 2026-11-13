package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/ads"
	"github.com/janearc/celestial-data/secrets"
	"github.com/janearc/celestial-data/web"
	"github.com/janearc/puzzlemouth/pzm"
	"github.com/janearc/puzzlemouth/verb"
)

// searchingADS is the ads verb's facts: it reads the kept token, asks
// ADS, and writes nothing. It asks no one.
var searchingADS = verb.NewFacts(verb.WritesNothing, verb.LeavesAlone,
	verb.Outside, verb.NoOne, verb.Light)

const adsPage = "Searches NASA's Astrophysics Data System for the words " +
	"given, as ADS's own search box takes them: author:, year:, " +
	"full:\"words\" and the rest. It says how many papers match, the " +
	"newest of them, as many as --most or ads-most says, each by its " +
	"bibcode, year, first author and title, and how many searches ADS " +
	"allows today. The token is read from ads-token beside the " +
	"secrets' config, or from the file ADS_SESSION names: where " +
	"celestial-data-secrets ads-login keeps it, so a search asks no " +
	"one. A run sends one request, and none more once ADS asks us to " +
	"stop."

// fewestPapers is the fewest papers a search may ask for. It's defined:
// a search of none asks nothing.
const fewestPapers = 1

// adsVerb searches ADS for the words given and says what it found.
func (tool command) adsVerb(call pzm.Call) error {
	words := strings.Join(call.Args(), " ")
	query := strings.TrimSpace(words)
	if query == "" {
		return celestialdata.Refuse("ads needs words to search for")
	}
	picked, err := tool.picked.Get("ads-most")
	if err != nil {
		return err
	}
	most, err := count(call, "most", int(picked))
	if err != nil {
		return err
	}
	if most < fewestPapers {
		return celestialdata.Refuse("--most is how many papers to " +
			"show, at least one")
	}
	token, err := tool.adsToken()
	if err != nil {
		return err
	}
	return tool.searchADS(call, token, query, most)
}

// adsToken is the token celestial-data-secrets ads-login kept: in the
// file ADS_SESSION names, or else in ads-token beside the secrets'
// config.
func (tool command) adsToken() (string, error) {
	path := tool.env(ads.Session)
	if path == "" {
		home := tool.env("HOME")
		if home == "" {
			return "", celestialdata.Refuse(noHome, ads.Session)
		}
		path = secrets.Beside(home, ads.TokenFile)
	}
	token, err := ads.Load(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", celestialdata.Refuse(noTokenKept, path)
	}
	return token, err
}

// noHome is the refusal of a search with no ADS_SESSION and no home to
// find the token under: the variable.
const noHome = "ads finds the token under the home HOME names, or " +
	"where %s says, and neither names one"

// noTokenKept is the refusal of a search with no token kept where it
// looked: the path.
const noTokenKept = "no ads token is kept in %s: run " +
	"celestial-data-secrets ads-login once to keep it"

// searchADS asks ADS once with the token, and says what it found.
func (tool command) searchADS(call pzm.Call, token, query string,
	most int) error {
	asker, err := tool.adsAsker()
	if err != nil {
		return err
	}
	found, err := ads.Search(context.Background(), asker, token, query,
		most, agent)
	var asked *ads.Stopped
	if errors.As(err, &asked) {
		when := until(asked.Reset())
		return fmt.Errorf("ads: ADS asked us to stop%s; nothing more "+
			"was asked", when)
	}
	if err != nil {
		return err
	}
	out, papers := call.Out(), found.Papers()
	fmt.Fprintf(out, "%d papers match; the newest %d:\n", found.Count(),
		len(papers))
	sayPapers(out, papers)
	if left := found.Remaining(); left != "" {
		fmt.Fprintf(out, "ADS allows %s more searches today\n", left)
	}
	return nil
}

// sayPapers writes each paper on a line: its bibcode, its year, its
// first author and its title, saying so when ADS gave no author or no
// title, and its arXiv id last, when it has one, for the papers verb.
func sayPapers(out io.Writer, papers []ads.Paper) {
	for _, paper := range papers {
		author, title := "no author named", paper.Title()
		if authors := paper.Authors(); len(authors) > noAuthors {
			author = authors[firstAuthor]
		}
		if title == "" {
			title = "no title given"
		}
		onArXiv := ""
		if id := paper.ArXiv(); id != "" {
			onArXiv = " (arXiv " + id + ")"
		}
		fmt.Fprintf(out, "  %s %s %s: %s%s\n", paper.Bibcode(),
			paper.Year(), author, title, onArXiv)
	}
}

// The counts sayPapers reads authors by.
const (
	noAuthors   = 0 // defined: none
	firstAuthor = 0 // defined: a list begins at its first
)

// until says when ADS's limit resets, from the reset it gave: as a UTC
// time when it is a count of seconds since 1970, as a header often
// writes one; as it wrote it otherwise; or that it gave no time.
func until(reset string) string {
	if reset == "" {
		return " and gave no time"
	}
	seconds, err := strconv.ParseInt(reset, decimal, wholeBits)
	if err != nil {
		return " until " + reset
	}
	at := time.Unix(seconds, noNanoseconds).UTC()
	return " until " + at.Format(time.RFC3339)
}

// How a reset is read as a whole number, and the time it names.
const (
	decimal       = 10 // defined by strconv: base ten
	wholeBits     = 64 // defined by strconv: an int64
	noNanoseconds = 0  // defined by time.Unix: a whole second
)

// adsAsker is what ADS is asked through: the one a test gives, or the
// network's, with the whole request held to ads-whole.
func (tool command) adsAsker() (ads.Doer, error) {
	if tool.ask != nil {
		return tool.ask, nil
	}
	whole, err := tool.picked.Get("ads-whole")
	if err != nil {
		return nil, err
	}
	return adsClient(web.Seconds(whole)), nil
}

// adsClient is the network's way to ADS: a whole request held to so
// long, and no redirect followed, so a search is one exchange and its
// token goes to the host it was sent to alone. A redirect comes back as
// the status it is, which the search says.
func adsClient(whole time.Duration) *http.Client {
	return &http.Client{Timeout: whole,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}
}
