package main

import (
	_ "embed"
	"io"
	"os"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/ads"
	"github.com/janearc/celestial-data/arxiv"
	"github.com/janearc/celestial-data/dasch"
	"github.com/janearc/celestial-data/gaia"
	"github.com/janearc/celestial-data/web"
	"github.com/janearc/puzzlemouth/pzm"
	"github.com/janearc/puzzlemouth/verb"
)

// numbersFile is the numbers file celestial-data picks its settings from.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// agent is what celestial-data says it is to the archives it asks.
const agent = "celestial-data"

// command is celestial-data: the web its archives are asked through, the
// numbers it picks when a call gives none, the environment it reads the
// folder of a set and the gaia session from, and the sets it knows.
//
// signed is the web that keeps a session. A test gives it; run gives
// the network.
//
// search, clock and record are what papers asks arXiv's API through,
// the clock its gate keeps and where the gate's record is kept between
// runs. A test gives them; run gives the library's client, the wall's
// clock and the user's cache folder.
//
// ask is what ads asks ADS through. A test gives it; run gives the
// network.
//
// curves is what dasch asks DASCH's API through, to find a place's
// sources and to fetch their light curves. A test gives it; run gives
// the network.
type command struct {
	get    web.Web
	picked *celestialdata.Measured
	lookup func(name string) string
	sets   map[string]maker
	signed session
	search arxiv.Searcher
	clock  arxiv.Clock
	record string
	ask    ads.Doer
	curves dasch.Doer
}

// run runs celestial-data with its words, input and output given. Its archives
// are asked through the network, with the waits it picks. It reads the
// process's environment.
func run(args []string, in io.Reader, out, errs io.Writer) int {
	picked := celestialdata.NewMeasured(numbersFile, numbersRaw)
	tool := command{picked: picked, lookup: os.Getenv,
		sets: standardSets()}
	return tool.run(args, in, out, errs)
}

// run runs the command through pzm.
func (tool command) run(args []string, in io.Reader, out,
	errs io.Writer) int {
	return tool.declared().RunWith(args, in, out, errs)
}

// web is the web the archives are asked through. Commands given none
// ask the network and wait as long as the numbers say, with the gaia
// archive's session when GAIA_SESSION names one.
func (tool command) web() (web.Web, error) {
	if tool.get != nil {
		return tool.get, nil
	}
	client, err := tool.client()
	if err != nil {
		return nil, err
	}
	if err := tool.sendSession(client); err != nil {
		return nil, err
	}
	return client, nil
}

// client is a web that keeps the cookies its answers set: the one a
// test gives, or the network with the waits the numbers say.
func (tool command) client() (session, error) {
	if tool.signed != nil {
		return tool.signed, nil
	}
	wait, err := tool.picked.Get("wait")
	if err != nil {
		return nil, err
	}
	return web.Patient(web.Seconds(wait), agent), nil
}

// sendSession gives a client the gaia archive's session, when
// GAIA_SESSION names one.
func (tool command) sendSession(client session) error {
	path := tool.env(gaiaSession)
	if path == "" {
		return nil
	}
	cookies, err := gaia.Load(path)
	if err != nil {
		return err
	}
	return client.Keep(gaia.Service, cookies)
}

// The facts of celestial-data's verbs.
var (
	// listing is a verb that only reads what celestial-data knows.
	listing = verb.NewFacts(verb.WritesNothing, verb.LeavesAlone,
		verb.ThisMachine, verb.NoOne, verb.Light)
	// asking is a verb that asks the archives and writes nothing.
	asking = verb.NewFacts(verb.WritesNothing, verb.LeavesAlone,
		verb.Outside, verb.NoOne, verb.Light)
	// fetching is a verb that brings files home and replaces none.
	fetching = bringing("the folder --into names")
	// fetchingSet is a verb that brings a set's files home and replaces
	// none.
	fetchingSet = bringing("the folder --into or the set's _DATA names")
	// fetchingPapers is a verb that brings papers home, replaces none,
	// and keeps the gate's record in the user's cache folder.
	fetchingPapers = bringing("the folder --into names, and the " +
		"gate's record in the user's cache folder")
)

// bringing is the facts of a verb that brings files home from the
// archives into a place, and replaces none.
func bringing(place string) verb.Facts {
	where := verb.WritesTo(place)
	return verb.NewFacts(where, verb.LeavesAlone, verb.Outside,
		verb.NoOne, verb.Light)
}

// declared is celestial-data and its verbs, as pzm takes them. Each verb's
// example is the function Example_ and its name in this package's tests.
func (tool command) declared() *pzm.Tool {
	declared := pzm.New("celestial-data", "finds and fetches what "+
		"astronomy "+
		"archives hold near a place on the sky", pzm.Build(build),
		pzm.Built(built), pzm.Who(who), pzm.Sessions(sessions))
	declared.Verb("archives", verb.NoShort, "lists the archives "+
		"celestial-data "+
		"asks", archivesPage, "Example_archives", listing,
		tool.archivesVerb)
	declared.Verb("find", verb.NoShort, "names what archives hold near a "+
		"place", findPage, "Example_find", asking, tool.findVerb,
		searching()...)
	into := pzm.Value("into", verb.NoShort, "the folder to bring files "+
		"into")
	fetchFlags := append(searching(), into,
		pzm.Value("match", verb.NoShort, "only files whose name holds "+
			"this"))
	declared.Verb("fetch", verb.NoShort, "brings what archives hold near "+
		"a place home", fetchPage, "Example_fetch", fetching,
		tool.fetchVerb, fetchFlags...)
	declared.Verb("fetch-set", verb.NoShort, "brings the files of a "+
		"named set home", fetchSetPage, "Example_fetchSet", fetchingSet,
		tool.setVerb, into)
	declared.Verb("papers", verb.NoShort, "brings papers home from "+
		"arXiv, gently", papersPage, "Example_papers", fetchingPapers,
		tool.papersVerb, pzm.Value("into", verb.NoShort, "the folder "+
			"to bring papers into"), pzm.Value("list", verb.NoShort,
			"a file of ids, one a line; what follows a # is not "+
				"read"))
	declared.Verb("ads", verb.NoShort, "searches NASA's Astrophysics "+
		"Data System for papers", adsPage, "Example_ads", searchingADS,
		tool.adsVerb, pzm.Value("most", verb.NoShort, "how many papers "+
			"to show, the newest first; ads-most when none"))
	declared.Bare("archives")
	return declared
}

// searching is the flags that say where to look and what to ask.
func searching() []pzm.Flag {
	return []pzm.Flag{
		pzm.Value("ra", verb.NoShort, "the right ascension, in "+
			"degrees"),
		pzm.Value("dec", verb.NoShort, "the declination, in degrees"),
		pzm.Value("radius", verb.NoShort, "how far round it, in "+
			"degrees"),
		pzm.List("archive", verb.NoShort, "only these archives; every "+
			"one when none is named"),
		pzm.Value("most", verb.NoShort, "how many observations nustar "+
			"and chandra find, the longest first"),
		pzm.Value("hips-host", verb.NoShort, "the host whose "+
			"hips2fits draws the cutouts; Strasbourg's when none"),
		pzm.Value("hips", verb.NoShort, "the HiPS survey the cutouts "+
			"are drawn from; CDS/P/DSS2/red when none"),
		pzm.Value("pixels", verb.NoShort, "how many pixels a side a "+
			"cutout is"),
	}
}

// env is the value of an environment variable, or none when the command
// reads no environment.
func (tool command) env(name string) string {
	if tool.lookup == nil {
		return ""
	}
	return tool.lookup(name)
}
