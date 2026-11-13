package arxiv

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/janearc/celestial-data"
)

// ID is a paper's name on arXiv, with the version asked for or found,
// or none. A new-style name is four digits of year and month, a dot and
// four or five digits: 1809.03507. An old-style one is an archive,
// perhaps a subject, a slash and seven digits: astro-ph/0602086.
type ID struct {
	name    string
	version int
}

// unversioned is the version of an ID that names none. It's defined: no
// paper on arXiv has a version zero.
const unversioned = 0

// The shapes a name takes, and a version after it. A version is a v and
// a number that does not begin with a nought.
var (
	newStyle = regexp.MustCompile(`^[0-9]{4}\.[0-9]{4,5}$`)
	oldStyle = regexp.MustCompile(`^(?P<archive>[a-z]+(-[a-z]+)*)` +
		`(\.[A-Za-z-]+)?/(?P<number>[0-9]{7})$`)
	versioned = regexp.MustCompile(
		`^(?P<name>.+)v(?P<version>[1-9][0-9]*)$`)
	number = regexp.MustCompile(`^[1-9][0-9]*$`)
)

// The places of the parts a pattern names among its matches: a
// versioned id's name and version, and an old-style name's archive and
// number.
var (
	nameAt    = versioned.SubexpIndex("name")
	versionAt = versioned.SubexpIndex("version")
	archiveAt = oldStyle.SubexpIndex("archive")
	numberAt  = oldStyle.SubexpIndex("number")
)

// prefix is how a name is often written before it, as in arXiv:1809.03507.
const prefix = "arxiv:"

// ParseID reads an id as people and papers write one. That is the name
// alone, after arXiv:, or in an abs or pdf address on arxiv.org, with a
// version or without. It refuses anything else before arXiv is asked,
// says what it was given, and gives nil beside the error.
//
// An old-style name with a subject is read as arXiv names the paper,
// without it: math.GT/0309136 is math/0309136, the only name arXiv's
// API answers to.
func ParseID(written string) (*ID, error) {
	bare, err := strip(strings.TrimSpace(written))
	if err != nil {
		return nil, err
	}
	id := ID{name: bare}
	if parts := versioned.FindStringSubmatch(bare); parts != nil {
		// the pattern holds digits alone there, but not that an int
		// holds them: a version too long for one is refused, not cut
		// to another
		count, err := strconv.Atoi(parts[versionAt])
		if err != nil {
			return nil, celestialdata.Refuse("%q is not an arXiv id: "+
				"its version is longer than a version can be", written)
		}
		id = ID{name: parts[nameAt], version: count}
	}
	if parts := oldStyle.FindStringSubmatch(id.name); parts != nil {
		id.name = parts[archiveAt] + "/" + parts[numberAt]
		return &id, nil
	}
	if !newStyle.MatchString(id.name) {
		return nil, celestialdata.Refuse("%q is not an arXiv id: one "+
			"is written 1809.03507 or astro-ph/0602086, with a "+
			"version such as v2 or without", written)
	}
	return &id, nil
}

// strip takes an arXiv: off the front of an id, or the address round
// it. It refuses an address that is not an abs or pdf page on
// arxiv.org.
func strip(written string) (string, error) {
	if strings.HasPrefix(strings.ToLower(written), prefix) {
		return written[len(prefix):], nil
	}
	if !strings.Contains(written, "://") {
		return written, nil
	}
	address, err := url.Parse(written)
	if err != nil || !onArxiv(address.Host) {
		return "", celestialdata.Refuse("%q is not an address on "+
			"arxiv.org", written)
	}
	for _, page := range []string{"/abs/", "/pdf/"} {
		if rest, ok := strings.CutPrefix(address.Path, page); ok {
			return strings.TrimSuffix(rest, ".pdf"), nil
		}
	}
	return "", celestialdata.Refuse("%q is not a paper's abs or pdf "+
		"page", written)
}

// onArxiv checks if a host is one arxiv.org answers to.
func onArxiv(host string) bool {
	switch host {
	case "arxiv.org", "www.arxiv.org", "export.arxiv.org":
		return true
	}
	return false
}

// String is the id as arXiv writes it: the name, then the version when
// there is one.
func (id ID) String() string {
	if id.version == unversioned {
		return id.name
	}
	return id.name + "v" + strconv.Itoa(id.version)
}

// file is the name of the file a version of the paper is kept in. It is
// the id with an old-style slash made an underscore, so it is one plain
// file name, then .pdf.
func (id ID) file() string {
	return stem(id.name) + "v" + strconv.Itoa(id.version) + ".pdf"
}

// stem is a name made fit to begin a file name.
func stem(name string) string {
	return strings.ReplaceAll(name, "/", "_")
}

// Holds is whether a file is the paper the id names: the version's own
// file, or, for an id without a version, the file of any version.
func (id ID) Holds(file string) bool {
	if id.version != unversioned {
		return file == id.file()
	}
	rest, ok := strings.CutPrefix(file, stem(id.name)+"v")
	if !ok {
		return false
	}
	version, ok := strings.CutSuffix(rest, ".pdf")
	return ok && number.MatchString(version)
}
