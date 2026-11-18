// Package refractiveindex fetches and reads the refractiveindex.info
// database: optical constants of thousands of materials, their
// refractive index n and extinction coefficient k by wavelength.
//
// The measurements span the ultraviolet into the infrared. Mikhail
// Polyanskiy keeps the database; each material is a book on a shelf,
// and each page is one measurement by the researchers who made it.
//
// The database is in the public domain: copyright and related rights
// were waived through the CC0 1.0 Universal Public Domain Dedication.
//
// Work using it cites Polyanskiy, M. N. Refractiveindex.info database
// of optical constants. Scientific Data 11, 94 (2024), doi
// 10.1038/s41597-023-02898-2, and the original publications behind the
// specific datasets it uses; those are each data file's references.
package refractiveindex

import (
	"archive/zip"
	"bufio"
	_ "embed"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/janearc/celestial-data"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "refractiveindex"

// The database's release, as GitHub serves it.
const (
	// release is the release's tag.
	release = "v2026-05-24"
	// archiveName is the release's zip of the whole database.
	archiveName = "rii-database-2026-05-24.zip"
	// address is where the release serves the zip.
	address = "https://github.com/polyanskiy/refractiveindex.info-" +
		"database/releases/download/" + release + "/" + archiveName
	// doi names the data descriptor in Scientific Data that the
	// database asks to be cited.
	doi = "10.1038/s41597-023-02898-2"
	// sha256 is the sum of the zip's bytes, read beside the asset on the
	// release page identified by rii-release in sources.yaml.
	sha256 = "6cdebebd17fe02b6dc2b03189e15e6c6" +
		"89afc6619c558e1df7b9ac307b6d14eb"
)

// The number the release gives, by its name in numbers.yaml.
const (
	// archiveSize is the release zip's length in bytes, as served.
	archiveSize = "archive-size"
)

// numbersFile is the numbers file that holds the release's number.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// measured is the numbers file, read.
func measured() *celestialdata.Measured {
	return celestialdata.NewMeasured(numbersFile, numbersRaw)
}

// New is the database's archive, asked by name only. It holds the one
// release zip, held to its size and to the sum of its bytes.
func New() (*celestialdata.Fixed, error) {
	size := int64(measured().Value(archiveSize))
	sum, err := celestialdata.ParseDigest(sha256)
	if err != nil {
		return nil, refuse("cannot read its own sha256: %w", err)
	}
	about := "refractiveindex.info database, in the public domain by " +
		"the CC0 1.0 Universal Public Domain Dedication; cite " +
		"Polyanskiy, M. N. Refractiveindex.info database of optical " +
		"constants. Scientific Data 11, 94 (2024), doi:" + doi
	database := celestialdata.NewFile(release, archiveName, address,
		size, about).Summed(sum)
	return celestialdata.NewFixed(adapterName, database)
}

// refuse is a refusal that begins with the archive's name.
func refuse(format string, args ...any) error {
	return celestialdata.Refuse(adapterName+" "+format, args...)
}

// The archive's own names for the files Read asks it for.
const (
	// catalog is the catalog of the pages with n and k data, as the
	// archive names it. It is a list of shelves, each of books, each
	// of pages, and each page says where its data file is.
	catalog = "database/catalog-nk.yml"
	// dataFolder is the folder every page's data file is under; a
	// page's data path is relative to it. Some paths hold spaces, as
	// "other/alloys/stainless steel/nk/Karlsson-austenitic.yml", and
	// the spaces are the path's own.
	dataFolder = "database/data/"
)

// Table is one page's optical constants: what the page's data file says
// of itself, and its rows.
type Table struct {
	references string
	comments   string
	rows       []Row
}

// References is the page's references section: the publications the
// page asks to be cited for its numbers.
func (table Table) References() string { return table.references }

// Comments is the page's comments section: what else its file says of
// its numbers, or the empty string when it says none.
func (table Table) Comments() string { return table.comments }

// Rows is the page's constants, a wavelength each, in the file's order.
func (table Table) Rows() []Row { return table.rows }

// Row is one wavelength's optical constants. K is zero for a page that
// tabulates only n: such a page gives no k at all, and none is drawn
// from it, so zero stands for none.
type Row struct {
	Micrometres float64
	N           float64
	K           float64
}

// Read is a page of a book on a shelf, read from the database's archive
// by the names the catalog takes: shelf, book and page, as "main", "Au"
// and "Johnson".
//
// It refuses a shelf, book or page the catalog does not hold, by name,
// and a page whose constants are a formula or anything but tabulated n
// and k, by name.
func Read(archive io.ReaderAt, size int64, shelf, book, page string) (
	Table, error) {
	files, err := zip.NewReader(archive, size)
	if err != nil {
		return Table{}, fmt.Errorf("%s: %w", adapterName, err)
	}
	path, err := pageOf(files.File, shelf, book, page)
	if err != nil {
		return Table{}, err
	}
	for _, file := range files.File {
		if file.Name != dataFolder+path {
			continue
		}
		return readTable(file, page)
	}
	return Table{}, refuse("holds no %s", dataFolder+path)
}

// pageOf is the data path the catalog gives a page, found by its shelf,
// book and page. It refuses what the catalog does not hold, by name.
func pageOf(files []*zip.File, shelf, book, page string) (string, error) {
	for _, file := range files {
		if file.Name != catalog {
			continue
		}
		opened, err := file.Open()
		if err != nil {
			return "", unread(file, err)
		}
		defer opened.Close()
		return pathOf(opened, file, shelf, book, page)
	}
	return "", refuse("holds no %s", catalog)
}

// pathOf is the data path a catalog read gives a page. The catalog lists
// its shelves in turn, each its books, each its pages, and each page
// holds a data path.
//
// A book belongs to the preceding shelf, and a page to its preceding
// book. One pass through the file finds the page and says whether its
// shelf, book and page are there.
func pathOf(read io.Reader, file *zip.File, shelf, book, page string) (string,
	error) {
	lines := bufio.NewScanner(read)
	var currentShelf, currentBook, currentPage string
	var onShelf, onBook, found bool
	var path string
	for lines.Scan() {
		line := lines.Text()
		if named, ok := keyed(line, "SHELF"); ok {
			currentShelf, currentBook, currentPage = named, "", ""
			onShelf = onShelf || named == shelf
			continue
		}
		if named, ok := keyed(line, "BOOK"); ok {
			currentBook, currentPage = named, ""
			onBook = onBook ||
				(currentShelf == shelf && named == book)
			continue
		}
		if named, ok := keyed(line, "PAGE"); ok {
			currentPage = named
			continue
		}
		if where, ok := value(strings.TrimSpace(line), "data"); ok &&
			currentPage != "" && currentShelf == shelf &&
			currentBook == book && currentPage == page {
			path, found = where, true
		}
	}
	if err := lines.Err(); err != nil {
		return "", unread(file, err)
	}
	if !onShelf {
		return "", refuse("holds no shelf %s", shelf)
	}
	if !onBook {
		return "", refuse("holds no book %s on shelf %s", book, shelf)
	}
	if !found {
		return "", refuse("holds no page %s in book %s", page, book)
	}
	return path, nil
}

// keyed is the value a catalog line gives one of its keys, when it gives
// one: a line giving a shelf, book or page key. A value can be quoted,
// as the page name "4786" is; the quotes are not part of the name.
func keyed(line, key string) (string, bool) {
	marked := strings.TrimSpace(line)
	marked = strings.TrimPrefix(marked, "- ")
	if named, ok := value(marked, key); ok {
		return unquoted(named), true
	}
	return "", false
}

// value is the value a line gives a key, when it gives one: what follows
// "key:" on the line. Only the line's ends are trimmed: values hold
// spaces of their own, as data paths do.
func value(line, key string) (string, bool) {
	if !strings.HasPrefix(line, key+":") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, key+":")), true
}

// unquoted is a value without the quotes a catalog may give it. Only a
// pair around the whole value is taken; quotes inside are the value's.
func unquoted(named string) string {
	if len(named) >= 2 && named[0] == '"' && named[len(named)-1] == '"' {
		return named[1 : len(named)-1]
	}
	return named
}

// The kinds of page a data file holds, and the rows of each.
const (
	// nk is a page of n and k together: a row of three numbers.
	nk = "tabulated nk"
	// n is a page of n alone: a row of two numbers.
	n = "tabulated n"
	// dataIndent is the indent of a data entry's "data:" line; its
	// rows are written deeper.
	dataIndent = 4
	// bits is a float64's width, for ParseFloat. It's defined.
	bits = 64
)

// readTable is a page's table, read from its data file. The file begins
// with the database's comments, then says where its numbers came from
// in the references section and what else it says in the comments.
//
// Its constants are in the data section, by the type it gives them. A
// file can hold more than one data entry. Types other than tabulated n
// or k are refused.
func readTable(file *zip.File, page string) (Table, error) {
	opened, err := file.Open()
	if err != nil {
		return Table{}, unread(file, err)
	}
	defer opened.Close()
	lines := bufio.NewScanner(opened)
	var (
		table   Table
		block   string   // references or comments section being read
		text    []string // that text, a line each, its indent taken off
		base    = -1     // first line's indent; -1 until one is read
		kind    string   // the type of the data entry being read
		refusal string   // first unsupported type in the file
		entries bool     // whether any type was read at all
		inData  bool     // while a tabulated data block's rows are read
		rows    []Row
	)
	for lines.Scan() {
		line := lines.Text()
		if strings.TrimSpace(line) == "" {
			if block != "" && base >= 0 {
				text = append(text, "")
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if header, named, blockText := headerOf(line); header != "" {
			// A header starts at column zero and ends prior blocks.
			// Each block's contents are indented.
			inData = false
			table = closed(table, block, text)
			block, text, base = "", nil, -1
			if !blockText {
				table = inline(table, header, named)
				continue
			}
			block = header
			continue
		}
		if block != "" {
			if base < 0 {
				base = indentOf(line)
			}
			text = append(text, line[min(base, len(line)):])
			continue
		}
		if inData {
			if indentOf(line) > dataIndent {
				row, err := rowOf(line, page, want(kind), file)
				if err != nil {
					return Table{}, err
				}
				rows = append(rows, row)
				continue
			}
			inData = false // a line at the data's depth or above
			// ends the rows; it is read as what it is, below
		}
		trimmed := strings.TrimSpace(line)
		if named, ok := value(strings.TrimPrefix(trimmed, "- "),
			"type"); ok {
			kind, entries = unquoted(named), true
			if kind != nk && kind != n {
				refusal = refusalOr(refusal, kind)
			}
			continue
		}
		if _, ok := value(trimmed, "data"); ok {
			if kind == nk || kind == n {
				inData = true
			}
			continue
		}
		// anything else a data entry says of itself: its coefficients,
		// its wavelength range, and the rest, is not its rows
	}
	if err := lines.Err(); err != nil {
		return Table{}, unread(file, err)
	}
	table = closed(table, block, text)
	if refusal != "" {
		return Table{}, refuse("page %s is a %s, not tabulated n and k",
			page, refusal)
	}
	if !entries {
		return Table{}, refuse(
			"page %s gives no type in its DATA", page)
	}
	table.rows = rows
	return table, nil
}

// headerOf is the header a data file's line begins, at the start of a
// line, with the value it gives and whether that value is text on the
// lines that follow.
//
// Only a header ends text: what follows one is indented, so no line of
// its text can be mistaken for the next header.
func headerOf(line string) (header, named string, text bool) {
	for _, each := range [2]string{"REFERENCES", "COMMENTS"} {
		if given, ok := value(line, each); ok {
			return each, unquoted(given), given == "|"
		}
	}
	if _, ok := value(line, "DATA"); ok {
		return "DATA", "", false
	}
	return "", "", false
}

// closed is a table with the text of a block it was reading, kept as
// the block's own words, a line each.
func closed(table Table, block string, text []string) Table {
	if block == "" || len(text) == 0 {
		return table
	}
	words := strings.Join(text, "\n") + "\n"
	if block == "REFERENCES" {
		table.references = words
	} else {
		table.comments = words
	}
	return table
}

// inline is a table with the value a header gave on its own line: a
// comments value that is a quoted sentence, not a block. Data gives none.
func inline(table Table, header, named string) Table {
	switch header {
	case "REFERENCES":
		table.references = named
	case "COMMENTS":
		table.comments = named
	}
	return table
}

// indentOf is how deeply a line is written.
func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

// refusalOr is the first of two refusals, the one already made.
func refusalOr(made, kind string) string {
	if made != "" {
		return made
	}
	return kind
}

// want is how many numbers a row of a kind holds.
func want(kind string) int {
	if kind == nk {
		return 3
	}
	return 2
}

// rowOf is one row of a page's tabulated data, read from a line of it:
// the wavelength in micrometres, then n, then k for a page that has it.
// A page that tabulates n alone has no k, and zero stands for none.
func rowOf(line, page string, want int, file *zip.File) (Row, error) {
	numbers := strings.Fields(line)
	if len(numbers) != want {
		return Row{}, refuse("page %s has a row of %d numbers, not %d",
			page, len(numbers), want)
	}
	var read [3]float64
	for at, number := range numbers {
		parsed, err := strconv.ParseFloat(number, bits)
		if err != nil {
			return Row{}, unread(file, err)
		}
		read[at] = parsed
	}
	return Row{Micrometres: read[0], N: read[1], K: read[2]}, nil
}

// unread is why a file in the archive would not read.
func unread(file *zip.File, err error) error {
	return fmt.Errorf("%s: %s: %w", adapterName, file.Name, err)
}
