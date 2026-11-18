package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/ntrs"
	"github.com/janearc/celestial-data/web"
	"github.com/janearc/puzzlemouth/pzm"
	"github.com/janearc/puzzlemouth/verb"
)

// ntrsPage is what ntrs does, as its help says it.
const ntrsPage = "Searches NASA's Technical Reports Server for the " +
	"words given, as its search box takes them. It says how many " +
	"citations match, the first of them in NTRS's order, as many as " +
	"--most or ntrs-most says, each by its NTRS id, year, first " +
	"author and title, and whether it holds a PDF to bring home with " +
	"ntrs-fetch. Public documents need no account and no key. A run " +
	"sends one request, and asks again only after a failure with no " +
	"word from NTRS, as its policy allows. Once NTRS asks us to stop, " +
	"nothing more is asked until the wait it named has passed, and no " +
	"run asks before then; --i-mean-it asks once more anyway, and says " +
	"what NTRS asked for."

// ntrsFetchPage is what ntrs-fetch does, as its help says it.
const ntrsFetchPage = "Brings the PDFs of NTRS citations home by their " +
	"ids, which are digits and nothing else, into the folder --into " +
	"names, each as its id and .pdf. A PDF already there is kept and " +
	"asks NTRS nothing. Every request to NTRS, for a citation's " +
	"details or for its PDF, goes one at a time, each at least two " +
	"seconds after the last began. A citation NTRS does not have, or " +
	"that holds no PDF, is said, and the rest are still asked for. " +
	"When NTRS asks us to stop, or anything else goes wrong in " +
	"asking, the run stops, says what is left, and asks nothing more; " +
	"--i-mean-it asks once more anyway, and says what NTRS asked for. " +
	"A PDF is written to a part and kept only when it is a PDF and " +
	"within the most ntrs-pdf-most allows. With --dry-run the " +
	"citations are looked up, and no PDF is fetched."

// noCitations is how many citations a run has before it counts any.
// It's defined: none.
const noCitations = 0

// ntrsFlags is the flags of ntrs: how many citations to show, and the
// switch that overrides a stop.
func ntrsFlags() []pzm.Flag {
	most := pzm.Value("most", verb.NoShort, "how many citations to "+
		"show, in NTRS's order; ntrs-most when none")
	return append([]pzm.Flag{most}, insisting()...)
}

// ntrsFetchFlags is the flags of ntrs-fetch: where the PDFs go, and the
// switch that overrides a stop.
func ntrsFetchFlags() []pzm.Flag {
	into := pzm.Value("into", verb.NoShort, "the folder to bring PDFs "+
		"into")
	return append([]pzm.Flag{into}, insisting()...)
}

// ntrsVerb searches NTRS for the words given and says what it found.
func (tool command) ntrsVerb(call pzm.Call) error {
	query := strings.TrimSpace(strings.Join(call.Args(), " "))
	if query == "" {
		return celestialdata.Refuse("ntrs needs words to search for")
	}
	picked, err := tool.picked.Get("ntrs-most")
	if err != nil {
		return err
	}
	most, err := count(call, "most", int(picked))
	if err != nil {
		return err
	}
	asker, err := tool.ntrsAsker()
	if err != nil {
		return err
	}
	if err := tool.insist(call); err != nil {
		return err
	}
	found, err := ntrs.Search(context.Background(), asker, query, most,
		agent)
	if err != nil {
		return err
	}
	out, cited := call.Out(), found.Citations()
	fmt.Fprintf(out, "%d citations match; the first %d:\n",
		found.Count(), len(cited))
	sayCitations(out, cited)
	return nil
}

// sayCitations writes each citation on a line: its id, its year, its
// first author and its title, saying so when NTRS gave none of a year,
// an author or a title, and whether it holds a PDF.
func sayCitations(out io.Writer, cited []ntrs.Citation) {
	for _, each := range cited {
		year, author, title := each.Year(), each.FirstAuthor(),
			each.Title()
		if year == "" {
			year = "no year given"
		}
		if author == "" {
			author = "no author named"
		}
		if title == "" {
			title = "no title given"
		}
		pdf := "no PDF"
		if each.HasPDF() {
			pdf = "PDF"
		}
		fmt.Fprintf(out, "  %s %s %s: %s (%s)\n", each.ID(), year,
			author, title, pdf)
	}
}

// ntrsPolicies is the hosts celestial-data registers a policy of their
// own for, by the host's name as web.Hosts spells it. Here, NTRS.
func ntrsPolicies() map[string]web.Policy {
	return map[string]web.Policy{ntrs.Host: ntrs.Policy()}
}

// ntrsAsker is what NTRS is asked through: the one a test gives, or the
// network's, with the whole request held to ntrs-whole, and, when the
// tool has hosts, through the gentle of NTRS's host.
func (tool command) ntrsAsker() (ntrs.Doer, error) {
	doer := tool.reports
	if doer == nil {
		whole, err := tool.picked.Get("ntrs-whole")
		if err != nil {
			return nil, err
		}
		doer = &http.Client{Timeout: web.Seconds(whole)}
	}
	if tool.hosts == nil {
		return doer, nil
	}
	// the doer and the hosts are here, which is all it refuses
	through, _ := web.Through(doer, tool.hosts)
	return web.RewritingDoer(through, nameFlag), nil
}

// ntrsBare is the web NTRS's PDFs are fetched through, with no pace of
// its own: the one a test gives, or the network's, which gives NTRS
// ntrs-pdf-whole for the whole of a PDF, so a PDF that stalls is let go.
func (tool command) ntrsBare() (web.Web, error) {
	if tool.get != nil {
		return tool.get, nil
	}
	wait, waitErr := tool.picked.Get("wait")
	whole, wholeErr := tool.picked.Get("ntrs-pdf-whole")
	if err := errors.Join(waitErr, wholeErr); err != nil {
		return nil, err
	}
	return web.New(web.Seconds(wait), web.Seconds(whole), agent), nil
}

// ntrsWeb is the web a PDF is asked through: the bare one, cut off at
// ntrs-pdf-most, asked through the gentle of NTRS's host.
func (tool command) ntrsWeb() (web.Web, error) {
	bare, bareErr := tool.ntrsBare()
	most, mostErr := tool.picked.Get("ntrs-pdf-most")
	if err := errors.Join(bareErr, mostErr); err != nil {
		return nil, err
	}
	return tool.gentle(cappedWeb{inner: bare, most: int64(most)}), nil
}

// cappedWeb is a web whose files are refused when they are longer than
// the most, by the length the server says or by the bytes that come.
type cappedWeb struct {
	inner web.Web
	most  int64
}

// Get is the inner web's Get, refused when the file is over the most.
func (capped cappedWeb) Get(address string) (io.ReadCloser, int64, error) {
	body, length, err := capped.inner.Get(address)
	if err != nil {
		return body, length, err
	}
	if length > capped.most {
		body.Close()
		return nil, length, capped.over()
	}
	return &cappedBody{body: body, left: capped.most, over: capped.over},
		length, nil
}

// Head is the inner web's Head.
func (capped cappedWeb) Head(address string) (int64, error) {
	return capped.inner.Head(address)
}

// Post is the inner web's Post.
func (capped cappedWeb) Post(address string, form url.Values) (string,
	error) {
	return capped.inner.Post(address, form)
}

// over is the refusal of a file longer than the most.
func (capped cappedWeb) over() error {
	return fmt.Errorf("ntrs: a PDF is over %d bytes, the most "+
		"ntrs-pdf-most allows, and it was not kept", capped.most)
}

// cappedBody is a body that fails once more has been read from it than
// it was allowed.
type cappedBody struct {
	body io.ReadCloser
	left int64
	over func() error
}

// Read reads the body, and fails when more has come than was allowed.
func (capped *cappedBody) Read(into []byte) (int, error) {
	count, err := capped.body.Read(into)
	capped.left -= int64(count)
	if capped.left < 0 {
		return 0, capped.over()
	}
	return count, err
}

// Close closes the body.
func (capped *cappedBody) Close() error { return capped.body.Close() }

// ntrsFetchVerb brings the PDFs of NTRS citations home. It refuses a
// call with no folder, no ids, or an id that is not one, before it asks
// NTRS.
func (tool command) ntrsFetchVerb(call pzm.Call) error {
	into := call.Value("into")
	if into == "" {
		return celestialdata.Refuse("ntrs-fetch needs --into, the " +
			"folder to bring PDFs into")
	}
	ids, err := ntrsIDs(call.Args())
	if err != nil {
		return err
	}
	run := &ntrsRun{out: call.Out(), into: into, dry: call.DryRun()}
	left, err := run.keep(ids)
	if err != nil || len(left) == noCitations {
		return errors.Join(err, run.done())
	}
	if err := tool.insist(call); err != nil {
		return err
	}
	return tool.bringCitations(run, left)
}

// ntrsIDs is the ids the words give, each once, in the order given. It
// refuses none, and the first that is not an id.
func ntrsIDs(words []string) ([]string, error) {
	var ids []string
	for _, word := range words {
		id, err := ntrs.ParseID(word)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == noCitations {
		return nil, celestialdata.Refuse("ntrs-fetch needs ids, " +
			"which are digits")
	}
	return ids, nil
}

// ntrsRun is one run of ntrs-fetch: where the PDFs go, what it says,
// whether it fetches, what it asks through, and how many of each it has.
type ntrsRun struct {
	out                             io.Writer
	into                            string
	dry                             bool
	asker                           ntrs.Doer
	get                             web.Web
	adapter                         *celestialdata.Adapter
	fetched, kept, missing, without int
	asked                           int
}

// ntrsFile is the name a citation's PDF is kept by: its id, and .pdf.
func ntrsFile(id string) string { return id + ".pdf" }

// keep says each id whose PDF is in the folder already, and gives the
// ids that are not. A folder not made yet holds none. A file of a PDF's
// name that is not a PDF is refused, so it is never taken for one.
func (run *ntrsRun) keep(ids []string) ([]string, error) {
	entries, err := os.ReadDir(run.into)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var left []string
	for _, id := range ids {
		name := ntrsFile(id)
		if !inFolder(entries, name) {
			left = append(left, id)
			continue
		}
		if err := beginsAsPDF(run.into, name); err != nil {
			return nil, err
		}
		fmt.Fprintf(run.out, "kept     %s (already there)\n", name)
		run.kept++
	}
	return left, nil
}

// inFolder is whether a folder's entries hold one of a name.
func inFolder(entries []os.DirEntry, name string) bool {
	for _, entry := range entries {
		if entry.Name() == name {
			return true
		}
	}
	return false
}

// bringCitations asks for each citation and fetches the PDF of each that
// holds one, one request at a time, stopping at once when NTRS asks us
// to stop or anything unexpected goes wrong.
func (tool command) bringCitations(run *ntrsRun, left []string) error {
	asker, askErr := tool.ntrsAsker()
	get, getErr := tool.ntrsWeb()
	if err := errors.Join(askErr, getErr); err != nil {
		return err
	}
	// the name is one word and the web is here, which is all it refuses
	adapter, _ := celestialdata.NewAdapter(ntrs.New().Name(), get)
	run.asker, run.get, run.adapter = asker, get, adapter
	var failed []error
	for at, id := range left {
		stop, err := run.bring(id)
		if err != nil {
			failed = append(failed, err)
		}
		if stop {
			failed = append(failed, stoppedAt(left[at+1:]))
			break
		}
	}
	return errors.Join(append(failed, run.done())...)
}

// stoppedAt is what a run says when it stopped with ids not asked for.
// With none, it says nothing.
func stoppedAt(unasked []string) error {
	if len(unasked) == noCitations {
		return nil
	}
	return fmt.Errorf("stopped with %d not asked for: %s. Nothing more "+
		"was asked; run ntrs-fetch again later, and what is here is "+
		"kept", len(unasked), strings.Join(unasked, " "))
}

// bring asks for one citation and fetches its PDF into the folder. On a
// dry run it says it would, and fetches nothing. What came is kept only
// when it is a PDF.
//
// It says whether the run must stop, and why. That is so for NTRS asking
// us to stop and for anything unexpected, and not for a citation NTRS
// does not have or that holds no PDF.
func (run *ntrsRun) bring(id string) (bool, error) {
	run.asked++
	cited, err := ntrs.Lookup(context.Background(), run.asker, id, agent)
	if errors.Is(err, ntrs.ErrNotFound) {
		fmt.Fprintf(run.out, "not on NTRS %s\n", id)
		run.missing++
		return false, fmt.Errorf("%s: %w", id, err)
	}
	if err != nil {
		return true, fmt.Errorf("%s: %w", id, err)
	}
	address, err := cited.PDF()
	if err != nil {
		return false, err
	}
	if address == "" {
		fmt.Fprintf(run.out, "no PDF   %s\n", id)
		run.without++
		return false, celestialdata.Refuse("%s holds no PDF", id)
	}
	label := fmt.Sprintf("%s (%s)", ntrsFile(id), cited.Title())
	if run.dry {
		fmt.Fprintf(run.out, "would fetch %s\n", label)
		run.fetched++
		return false, nil
	}
	// the id and the name are plain and the address is NTRS's own, which
	// is all Product refuses
	product, _ := run.adapter.Product(id, ntrsFile(id), address,
		celestialdata.Unsized, "the PDF of NTRS citation "+id)
	run.asked++
	// the file is checked as a part, so what is not a PDF is never put
	// in place under the PDF's name
	_, err = celestialdata.Fetch(run.get, product.Checked(pdfOnly{}),
		run.into)
	if err != nil {
		return true, fmt.Errorf("%s: %w", id, err)
	}
	fmt.Fprintf(run.out, "fetched  %s\n", label)
	run.fetched++
	return false, nil
}

// pdfOnly is the check that what came is a PDF, which Fetch makes of the
// part before it puts the file in place.
type pdfOnly struct{}

// Check says nothing for a file that begins as a PDF does.
func (pdfOnly) Check(path string) error {
	if beginsAsPDF(filepath.Dir(path), filepath.Base(path)) != nil {
		return errors.New("NTRS sent what is not a PDF, and it was " +
			"not kept")
	}
	return nil
}

// done says what the run did, and how many requests went to NTRS.
func (run *ntrsRun) done() error {
	verbed := "fetched"
	if run.dry {
		verbed = "to fetch"
	}
	_, err := fmt.Fprintf(run.out, "ntrs-fetch: %d %s, %d kept, %d not "+
		"on NTRS, %d with no PDF, in %s; requests to NTRS: %d, one at "+
		"a time\n", run.fetched, verbed, run.kept, run.missing,
		run.without, run.into, run.asked)
	return err
}
