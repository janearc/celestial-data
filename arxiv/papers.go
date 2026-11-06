// Package arxiv finds papers on arXiv by their ids and names their PDFs
// as products to fetch, gently.
//
// Every request, to the API and for a PDF, goes through one Gate: one
// at a time, three seconds apart at least, as arXiv's API terms ask.
// When arXiv answers 403, 429 or 503 it is asking us to stop, and
// nothing more is asked.
//
// A gate held through a record keeps the pace, and any wait arXiv asked
// for, from one run to the next.
//
// The API is asked through github.com/Epistemic-Technology/arxiv. Its
// v1.0.1 paces by three nanoseconds, replaces a limiter it is given,
// and parses a 429 as results; so its pacing is turned off, the Gate
// paces instead, and the answer's status is read here before it is.
package arxiv

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	library "github.com/Epistemic-Technology/arxiv/arxiv"
	"github.com/janearc/celestial-data"
)

// API is the address of arXiv's API on export.arxiv.org, the host arXiv
// sets aside for programs.
const API = "https://export.arxiv.org/api/query"

// pdfs is where a paper's PDF is fetched from, on the same host.
const pdfs = "https://export.arxiv.org/pdf/"

// Searcher is what papers are looked up through. It gives the API's
// answer raw, as the library's Client.RawSearch does.
type Searcher interface {
	RawSearch(ctx context.Context,
		params library.SearchParams) (*http.Response, error)
}

// NewSearcher is the library's client asking the API at an address,
// waiting so long for an answer. Its own pacing is off, since a Gate
// paces every request to arXiv, and so are its retries: a server that
// asks us to wait is waited on, not asked again.
func NewSearcher(address string, wait time.Duration) Searcher {
	return library.NewClient(library.WithBaseURL(address),
		library.WithRateLimit(unpaced), library.WithTimeout(wait))
}

// unpaced is the library's rate limit for none. It's defined by the
// library: at zero it makes no limiter of its own.
const unpaced time.Duration = 0

// oneID is the fewest ids a query asks. It's defined: a query of none
// asks nothing.
const oneID = 1

// firstID is where a look begins among its ids. It's defined: the first.
const firstID = 0

// Papers finds papers on arXiv, so many ids a query. Each query goes
// through the gate.
type Papers struct {
	adapter *celestialdata.Adapter
	search  Searcher
	gate    *Gate
	most    int
}

// New is papers found through a searcher and a gate, most ids a query.
// It refuses a missing searcher or gate, and fewer than one id a query.
func New(search Searcher, gate *Gate, most int) (*Papers, error) {
	// the name is one word, which is all Named refuses
	adapter, _ := celestialdata.Named("arxiv")
	if search == nil || gate == nil {
		return nil, adapter.Refuse("needs a searcher and a gate")
	}
	if most < oneID {
		return nil, adapter.Refuse("asks at least one id a query, "+
			"not %d", most)
	}
	return &Papers{adapter: adapter, search: search, gate: gate,
		most: most}, nil
}

// Look is the papers the ids name, as products to fetch, and the ids
// arXiv has no paper for. Each paper is named for the version arXiv
// has: its latest, unless the id names one.
//
// A version named twice, as 1809.03507 and 1809.03507v2 may, is found
// once. When a query fails, what was found before it comes back beside
// the error.
func (papers *Papers) Look(ctx context.Context, ids []ID) (
	[]*celestialdata.Product, []ID, error) {
	var found []*celestialdata.Product
	var missing []ID
	seen := map[string]bool{}
	for start := firstID; start < len(ids); start += papers.most {
		batch := ids[start:min(start+papers.most, len(ids))]
		entries, err := papers.ask(ctx, batch)
		if err != nil {
			return found, missing, err
		}
		products, absent, err := papers.match(batch, entries)
		if err != nil {
			return found, missing, err
		}
		for _, product := range products {
			if !seen[product.ID()] {
				seen[product.ID()] = true
				found = append(found, product)
			}
		}
		missing = append(missing, absent...)
	}
	return found, missing, nil
}

// ask is one query for a batch of ids, through the gate. It gives
// arXiv's entries, or why there are none.
func (papers *Papers) ask(ctx context.Context, batch []ID) (
	[]library.EntryMetadata, error) {
	if err := papers.gate.enter(ctx); err != nil {
		return nil, err
	}
	defer papers.gate.leave()
	names := make([]string, len(batch))
	for at, id := range batch {
		names[at] = id.String()
	}
	answer, err := papers.search.RawSearch(ctx, library.SearchParams{
		IdList: names, MaxResults: len(names)})
	if err != nil {
		return nil, papers.adapter.Wrap(err)
	}
	defer answer.Body.Close()
	entries, err := papers.read(answer)
	var asked *halt
	if errors.As(err, &asked) {
		papers.gate.stopUntil(asked.retryAfter)
	}
	return entries, err
}

// read is the entries of an answer. A 403, 429 or 503 is arXiv asking
// us to stop. A 400 is arXiv refusing what was asked, in its own words.
func (papers *Papers) read(answer *http.Response) (
	[]library.EntryMetadata, error) {
	switch answer.StatusCode {
	case http.StatusOK, http.StatusBadRequest:
	case http.StatusForbidden, http.StatusTooManyRequests,
		http.StatusServiceUnavailable:
		return nil, &halt{status: answer.Status,
			retryAfter: answer.Header.Get("Retry-After")}
	default:
		return nil, papers.adapter.Fail("the API answered %s",
			answer.Status)
	}
	results, err := library.ParseResponse(answer.Body)
	if err != nil {
		return nil, papers.adapter.Wrap(err)
	}
	if answer.StatusCode == http.StatusBadRequest {
		return nil, papers.refused(answer.Status, results.Entries)
	}
	return results.Entries, nil
}

// refused is arXiv's refusal of a query, in the words of its entries.
func (papers *Papers) refused(status string,
	entries []library.EntryMetadata) error {
	words := []string{status}
	for _, entry := range entries {
		words = append(words, strings.TrimSpace(entry.Summary))
	}
	return papers.adapter.Refuse("was refused by the API: %s",
		strings.Join(words, "; "))
}

// entry is an entry arXiv answered, with the id it is for.
type entry struct {
	id   *ID
	meta library.EntryMetadata
}

// match is the products for a batch's ids, and the ids no entry is
// for. arXiv answers in its own order, not the batch's.
//
// An id with a version is that version's entry; one without is the
// latest arXiv answered. A version arXiv did not answer is missing, as
// a paper it does not have is. An entry that is not a paper is refused.
func (papers *Papers) match(batch []ID,
	entries []library.EntryMetadata) ([]*celestialdata.Product, []ID,
	error) {
	latest, exact, err := papers.answered(entries)
	if err != nil {
		return nil, nil, err
	}
	var found []*celestialdata.Product
	var missing []ID
	for _, id := range batch {
		got, ok := latest[id.name]
		if id.version != unversioned {
			got, ok = exact[id]
		}
		if !ok {
			missing = append(missing, id)
			continue
		}
		found = append(found, papers.product(got))
	}
	return found, missing, nil
}

// answered is arXiv's entries, by name the latest version of each, and
// by id every version. It refuses an entry that is not a paper.
func (papers *Papers) answered(entries []library.EntryMetadata) (
	map[string]entry, map[ID]entry, error) {
	latest, exact := map[string]entry{}, map[ID]entry{}
	for _, meta := range entries {
		id, err := ParseID(meta.ID)
		if err != nil {
			return nil, nil, papers.adapter.Refuse("answered %q, "+
				"which is not a paper: %s", meta.ID,
				strings.TrimSpace(meta.Summary))
		}
		got := entry{id: id, meta: meta}
		exact[*id] = got
		if before, ok := latest[id.name]; !ok ||
			id.version > before.id.version {
			latest[id.name] = got
		}
	}
	return latest, exact, nil
}

// product is an entry as a product: the version's PDF, kept as its
// file, with who wrote it, when, and its title.
func (papers *Papers) product(found entry) *celestialdata.Product {
	// an id arXiv answered with is versioned, so its file is one plain
	// name, and the address is not empty: nothing Product refuses
	product, _ := papers.adapter.Product(found.id.String(),
		found.id.file(), pdfs+found.id.String(), celestialdata.Unsized,
		about(found.meta))
	return product
}

// about is who wrote a paper, the year it was first published, and its
// title on one line.
func about(meta library.EntryMetadata) string {
	title := strings.Join(strings.Fields(meta.Title), " ")
	return fmt.Sprintf("%s %d: %s", authors(meta.Authors),
		meta.Published.Year(), title)
}

// The counts of authors a citation writes by name: one is named, two
// are joined by and, and more are the first and et al.
const (
	noAuthor   = 0 // defined: none named
	oneAuthor  = 1 // defined: named alone
	twoAuthors = 2 // defined: the most a citation names
)

// authors is a paper's authors as a citation writes them: one surname,
// two joined by and, or the first and et al.
func authors(names []library.Author) string {
	surnames := make([]string, len(names))
	for at, name := range names {
		surnames[at] = surname(name)
	}
	switch len(names) {
	case noAuthor:
		return "no author named"
	case oneAuthor, twoAuthors:
		return strings.Join(surnames, " and ")
	}
	return strings.Join(surnames[:oneAuthor], "") + " et al."
}

// surname is the last word of an author's name, or a word saying there
// is none.
func surname(author library.Author) string {
	for _, word := range slices.Backward(strings.Fields(author.Name)) {
		return word
	}
	return "someone unnamed"
}

// halt is arXiv asking us to stop: an answer of 403, 429 or 503, or a
// wait it asked for that has not passed. It holds what was said, and
// the wait its Retry-After asked as written, or none.
type halt struct {
	status, retryAfter string
}

// Error says arXiv asked us to stop, and how long when it said.
func (asking *halt) Error() string {
	if asking.retryAfter == "" {
		return "arXiv asked us to stop: " + asking.status
	}
	return fmt.Sprintf("arXiv asked us to stop: %s; its Retry-After "+
		"is %s", asking.status, asking.retryAfter)
}

// AskedToStop is whether an error is arXiv asking us to stop: a 403,
// 429 or 503 from the API or a PDF fetched through a paced web, or a
// wait arXiv asked for that has not passed.
func AskedToStop(err error) bool {
	var asking *halt
	return errors.As(err, &asking)
}
