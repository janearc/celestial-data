package main

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/ads"
	"github.com/janearc/celestial-data/gaia"
	"github.com/janearc/celestial-data/secrets"
	"github.com/janearc/celestial-data/web"
	"github.com/janearc/puzzlemouth/pzm"
	"github.com/janearc/puzzlemouth/verb"
)

//go:generate -command mock go tool mockgen -package=main
//go:generate mock -source=tool.go -destination=tool_mock_test.go

// numbersFile is the numbers file the command picks its wait from.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// agent is what the command says it is to the archive it signs in to:
// its name.
const agent = secrets.Tool

// gaiaSession is the variable that names where the gaia archive's
// session is kept: gaia-login writes it, and celestial-data's fetch
// sends it.
const gaiaSession = "GAIA_SESSION"

// session is a web that keeps the cookies its answers set, and can be
// asked for them.
type session interface {
	web.Web
	Cookies(address string) ([]*http.Cookie, error)
}

// command is celestial-data-secrets: where op reads secrets, the
// environment it finds the home and the gaia session in, the numbers
// it picks, and the web it signs in through. A test gives each; run
// gives op, the process's environment and the network.
type command struct {
	keys   secrets.Reader
	lookup func(name string) string
	picked *celestialdata.Measured
	signed session
}

// run runs celestial-data-secrets with its words, input and output
// given, reading secrets with op and the process's environment.
func run(args []string, in io.Reader, out, errs io.Writer) int {
	tool := command{keys: secrets.NewOnePassword("op"),
		lookup: os.Getenv,
		picked: celestialdata.NewMeasured(numbersFile, numbersRaw)}
	return tool.run(args, in, out, errs)
}

// run runs the command through pzm.
func (tool command) run(args []string, in io.Reader, out,
	errs io.Writer) int {
	return tool.declared().RunWith(args, in, out, errs)
}

// The facts of the command's verbs.
var (
	// listing only reads the config.
	listing = verb.NewFacts(verb.WritesNothing, verb.LeavesAlone,
		verb.ThisMachine, verb.NoOne, verb.Light)
	// checking reads secrets through op, which asks the person at the
	// keyboard and may ask 1Password's servers, and writes nothing.
	checking = verb.NewFacts(verb.WritesNothing, verb.LeavesAlone,
		verb.Outside, verb.Confirmation, verb.Light)
	// signingIn reads op, which asks the person at the keyboard, signs
	// in to the archive and writes the session.
	signingIn = verb.NewFacts(verb.WritesTo("the file --session or "+
		gaiaSession+" names"), verb.Changes, verb.Outside,
		verb.Confirmation, verb.Light)
	// keepingToken reads op, which asks the person at the keyboard, and
	// writes the token where searches find it.
	keepingToken = verb.NewFacts(verb.WritesTo("ads-token beside the "+
		"config, or the file --session or "+ads.Session+" names"),
		verb.Changes, verb.Outside, verb.Confirmation, verb.Light)
)

// declared is celestial-data-secrets and its verbs, as pzm takes them.
// Each verb's example is the function Example_ and its name in this
// package's tests.
func (tool command) declared() *pzm.Tool {
	declared := pzm.New(agent, "keeps the secrets celestial-data signs "+
		"in with, in one place", pzm.Build(build), pzm.Built(built),
		pzm.Who(who), pzm.Sessions(sessions))
	declared.Verb("list", verb.NoShort, "lists each service and where "+
		"its secret is kept", listPage, "Example_list", listing,
		tool.listVerb)
	declared.Verb("check", verb.NoShort, "reads each secret through op, "+
		"and says only whether it was read", checkPage, "Example_check",
		checking, tool.checkVerb)
	declared.Verb("gaia-login", verb.NoShort, "signs in to the gaia "+
		"archive with its secret", loginPage, "Example_gaiaLogin",
		signingIn, tool.loginVerb, pzm.Value("session", verb.NoShort,
			"where to keep the session; "+gaiaSession+" when none"))
	declared.Verb("ads-login", verb.NoShort, "keeps the ads token "+
		"where searches find it", adsLoginPage, "Example_adsLogin",
		keepingToken, tool.adsLoginVerb, pzm.Value("session",
			verb.NoShort, "where to keep the token; else "+
				ads.Session+", else beside the config"))
	declared.Bare("list")
	return declared
}

// env is the value of an environment variable.
func (tool command) env(name string) string {
	return tool.lookup(name)
}

// config is where the person's config is: under the home HOME names.
func (tool command) config() (string, error) {
	home := tool.env("HOME")
	if home == "" {
		return "", celestialdata.Refuse(agent + " finds its config " +
			"under the home HOME names, and HOME names none")
	}
	return secrets.Path(home), nil
}

// keeper reads the config, and secrets through op.
func (tool command) keeper() (*secrets.Keeper, error) {
	path, err := tool.config()
	if err != nil {
		return nil, err
	}
	return secrets.Open(path, tool.keys)
}

const listPage = "Lists each service celestial-data signs in to, what " +
	"its secret is, the fields of its 1Password item, and where the " +
	"config says that item is. It reads the config, and never op, so " +
	"it says no secret and asks no one. The config is the tool's own, " +
	"in the .config folder of the home HOME names, one line a " +
	"service: secret NAME :: op://vault/item."

// listVerb lists each service, its fields and its item.
func (tool command) listVerb(call pzm.Call) error {
	path, err := tool.config()
	if err != nil {
		return err
	}
	keeper, err := secrets.Open(path, tool.keys)
	if err != nil {
		return err
	}
	out := call.Out()
	fmt.Fprintf(out, "the config: %s\n", path)
	for _, service := range secrets.Known() {
		fields := strings.Join(service.Fields(), ", ")
		fmt.Fprintf(out, "%s: %s; its fields: %s\n", service.Name(),
			service.About(), fields)
		where := placed(keeper, service.Name())
		fmt.Fprintf(out, "  %s\n", where)
	}
	return nil
}

// placed says where the config keeps a known service's secret, or the
// line that would place it. A known service's only refusal is that the
// config gives it no place.
func placed(keeper *secrets.Keeper, name string) string {
	item, err := keeper.Item(name)
	if errors.Is(err, secrets.Unplaced()) {
		return "not placed: add the line \"secret " + name +
			" :: op://vault/item\""
	}
	return "kept in " + item
}

const checkPage = "Reads the secret of each service named, or of every " +
	"service when none is, through op, and says of each only whether " +
	"it was read, or why not. It never says a secret. op asks the " +
	"person at the keyboard to allow each read, so a person runs this. " +
	"It fails when any secret was not read."

// checkVerb reads each secret named, or every one, and says whether it
// was read.
func (tool command) checkVerb(call pzm.Call) error {
	keeper, err := tool.keeper()
	if err != nil {
		return err
	}
	names := call.Args()
	if len(names) == noneNamed {
		for _, service := range secrets.Known() {
			names = append(names, service.Name())
		}
	}
	unread := noneNamed
	for _, name := range names {
		if _, err := keeper.Secret(name); err != nil {
			unread++
			fmt.Fprintf(call.Out(), "%s: not read: %v\n", name, err)
			continue
		}
		fmt.Fprintf(call.Out(), "%s: read\n", name)
	}
	if unread > noneNamed {
		return fmt.Errorf("%d of %d secrets were not read", unread,
			len(names))
	}
	return nil
}

// noneNamed is a count of nothing. It's defined: no services named.
const noneNamed = 0

const loginPage = "Signs in to the Gaia archive with the gaia secret, " +
	"its username and password read through op from the item the " +
	"config places it in, and keeps the session in the file --session " +
	"names, or GAIA_SESSION, readable by its owner alone. op asks the " +
	"person at the keyboard to allow each read, so a person runs this. " +
	"celestial-data's fetch sends the session when GAIA_SESSION names " +
	"it, so the archive runs its jobs as that person's. A refused " +
	"sign-in names the username it tried, and never the password."

// readyLogin is what a login verb has before op is asked: the config's
// keeper, the item it places the service's secret in, and where to keep
// what the login gives.
type readyLogin struct {
	keeper     *secrets.Keeper
	item, path string
}

// readied is a service's login, where --session or the variable says to
// keep what it gives, or else the file of the name beside the config,
// when there is one. It refuses before op is asked.
func (tool command) readied(call pzm.Call, service, variable,
	beside string,
) (*readyLogin, error) {
	keeper, err := tool.keeper()
	if err != nil {
		return nil, err
	}
	item, err := keeper.Item(service)
	if err != nil {
		return nil, err
	}
	path := call.Value("session")
	if path == "" {
		path = tool.env(variable)
	}
	if path == "" && beside != "" {
		path = secrets.Beside(tool.env("HOME"), beside)
	}
	if path == "" {
		return nil, celestialdata.Refuse(needsSession, service,
			variable)
	}
	return &readyLogin{keeper: keeper, item: item, path: path}, nil
}

// noFile is no file beside the config: a login whose session is kept
// only where the person says. gaia's is, since fetch sends a session
// only when GAIA_SESSION names it.
const noFile = ""

// needsSession is the refusal of a login with nowhere to keep what it
// gives: the service and its variable.
const needsSession = "%s-login needs --session, or %s, to keep the " +
	"session in"

// loginVerb signs in to the gaia archive and keeps the session where
// --session or GAIA_SESSION says.
func (tool command) loginVerb(call pzm.Call) error {
	ready, err := tool.readied(call, "gaia", gaiaSession, noFile)
	if err != nil {
		return err
	}
	keeper, item, path := ready.keeper, ready.item, ready.path
	if call.DryRun() {
		_, err := fmt.Fprintf(call.Out(), wouldSignIn, item,
			gaia.Service, path)
		return err
	}
	if err := tool.signIn(keeper, path); err != nil {
		return err
	}
	_, err = fmt.Fprintf(call.Out(), "signed in to %s; the session is "+
		"in %s\n", gaia.Service, path)
	return err
}

const adsLoginPage = "Reads the ads secret's token through op, from " +
	"the item the config places it in, and keeps it, readable by its " +
	"owner alone, in ads-token beside the config, or in the file " +
	"--session or ADS_SESSION names. op asks the person at the " +
	"keyboard to allow the read, once. celestial-data ads reads the " +
	"token from the same place and never asks op, so searches ask no " +
	"one. Run it again when the token changes."

// adsLoginVerb reads the ads token and keeps it where --session or
// ADS_SESSION says.
func (tool command) adsLoginVerb(call pzm.Call) error {
	ready, err := tool.readied(call, "ads", ads.Session, ads.TokenFile)
	if err != nil {
		return err
	}
	if call.DryRun() {
		_, err := fmt.Fprintf(call.Out(), wouldKeep, ready.item,
			ready.path)
		return err
	}
	secret, err := ready.keeper.Secret("ads")
	if err != nil {
		return err
	}
	if err := ads.Save(secret["credential"], ready.path); err != nil {
		return err
	}
	_, err = fmt.Fprintf(call.Out(), "the ads token is kept in %s, "+
		"readable by you alone\n", ready.path)
	return err
}

// wouldKeep is what a dry run of ads-login says: the item and the file.
const wouldKeep = "would read the ads token from %s with op, and keep " +
	"it in %s, readable by you alone\n"

// wouldSignIn is what a dry run of gaia-login says: the item, the
// archive and the file.
const wouldSignIn = "would read gaia's username and password from %s " +
	"with op, sign in to %s, and keep the session in %s\n"

// signIn reads the gaia secret, signs in with it, and keeps the
// session in path.
func (tool command) signIn(keeper *secrets.Keeper, path string) error {
	login, err := keeper.Secret("gaia")
	if err != nil {
		return err
	}
	signed, err := tool.client()
	if err != nil {
		return err
	}
	err = gaia.SignIn(signed, login["username"], login["password"])
	if err != nil {
		return err
	}
	cookies, err := signed.Cookies(gaia.Service)
	if err != nil {
		return err
	}
	return gaia.Save(cookies, path)
}

// client is a web that keeps the cookies its answers set: the one a
// test gives, or the network with the wait the numbers say.
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
