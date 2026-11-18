package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
	"github.com/janearc/puzzlemouth/pzm"
)

// The words fetch-set takes: the name of one set. They're defined.
const (
	oneWord = 1 // defined: how many words it takes
	first   = 0 // defined: where the name is among them
)

// dataSuffix is what the variable named for a set ends in.
const dataSuffix = "_DATA"

// setVerb brings the files of a named set home. It refuses an unknown
// set, and a call that names no folder, before it asks the network.
func (tool command) setVerb(call pzm.Call) error {
	name, made, err := tool.setOf(call.Args())
	if err != nil {
		return err
	}
	folder, err := tool.folderOf(call, name)
	if err != nil {
		return err
	}
	if err := tool.insist(call); err != nil {
		return err
	}
	get, err := tool.web()
	if err != nil {
		return err
	}
	wanted, err := made(tool, get)
	if err != nil {
		return err
	}
	bringing := &bringer{get: get, near: wanted.near, out: call.Out(),
		folder: folder, dry: call.DryRun()}
	return bringing.all(name, wanted.parts)
}

// setOf is the name of the one set the words name, and its maker. It
// refuses any other words, and says which sets there are.
func (tool command) setOf(words []string) (string, maker, error) {
	if len(words) != oneWord {
		return "", nil, celestialdata.Refuse("fetch-set takes the "+
			"name of one set, not %d words. The sets are: %s",
			len(words), tool.setNames())
	}
	made, ok := tool.sets[words[first]]
	if !ok {
		return "", nil, celestialdata.Refuse("no set is called %s. "+
			"The sets are: %s", words[first], tool.setNames())
	}
	return words[first], made, nil
}

// folderOf is the folder a call brings a set into: --into, or else the
// variable named for the set. It refuses when neither is given, naming
// both and how to give the variable.
func (tool command) folderOf(call pzm.Call, name string) (string, error) {
	if into := call.Value("into"); into != "" {
		return into, nil
	}
	variable := strings.ToUpper(name) + dataSuffix
	if folder := tool.lookup(variable); folder != "" {
		return folder, nil
	}
	return "", celestialdata.Refuse("fetch-set %s needs --into, the "+
		"folder to bring its files into, or %s. Set it in your game "+
		"config as: set %s /some/folder", name, variable, variable)
}

// A bringer brings the files of a set home and counts what it did. When
// it is dry it brings none and says what it would.
type bringer struct {
	get           web.Web
	near          *celestialdata.Near
	out           io.Writer
	folder        string
	dry           bool
	fetched, kept int
}

// all brings each part of a set, saying a line for every file. A failure
// does not stop the others. It says how many failed, and the reason for
// each, in what it returns.
func (bringing *bringer) all(name string, parts []part) error {
	var failed []error
	for _, each := range parts {
		failed = append(failed, bringing.gather(each)...)
	}
	verb := "fetched"
	if bringing.dry {
		verb = "to fetch"
	}
	fmt.Fprintf(bringing.out, "fetch-set %s: %d %s, %d kept, in %s\n",
		name, bringing.fetched, verb, bringing.kept, bringing.folder)
	if len(failed) == none {
		return nil
	}
	count := len(failed)
	failed = append(failed, fmt.Errorf("fetch-set %s: %d failed. What "+
		"came is kept. Fix what is said above and run it again", name,
		count))
	return errors.Join(failed...)
}

// gather brings the files a part names that are not there. When all of
// them are there it asks the archive nothing. A dry call asks it nothing
// when the part names its files. It gives the reason for each file it
// could not bring, and for each file the archive does not list.
func (bringing *bringer) gather(each part) []error {
	dir := filepath.Join(bringing.folder, filepath.FromSlash(each.into))
	if each.only != nil && allThere(dir, each.only) {
		for _, name := range each.only {
			bringing.keep(each.into, name)
		}
		return nil
	}
	if bringing.dry {
		return bringing.sketch(each, dir)
	}
	listed, err := each.from.Find(bringing.near)
	if err != nil {
		return []error{fmt.Errorf("%s/: %w", each.into, err)}
	}
	chosen, failed := each.choose(listed)
	for _, product := range chosen {
		if err := bringing.bring(product, each.into, dir); err != nil {
			failed = append(failed, err)
		}
	}
	return failed
}

// sketch says what a dry call would bring of a part. It names the files
// the part names. A part that names none is asked for its listing, since
// that is the only way to know them.
func (bringing *bringer) sketch(each part, dir string) []error {
	names := each.only
	if names == nil {
		listed, err := each.from.Find(bringing.near)
		if err != nil {
			return []error{fmt.Errorf("%s/: %w", each.into, err)}
		}
		for _, product := range listed {
			names = append(names, product.Name())
		}
	}
	var failed []error
	for _, name := range names {
		found, err := there(filepath.Join(dir, name))
		label := each.into + "/" + name
		switch {
		case err != nil:
			err = fmt.Errorf("%s: %w", label, err)
			failed = append(failed, err)
		case found:
			bringing.keep(each.into, name)
		default:
			fmt.Fprintf(bringing.out, "would fetch %s/%s\n",
				each.into, name)
			bringing.fetched++
		}
	}
	return failed
}

// choose is the files of a listing that are wanted. A wanted file the
// listing does not hold is an error, which names it.
func (each part) choose(listed []*celestialdata.Product) (
	[]*celestialdata.Product, []error) {
	if each.only == nil {
		return listed, nil
	}
	var chosen []*celestialdata.Product
	var missing []error
	for _, name := range each.only {
		product := listedAs(listed, name)
		if product == nil {
			archive := each.from.Name()
			missing = append(missing, fmt.Errorf("%s/%s: %s does "+
				"not list it. Its page may have changed",
				each.into, name, archive))
			continue
		}
		chosen = append(chosen, product)
	}
	return chosen, missing
}

// listedAs is the product of a name in a listing, or none.
func listedAs(listed []*celestialdata.Product,
	name string) *celestialdata.Product {
	for _, product := range listed {
		if product.Name() == name {
			return product
		}
	}
	return nil
}

// bring fetches one file into dir, or keeps the one that is there.
func (bringing *bringer) bring(product *celestialdata.Product, into,
	dir string) error {
	label := into + "/" + product.Name()
	home := filepath.Join(dir, product.Name())
	found, err := there(home)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if found {
		bringing.keep(into, product.Name())
		return nil
	}
	_, err = celestialdata.Fetch(bringing.get, product, dir)
	if errors.Is(err, celestialdata.Refused()) {
		return fmt.Errorf("%s: %w. If no other fetch is running, "+
			"remove the .part file there and run it again", label,
			err)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	fmt.Fprintf(bringing.out, "fetched  %s\n", label)
	bringing.fetched++
	return nil
}

// keep says a file is kept and counts it.
func (bringing *bringer) keep(into, name string) {
	fmt.Fprintf(bringing.out, "kept     %s/%s (already there)\n", into,
		name)
	bringing.kept++
}

// there is whether a path exists, or why that is not known.
func there(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// allThere is whether every named file is in the folder.
func allThere(dir string, names []string) bool {
	for _, name := range names {
		found, _ := there(filepath.Join(dir, name))
		if !found {
			return false
		}
	}
	return true
}
