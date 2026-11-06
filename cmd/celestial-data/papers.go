package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/arxiv"
	"github.com/janearc/celestial-data/web"
	"github.com/janearc/puzzlemouth/pzm"
)

// papersPage is what papers does, as its help says it.
const papersPage = "Brings papers home from arXiv by their ids, given " +
	"as words or one a line in the file --list names, into the folder " +
	"--into names. An id is written 1809.03507 or astro-ph/0602086, " +
	"with a version such as v2 or without, or as arXiv: or an " +
	"arxiv.org address. A paper already there, any version of it for " +
	"an id without one, is kept and asks arXiv nothing. Every request " +
	"to arXiv, for a paper's details or its PDF, goes one at a time, " +
	"at least three seconds after the last began, as arXiv asks, and " +
	"from one run to the next for this user on this machine: the gate " +
	"keeps its record in the user's cache folder, and its lock " +
	"refuses a second run while one is asking. When arXiv answers " +
	"403, 429 or 503 it is asking us to stop: the run stops, says " +
	"what is left, and asks nothing more, and no run asks again until " +
	"the wait it asked has passed. With --dry-run the papers are " +
	"looked up through the API, the record is kept, and none is " +
	"fetched."

// comment begins the part of a --list line that is not an id.
const comment = "#"

// noPapers is how many papers a run has before it counts any. It's
// defined: none.
const noPapers = 0

// papersVerb brings papers home from arXiv. It refuses a call with no
// folder, no ids, or an id that is not one, before it asks arXiv.
func (tool command) papersVerb(call pzm.Call) error {
	into := call.Value("into")
	if into == "" {
		return celestialdata.Refuse("papers needs --into, the folder " +
			"to bring papers into")
	}
	wanted, err := idsOf(call.Args(), call.Value("list"))
	if err != nil {
		return err
	}
	run := &papersRun{out: call.Out(), into: into, dry: call.DryRun()}
	left, err := run.keep(wanted)
	if err != nil || len(left) == noPapers {
		return errors.Join(err, run.done(nil))
	}
	return tool.fetchPapers(run, left)
}

// idsOf is the ids the words give and the list file holds, each once,
// in the order given. It refuses none, and the first that is not an id.
func idsOf(words []string, list string) ([]arxiv.ID, error) {
	written, err := listed(list)
	if err != nil {
		return nil, err
	}
	written = append(words, written...)
	seen := map[arxiv.ID]bool{}
	var ids []arxiv.ID
	for _, each := range written {
		id, err := arxiv.ParseID(each)
		if err != nil {
			return nil, err
		}
		if !seen[*id] {
			seen[*id] = true
			ids = append(ids, *id)
		}
	}
	if len(ids) == noPapers {
		return nil, celestialdata.Refuse("papers needs ids, as words " +
			"or one a line in the file --list names")
	}
	return ids, nil
}

// listed is the ids a list file holds, one a line: what comes before a
// # on it, with blank lines passed over. No file is no ids.
func listed(list string) ([]string, error) {
	if list == "" {
		return nil, nil
	}
	file, err := os.Open(list)
	if err != nil {
		return nil, fmt.Errorf("papers could not read --list: %w", err)
	}
	defer file.Close()
	var written []string
	lines := bufio.NewScanner(file)
	for lines.Scan() {
		before, _, _ := strings.Cut(lines.Text(), comment)
		if id := strings.TrimSpace(before); id != "" {
			written = append(written, id)
		}
	}
	return written, lines.Err()
}

// papersRun is one run of papers: where they go, what it says, whether
// it fetches, and how many of each it has.
type papersRun struct {
	out                    io.Writer
	into                   string
	dry                    bool
	fetched, kept, missing int
}

// keep says each id whose paper is in the folder already, and gives the
// ids that are not. A folder not made yet holds none. A file of a
// paper's name that is not a PDF is refused, so it is never taken for
// the paper.
func (run *papersRun) keep(ids []arxiv.ID) ([]arxiv.ID, error) {
	entries, err := os.ReadDir(run.into)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var left []arxiv.ID
	for _, id := range ids {
		file, ok := heldBy(id, entries)
		if !ok {
			left = append(left, id)
			continue
		}
		if err := run.isPDF(file); err != nil {
			return nil, err
		}
		fmt.Fprintf(run.out, "kept     %s (already there)\n", file)
		run.kept++
	}
	return left, nil
}

// pdfMark is how every PDF begins.
const pdfMark = "%PDF-"

// isPDF refuses a file in the folder that does not begin as a PDF does,
// and says how to clear it.
func (run *papersRun) isPDF(name string) error {
	head := make([]byte, len(pdfMark))
	file, err := os.Open(filepath.Join(run.into, name))
	if err == nil {
		_, err = io.ReadFull(file, head)
		file.Close()
	}
	if err != nil || string(head) != pdfMark {
		return celestialdata.Refuse("%s in %s is not a PDF (%v). Move "+
			"it aside and run again", name, run.into, err)
	}
	return nil
}

// heldBy is the file among a folder's entries that holds an id's paper.
func heldBy(id arxiv.ID, entries []os.DirEntry) (string, bool) {
	for _, entry := range entries {
		if id.Holds(entry.Name()) {
			return entry.Name(), true
		}
	}
	return "", false
}

// fetchPapers looks the ids up and fetches what is found. Every request
// goes through one gate, at the pace the numbers say. The gate holds its
// record for the run: the pace and any wait arXiv asked for carry over
// to the next run, and no other run asks while this one does.
func (tool command) fetchPapers(run *papersRun, left []arxiv.ID) error {
	gate, papers, get, err := tool.arxivOf()
	if err != nil {
		return err
	}
	record, err := tool.recordPath()
	if err != nil {
		return err
	}
	if err := gate.Hold(record); err != nil {
		return err
	}
	failed := tool.lookAndBring(run, papers, get, left)
	failed = append(failed, run.done(gate), gate.Release())
	return errors.Join(failed...)
}

// lookAndBring looks the ids up and brings each paper found, stopping
// at once when arXiv asks us to stop. It gives what went wrong. When
// the API fails, no PDF is asked for, not even of what was found
// before: an API that fails is not asked for more.
func (tool command) lookAndBring(run *papersRun, papers *arxiv.Papers,
	get web.Web, left []arxiv.ID) []error {
	found, missing, err := papers.Look(context.Background(), left)
	if err != nil {
		return []error{tool.stopped(err, left)}
	}
	failed := run.missed(missing)
	for at, product := range found {
		err := run.bring(get, product)
		if arxiv.AskedToStop(err) {
			unfetched := idsOfProducts(found[at:])
			return append(failed, tool.stopped(err, unfetched))
		}
		failed = append(failed, err)
	}
	return failed
}

// recordPath is where the gate's record is kept between runs: where a
// test puts it, or arxiv-gate in celestial-data's folder of the user's
// cache. It refuses when the user has no cache folder.
func (tool command) recordPath() (string, error) {
	if tool.record != "" {
		return tool.record, nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("papers keeps arXiv's pace between runs "+
			"in the user's cache folder, and finds none: %w", err)
	}
	return filepath.Join(cache, "celestial-data", "arxiv-gate"), nil
}

// arxivOf is what papers needs to ask arXiv: a gate at the pace the
// numbers say, papers found through it so many ids a query, and the web
// that fetches the PDFs through it.
func (tool command) arxivOf() (*arxiv.Gate, *arxiv.Papers, web.Web,
	error) {
	pace, paceErr := tool.picked.Get("arxiv-pace")
	most, mostErr := tool.picked.Get("arxiv-most")
	wait, waitErr := tool.picked.Get("wait")
	get, getErr := tool.web()
	if err := errors.Join(paceErr, mostErr, waitErr, getErr); err != nil {
		return nil, nil, nil, err
	}
	gate, err := arxiv.NewGate(web.Seconds(pace), tool.clockOf())
	if err != nil {
		return nil, nil, nil, err
	}
	papers, err := arxiv.New(tool.searcherOf(wait), gate, int(most))
	if err != nil {
		return nil, nil, nil, err
	}
	// the context, web and gate are all here, which is all it refuses
	paced, _ := arxiv.Paced(context.Background(), get, gate)
	return gate, papers, paced, nil
}

// clockOf is the clock a test gives, or the wall's.
func (tool command) clockOf() arxiv.Clock {
	if tool.clock != nil {
		return tool.clock
	}
	return arxiv.Wall()
}

// searcherOf returns the searcher a test provides or the client that
// asks arXiv's API, waiting the given seconds.
func (tool command) searcherOf(wait float64) arxiv.Searcher {
	if tool.search != nil {
		return tool.search
	}
	return arxiv.NewSearcher(arxiv.API, web.Seconds(wait))
}

// stopped is why a run stopped and which ids it had not fetched. When
// arXiv asked us to stop, it says nothing more was asked, and how to go
// on.
func (tool command) stopped(err error, left []arxiv.ID) error {
	if !arxiv.AskedToStop(err) {
		return err
	}
	names := make([]string, len(left))
	for at, id := range left {
		names[at] = id.String()
	}
	return fmt.Errorf("%w. Stopped with %d not fetched: %s. Nothing "+
		"more was asked; run papers again later, and what is here is "+
		"kept", err, len(left), strings.Join(names, " "))
}

// idsOfProducts is the ids of products, as arXiv names them.
func idsOfProducts(products []*celestialdata.Product) []arxiv.ID {
	var ids []arxiv.ID
	for _, product := range products {
		// a product's id is one arXiv answered with, so it reads
		id, _ := arxiv.ParseID(product.ID())
		ids = append(ids, *id)
	}
	return ids
}

// missed says each id arXiv has no paper for. It gives one error that
// names them all, or none.
func (run *papersRun) missed(missing []arxiv.ID) []error {
	var names []string
	for _, id := range missing {
		fmt.Fprintf(run.out, "not on arXiv %s\n", id)
		names = append(names, id.String())
		run.missing++
	}
	if len(names) == noPapers {
		return nil
	}
	return []error{celestialdata.Refuse("%d not on arXiv: %s",
		len(names), strings.Join(names, " "))}
}

// bring fetches one paper into the folder. On a dry run it says it
// would, and fetches nothing. What came is kept only when it is a PDF.
func (run *papersRun) bring(get web.Web,
	product *celestialdata.Product) error {
	label := fmt.Sprintf("%s (%s)", product.Name(), product.About())
	if run.dry {
		fmt.Fprintf(run.out, "would fetch %s\n", label)
		run.fetched++
		return nil
	}
	home, err := celestialdata.Fetch(get, product, run.into)
	if err != nil {
		return fmt.Errorf("%s: %w", product.ID(), err)
	}
	if err := run.isPDF(product.Name()); err != nil {
		os.Remove(home)
		return fmt.Errorf("%s: arXiv sent what is not a PDF, and it "+
			"was not kept", product.ID())
	}
	fmt.Fprintf(run.out, "fetched  %s\n", label)
	run.fetched++
	return nil
}

// done says what the run did, and how many requests went to arXiv
// through the gate. With no gate there were none.
func (run *papersRun) done(gate *arxiv.Gate) error {
	asked := noPapers
	if gate != nil {
		asked = gate.Asked()
	}
	verbed := "fetched"
	if run.dry {
		verbed = "to fetch"
	}
	_, err := fmt.Fprintf(run.out, "papers: %d %s, %d kept, %d not on "+
		"arXiv, in %s; requests to arXiv: %d, one at a time\n",
		run.fetched, verbed, run.kept, run.missing, run.into, asked)
	return err
}
