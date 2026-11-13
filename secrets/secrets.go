// Package secrets is the one place celestial-data knows its secrets: each
// service it signs in to, the fields that service's secret has, and where
// in 1Password the person who runs it keeps that secret. The config says
// where; op reads it, each time it is needed.
//
// A secret goes only to the code that asked for it. What op prints, the
// secret, is never put in an error, and nothing this package writes
// holds it.
//
// Two things are passed on as they come, and are trusted to hold no
// secret: op's own words when it fails, and an error from the Reader a
// Keeper is given. That boundary is theirs to keep, and is the whole of
// what this package promises.
package secrets

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/janearc/celestial-data/internal/must"
	"github.com/janearc/puzzlemouth/config"
)

// Tool is the name of the command that keeps the secrets, and of its
// folder in the person's config.
const Tool = "celestial-data-secrets"

// errUnknown is the refusal of a service celestial-data does not sign in
// to.
var errUnknown = errors.New("secrets: no such service")

// errUnplaced is the refusal of a service the config gives no place.
var errUnplaced = errors.New("secrets: the config gives no place")

// Unplaced is the refusal of a service the config gives no place, for
// errors.Is.
func Unplaced() error { return errUnplaced }

// Reader reads a secret by its op:// reference.
type Reader interface {
	Read(reference string) (string, error)
}

// OnePassword reads secrets with the 1Password command, op. Each read
// may ask the person at the keyboard to allow it.
type OnePassword struct {
	command string
}

// NewOnePassword is a reader that runs command as op.
func NewOnePassword(command string) OnePassword {
	return OnePassword{command: command}
}

// Read runs op read for reference and gives what it printed, without the
// end of the line. A failure names the reference and passes on op's own
// words from its errors, never what it printed. That those words hold no
// secret is op's to keep.
func (keys OnePassword) Read(reference string) (string, error) {
	var said strings.Builder
	run := exec.Command(keys.command, "read", reference)
	run.Stderr = &said
	printed, err := run.Output()
	if err == nil {
		return strings.TrimRight(string(printed), "\r\n"), nil
	}
	failed := fmt.Errorf("%s read %s: %w", keys.command, reference, err)
	if refusal := strings.TrimSpace(said.String()); refusal != "" {
		return "", fmt.Errorf("%w: %s", failed, refusal)
	}
	return "", failed
}

// Service is one service celestial-data signs in to: its name, what it
// is, and the fields its secret has.
type Service struct {
	name, about string
	fields      []string
}

// Name is the name the config and the commands know the service by.
func (service Service) Name() string { return service.name }

// About says what the secret is.
func (service Service) About() string { return service.about }

// Fields are the fields of the service's 1Password item that are read.
func (service Service) Fields() []string {
	return slices.Clone(service.fields)
}

// known is every service celestial-data signs in to, by name.
var known = []Service{
	{name: "ads", about: "an API token for NASA's Astrophysics Data " +
		"System", fields: []string{"credential"}},
	{name: "gaia", about: "a login to the Gaia archive at ESA",
		fields: []string{"username", "password"}},
}

// Known is every service celestial-data signs in to, by name.
func Known() []Service {
	every := make([]Service, len(known))
	for at, service := range known {
		every[at] = Service{name: service.name, about: service.about,
			fields: service.Fields()}
	}
	return every
}

// find is the service of a name, or a refusal naming the ones there are.
func find(name string) (*Service, error) {
	for _, service := range Known() {
		if service.name == name {
			return &service, nil
		}
	}
	every := strings.Join(names(), " and ")
	return nil, fmt.Errorf("%w: %q; celestial-data signs in to %s",
		errUnknown, name, every)
}

// names is the name of every known service, in order.
func names() []string {
	every := make([]string, len(known))
	for at, service := range known {
		every[at] = service.name
	}
	return every
}

// place is one line of the config, "secret NAME :: op://vault/item":
// where a service's secret is kept, and the line that said so.
type place struct {
	service, item string
	at            int
}

// SetLine reads a config line into a place. It refuses a service that is
// not known and an item that is not an op:// reference to an item.
func (where *place) SetLine(line config.Line) error {
	name := line.Words[serviceWord]
	if _, err := find(name); err != nil {
		return err
	}
	if err := itemRefusal(line.Tail); err != nil {
		return err
	}
	where.service, where.item, where.at = name, line.Tail, line.At
	return nil
}

// serviceWord is the place of the service's name among a secret line's
// words. It's defined: a secret line takes one.
const serviceWord = 0

// itemParts is how many parts follow op:// in a reference to an item: a
// vault and an item. It's defined by op's secret reference syntax,
// op://vault/item/field, whose field the service names.
const itemParts = 2

// itemRefusal is why a reference does not name a 1Password item, or
// nothing.
func itemRefusal(item string) error {
	path, found := strings.CutPrefix(item, "op://")
	parts := strings.Split(path, "/")
	if !found || len(parts) != itemParts ||
		slices.ContainsFunc(parts, blank) {
		return fmt.Errorf("%q is not an item as op names one, "+
			"op://vault/item", item)
	}
	return nil
}

// blank is whether a part of a reference names nothing: it is empty, or
// spaces alone.
func blank(part string) bool { return strings.TrimSpace(part) == "" }

// file is what the config may say.
type file struct {
	//ninja:vow gate pzm's config sets it by reflection: the wire
	Places []place `config:"secret,1,tail" help:"a service and the op:// item its secret is kept in"`
}

// schema is the config's shape, made once. Its declaration is this
// package's own, so a refusal is a mistake in this file, which its tests
// find when the package loads.
var schema = must.Be(config.NewSchema[file]())

// Keeper reads the secrets of the services the config places, through a
// reader.
type Keeper struct {
	places map[string]place
	path   string
	keys   Reader
}

// Path is where the config is kept under a home: the tool's own folder
// of the home's .config, as pzm keeps every tool's.
func Path(home string) string {
	return Beside(home, "config")
}

// Beside is a file of a name in the config's folder under a home, where
// what a login keeps is kept unless the person says otherwise.
func Beside(home, name string) string {
	return filepath.Join(home, ".config", Tool, name)
}

// Open is a keeper of the services the config at path places, reading
// through keys. A config that is not there places nothing; one that will
// not read is refused, naming its line.
func Open(path string, keys Reader) (*Keeper, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		raw, err = nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("secrets: %w", err)
	}
	var read file
	if err := schema.Read(path, raw, &read); err != nil {
		return nil, err
	}
	keeper := &Keeper{places: map[string]place{}, path: path, keys: keys}
	for _, where := range read.Places {
		before, twice := keeper.places[where.service]
		if twice {
			return nil, placedTwice(path, where, before)
		}
		keeper.places[where.service] = where
	}
	return keeper, nil
}

// placedTwice is the refusal of a service a config places twice, naming
// both lines.
func placedTwice(path string, again, before place) error {
	return fmt.Errorf("%s:%d: secret: %s is placed at line %d already",
		path, again.at, again.service, before.at)
}

// Item is the op:// item the config places a service's secret in.
func (keeper Keeper) Item(name string) (string, error) {
	if _, err := find(name); err != nil {
		return "", err
	}
	where, found := keeper.places[name]
	if !found {
		line := "secret " + name + " :: op://vault/item"
		return "", fmt.Errorf("%w for %s in %s: add a line %q",
			errUnplaced, name, keeper.path, line)
	}
	return where.item, nil
}

// Secret is a service's secret, each field by name, read through op.
func (keeper Keeper) Secret(name string) (map[string]string, error) {
	item, err := keeper.Item(name)
	if err != nil {
		return nil, err
	}
	service, _ := find(name)
	secret := map[string]string{}
	for _, field := range service.fields {
		value, err := keeper.keys.Read(item + "/" + field)
		if err != nil {
			return nil, fmt.Errorf("secrets: %s's %s: %w", name,
				field, err)
		}
		secret[field] = value
	}
	return secret, nil
}
