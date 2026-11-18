// Package ntrs asks NASA's Technical Reports Server for citations, one
// search a call, and for the PDF a citation holds.
//
// Public documents need no account and no key. NTRS states no limit, but
// when it answers 429 it is asking us to stop: nothing more is asked.
// There are no retries here, and no loops.
//
// How often NTRS is asked, and whether a failure with no word from it is
// asked again, is the web package's gentle's to say, by Policy; a Doer
// that goes through it, as web.Through gives, asks gently.
//
// The shape of what NTRS asks and answers is the one its own OpenAPI
// description gives, at https://ntrs.nasa.gov/api/openapi-yaml.
//
// A search is a POST of JSON to /api/citations/search. A citation is a
// GET of /api/citations/{id}. A citation lists the files it holds, each
// with the address it is downloaded from.
package ntrs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/janearc/celestial-data"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "ntrs"

// The addresses NTRS keeps its citations at, on the host it keeps for
// them.
const (
	// host is NTRS's own.
	host = "https://" + Host
	// apiPath is where, on the host, its API keeps the citations.
	apiPath = "/api/citations/"
	// citations is the folder of its API that holds the citations.
	citations = host + "/api/citations"
	// search is its search endpoint.
	search = citations + "/search"
	// downloads is where, in a citation's folder, its files are kept.
	downloads = "/downloads/"
)

// pdfMimetype is how NTRS names a download that is a PDF.
const pdfMimetype = "application/pdf"

// The rows a search may ask for, and where a list begins.
const (
	// fewest is the fewest citations a search asks. It's defined: a
	// search of none asks nothing.
	fewest = 1
	// most is the most citations NTRS gives a search: the maximum of
	// the page size its API description states.
	most = 100
	// first is where a list begins. It's defined.
	first = 0
)

// yearWidth is how many characters begin a date NTRS writes, as
// 2009-10-01T00:00:00.0000000+00:00, and are its year. It's defined by
// the date's shape.
const yearWidth = 4

// ErrNotFound is what a lookup's error wraps when NTRS answers 404: it
// has no such citation.
var ErrNotFound = errors.New("ntrs: NTRS answered 404, there is no " +
	"such page")

// Doer is what a request is sent through. An *http.Client is one.
type Doer interface {
	Do(request *http.Request) (*http.Response, error)
}

// Archive is NTRS as celestial-data lists it. NTRS holds reports, not
// observations of a place on the sky, so a circle finds nothing in it:
// it is asked by words, through Search and Lookup.
type Archive struct{}

// New is the archive of NTRS, asked by name only.
func New() *Archive { return &Archive{} }

// Name is the archive's name as the command line takes it.
func (*Archive) Name() string { return adapterName }

// Find is what NTRS holds near a position: nothing, since its citations
// have no place.
func (*Archive) Find(*celestialdata.Near) ([]*celestialdata.Product,
	error) {
	return nil, nil
}

// ByNameOnly says the archive is asked only when it is named.
func (*Archive) ByNameOnly() bool { return true }

// ParseID is an NTRS id from the text given. An id is digits and
// nothing else: it is put into an address, so nothing more is let in.
func ParseID(text string) (string, error) {
	if text == "" || strings.ContainsFunc(text, notDigit) {
		return "", celestialdata.Refuse("%q is not an NTRS id, which "+
			"is digits and nothing else", text)
	}
	return text, nil
}

// notDigit is whether a rune is anything but an ASCII digit.
func notDigit(one rune) bool { return one < '0' || one > '9' }

// Citation is a citation NTRS found.
type Citation struct {
	id     int64
	year   string
	author string
	title  string
	pdf    string
}

// ID is NTRS's number for the citation, as digits.
func (cited Citation) ID() string {
	return strconv.FormatInt(cited.id, decimal)
}

// decimal is the base NTRS writes its ids in. It's defined by strconv.
const decimal = 10

// Year is the year the citation was published, as NTRS writes it, or
// none when NTRS gave no date.
func (cited Citation) Year() string { return cited.year }

// FirstAuthor is the author NTRS puts first, or none when it names no
// author.
func (cited Citation) FirstAuthor() string { return cited.author }

// Title is the citation's title.
func (cited Citation) Title() string { return cited.title }

// HasPDF is whether the citation lists a PDF to download.
func (cited Citation) HasPDF() bool { return cited.pdf != "" }

// PDF is the address the citation's PDF is downloaded from, or none
// when it has none. It refuses an address that NTRS wrote that is not
// a file in this citation's own folder of downloads, so no other
// address is ever fetched for it.
func (cited Citation) PDF() (string, error) {
	if cited.pdf == "" {
		return "", nil
	}
	folder := apiPath + cited.ID() + downloads
	name, kept := strings.CutPrefix(cited.pdf, folder)
	if !kept || !plainName(name) || !plainUnescaped(name) ||
		strings.ContainsAny(name, "?#") {
		return "", celestialdata.Refuse("ntrs: the PDF address NTRS "+
			"gave for %s is not a file of its own: %q", cited.ID(),
			cited.pdf)
	}
	return host + cited.pdf, nil
}

// plainName is whether a name is one file's, and not a way out of its
// folder.
func plainName(name string) bool {
	return name != "" && name == path.Base(name) && name != "." &&
		name != ".." && !strings.ContainsAny(name, `\`)
}

// plainUnescaped is whether a name is still one file's when its percent
// escapes are read, as a server may read them. A name that will not
// read is not.
func plainUnescaped(name string) bool {
	read, err := url.PathUnescape(name)
	return err == nil && plainName(read)
}

// Found is what a search found.
type Found struct {
	count     int
	citations []Citation
}

// Count is how many citations NTRS says match the query. It may be more
// than the citations it gave.
func (found *Found) Count() int { return found.count }

// Citations are the citations NTRS gave, in its order. The list is a
// copy.
func (found *Found) Citations() []Citation {
	return slices.Clone(found.citations)
}

// wire is the shape of one citation in NTRS's answers, as its API
// description gives it. Only what is read is named.
type wire struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Authors []struct {
		Sequence int `json:"sequence"`
		Meta     struct {
			Author struct {
				Name string `json:"name"`
			} `json:"author"`
		} `json:"meta"`
	} `json:"authorAffiliations"`
	Publications []struct {
		Date string `json:"publicationDate"`
	} `json:"publications"`
	Downloads []struct {
		Mimetype string `json:"mimetype"`
		Links    struct {
			PDF      string `json:"pdf"`
			Original string `json:"original"`
		} `json:"links"`
	} `json:"downloads"`
}

// citation is the citation a wire holds.
func (said wire) citation() Citation {
	return Citation{id: said.ID, year: said.year(),
		author: said.firstAuthor(), title: said.Title, pdf: said.pdf()}
}

// year is the year of the first publication date NTRS gives, or none.
func (said wire) year() string {
	if len(said.Publications) == first {
		return ""
	}
	date := said.Publications[first].Date
	if len(date) < yearWidth {
		return ""
	}
	if _, err := strconv.Atoi(date[:yearWidth]); err != nil {
		return ""
	}
	return date[:yearWidth]
}

// firstAuthor is the named author of the least sequence, or none.
func (said wire) firstAuthor() string {
	name, least, named := "", first, false
	for _, author := range said.Authors {
		each := author.Meta.Author.Name
		if each != "" && (!named || author.Sequence < least) {
			name, least, named = each, author.Sequence, true
		}
	}
	return name
}

// pdf is the address, as NTRS wrote it, of the first download that is a
// PDF, or none.
func (said wire) pdf() string {
	for _, file := range said.Downloads {
		if file.Mimetype != pdfMimetype {
			continue
		}
		if file.Links.PDF != "" {
			return file.Links.PDF
		}
		if file.Links.Original != "" {
			return file.Links.Original
		}
	}
	return ""
}

// question is the body of a search, as the API description gives it.
type question struct {
	Q    string `json:"q"`
	Page page   `json:"page"`
}

// page is which citations of a search to give: so many, from the first.
type page struct {
	Size int `json:"size"`
	From int `json:"from"`
}

// Search asks NTRS one question and gives what it found: at most so
// many citations, in the order NTRS ranks them. It sends one request and
// does not retry. It refuses an empty query, and a number of citations
// below one or above what NTRS gives, before it sends anything.
//
// A 429 is NTRS asking us to stop: its error is a *Stopped, for
// errors.As. Any other status but 200 is an error that names it. The
// agent is how the request names itself.
func Search(ctx context.Context, doer Doer, query string, asked int,
	agent string) (*Found, error) {
	switch {
	case query == "":
		return nil, errors.New("ntrs: a search needs a query")
	case asked < fewest || asked > most:
		return nil, fmt.Errorf("ntrs: a search asks from %d to %d "+
			"citations, not %d", fewest, most, asked)
	}
	// a question is made of a string and two numbers, which always encode
	body, _ := json.Marshal(question{Q: query,
		Page: page{Size: asked, From: first}})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		search, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ntrs: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	answer, err := send(doer, request, agent)
	if err != nil {
		return nil, err
	}
	defer answer.Body.Close()
	raw, err := readAnswer(answer.Body)
	if err != nil {
		return nil, err
	}
	return read(raw)
}

// readAnswer is the whole of an answer's body, refused when it is longer
// than AnswerLimit, so a server that never stops cannot fill memory.
func readAnswer(body io.Reader) ([]byte, error) {
	limit := AnswerLimit()
	raw, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("ntrs: the answer was cut off: %w", err)
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("ntrs: the answer is over %d "+
			"bytes, the "+
			"most ntrs-answer-most allows", limit)
	}
	return raw, nil
}

// Lookup asks NTRS for one citation by its id, which it takes only as
// ParseID does. It sends one request and does not retry. A 404 is an
// error that wraps ErrNotFound, and a 429 a *Stopped.
func Lookup(ctx context.Context, doer Doer, id, agent string) (*Citation,
	error) {
	id, err := ParseID(id)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		citations+"/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("ntrs: %w", err)
	}
	answer, err := send(doer, request, agent)
	if err != nil {
		return nil, err
	}
	defer answer.Body.Close()
	raw, err := readAnswer(answer.Body)
	if err != nil {
		return nil, err
	}
	var said wire
	if err := json.Unmarshal(raw, &said); err != nil {
		return nil, fmt.Errorf("ntrs: the answer is not JSON: %w", err)
	}
	cited := said.citation()
	return &cited, nil
}

// send sends a request and gives the answer when it is a 200. The caller
// closes the body.
func send(doer Doer, request *http.Request, agent string) (*http.Response,
	error) {
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", agent)
	answer, err := doer.Do(request)
	if err != nil {
		return nil, fmt.Errorf("ntrs: the request failed: %w", err)
	}
	switch answer.StatusCode {
	case http.StatusOK:
		return answer, nil
	case http.StatusNotFound:
		answer.Body.Close()
		return nil, ErrNotFound
	case http.StatusTooManyRequests:
		answer.Body.Close()
		return nil, &Stopped{wait: answer.Header.Get("Retry-After")}
	}
	answer.Body.Close()
	return nil, fmt.Errorf("ntrs: NTRS answered with status %d",
		answer.StatusCode)
}

// results is the shape of a search's answer. The results are a pointer
// so an answer without them can be told from one that found nothing.
type results struct {
	Results *[]wire `json:"results"`
	Stats   struct {
		Total int `json:"total"`
	} `json:"stats"`
}

// read is what a 200 answer to a search found.
func read(raw []byte) (*Found, error) {
	var said results
	if err := json.Unmarshal(raw, &said); err != nil {
		return nil, fmt.Errorf("ntrs: the answer is not JSON: %w", err)
	}
	if said.Results == nil {
		return nil, errors.New("ntrs: the answer has no results")
	}
	found := &Found{count: said.Stats.Total}
	for _, one := range *said.Results {
		found.citations = append(found.citations, one.citation())
	}
	return found, nil
}

// Stopped is NTRS asking us to stop: it answered 429. It holds the wait
// NTRS asked, as it wrote it, or none.
type Stopped struct {
	wait string
}

// Wait is the Retry-After NTRS gave, as it wrote it, or none.
func (stopped *Stopped) Wait() string { return stopped.wait }

// Error says NTRS asked us to stop, and for how long if it said.
func (stopped *Stopped) Error() string {
	if stopped.wait == "" {
		return "ntrs: NTRS asked us to stop and gave no time"
	}
	return "ntrs: NTRS asked us to stop, and to wait " + stopped.wait
}
