package nistxray

import (
	"errors"
	"fmt"
	"html"
	"io/fs"
	"strconv"
	"strings"
)

// The pages of material constants, as the archive's folder holds them.
const (
	// elementTable is Table 1, the material constants of the elements.
	elementTable = "tab1.html"
	// compoundTable is Table 2, the material constants and the
	// composition of the compounds and mixtures.
	compoundTable = "tab2.html"
)

// The shapes of the rows the pages write.
const (
	// rowNumbers is the numbers a row of coefficients holds: the
	// energy, μ/ρ and μen/ρ. It's defined by the pages' tables.
	rowNumbers = 3
	// mostLabels is the labels a row may carry before its numbers: a
	// shell, "K", or an atom's number and shell, "11 K". It's defined
	// by the pages' tables.
	mostLabels = 2
	// elementCells is the cells of a row of Table 1: the atomic
	// number, the symbol, the name, Z/A, the mean excitation energy
	// and the density. It's defined by Table 1.
	elementCells = 6
	// compoundCells is the cells of a row of Table 2: the name, Z/A,
	// the mean excitation energy, the density and the composition.
	// It's defined by Table 2.
	compoundCells = 5
)

// The cells a material's name is read from, in the two tables.
const (
	// symbolCell is where Table 1 writes an element's symbol.
	symbolCell = 1
	// nameCell is where Table 2 writes a compound's name.
	nameCell = 0
)

// bits is a float64's width, for ParseFloat. It's defined.
const bits = 64

// Table is one page's coefficients: what the page says of itself, and
// its rows.
type Table struct {
	header string
	rows   []Row
}

// Header is the page's own title for its table, as the page writes
// it: "Lead\nZ = 82" for an element, "Water, Liquid" for a compound.
// The pages state no density of their own and carry no citation of
// their own: the densities are in Table 1 and Table 2.
//
// The citation is the database's, in the package's sources.yaml.
func (table Table) Header() string { return table.header }

// Rows is the page's coefficients, an energy each, in the page's
// order. Nothing is interpolated and nothing is derived: the rows are
// the page's own, handed out as it records them.
func (table Table) Rows() []Row { return table.rows }

// Row is one energy's coefficients, as the page records them. Edge is
// the label the row carries in its first column, when it carries one:
// "K" for an element's K edge, "11 K" for the K edge of the sodium,
// Z 11, in a compound.
//
// A row at an edge is a second measurement at the same energy, and the
// label is NIST's own for it; Edge is empty when the page gives none.
type Row struct {
	Edge        string
	Energy      float64 // in MeV
	MuOverRho   float64 // in cm^2/g
	MuEnOverRho float64 // in cm^2/g
}

// Element is an element's page of coefficients, read from the folder
// the archive's pages were fetched into, by atomic number: 82 for lead,
// whose page NIST names z82.html.
//
// It refuses an element whose page the archive does not hold, by name,
// a page that holds no table of rows, by the page's name, and a row
// that is not three numbers with at most two labels before them, by the
// page's name.
func Element(pages fs.FS, z int) (Table, error) {
	if z < hydrogen || z > uranium {
		return Table{}, refuse("holds no element Z %d", z)
	}
	return readPage(pages, fmt.Sprintf("z%02d.html", z))
}

// Compound is a compound's or mixture's page of coefficients, read from
// the folder the archive's pages were fetched into, by the name of the
// page as NIST's address takes it: "water" for water, "air" for dry air,
// "a150" for A-150 tissue-equivalent plastic.
//
// It refuses a compound whose page the folder does not hold, by name,
// a page that holds no table of rows, by the page's name, and a row
// that is not three numbers with at most two labels before them, by the
// page's name.
func Compound(pages fs.FS, name string) (Table, error) {
	return readPage(pages, name+".html")
}

// readPage is one page of the archive, read from the folder its pages
// were fetched into. It refuses a page the folder does not hold, by
// name.
func readPage(pages fs.FS, page string) (Table, error) {
	raw, err := fs.ReadFile(pages, page)
	if err != nil {
		return Table{}, refuse("holds no %s", page)
	}
	return tableOf(page, raw)
}

// tableOf is one page's table, read from the page's bytes. A page gives
// its table twice, as HTML and as an ASCII block in a PRE; the ASCII
// block is read, with three numbers a line and an absorption-edge
// label before them on some lines.
//
// The page's title for its table is the bold text it writes above the
// block.
func tableOf(page string, raw []byte) (Table, error) {
	text := string(raw)
	block, ok := between(text, "<PRE>", "</PRE>")
	if !ok {
		return Table{}, refuse("page %s holds no table of rows", page)
	}
	rows, err := rowsOf(block, page)
	if err != nil {
		return Table{}, err
	}
	if len(rows) == none {
		return Table{}, refuse("page %s holds no table of rows", page)
	}
	return Table{header: headerOf(text), rows: rows}, nil
}

// rowsOf are the rows of a page's ASCII table: a line whose last three
// fields are numbers is a row, and up to two fields before them are its
// absorption-edge label.
//
// A line of one or two numbers alone is a row cut short, and is refused.
// A line whose last three fields are not all numbers is a table heading
// or rule, and is passed over.
func rowsOf(block, page string) ([]Row, error) {
	var rows []Row
	for _, line := range strings.Split(block, "\n") {
		fields := strings.Fields(line)
		if len(fields) < rowNumbers {
			if len(fields) > none && numbersOnly(fields) {
				return nil, refuse(
					"page %s has a row of %d "+
						"numbers, not %d",
					page, len(fields), rowNumbers)
			}
			continue
		}
		var read [rowNumbers]float64
		heading := false
		for at, number := range fields[len(fields)-rowNumbers:] {
			parsed, err := strconv.ParseFloat(number, bits)
			if err != nil {
				heading = true
				break
			}
			read[at] = parsed
		}
		if heading {
			continue
		}
		if len(fields) > rowNumbers+mostLabels {
			return nil, refuse(
				"page %s has a row of %d fields, "+
					"not %d numbers and at "+
					"most %d labels", page, len(fields),
				rowNumbers,
				mostLabels)
		}
		rows = append(rows, Row{
			Edge: strings.Join(fields[:len(fields)-rowNumbers],
				" "),
			Energy:      read[0],
			MuOverRho:   read[1],
			MuEnOverRho: read[2],
		})
	}
	return rows, nil
}

// numbersOnly is whether every field of a line that has any is a
// number.
func numbersOnly(fields []string) bool {
	for _, field := range fields {
		if _, err := strconv.ParseFloat(field, bits); err != nil {
			return false
		}
	}
	return true
}

// headerOf is the page's own title for its table: the last bold text
// the page writes above its ASCII block, its tags taken out and its
// entities read, a line a line. An element's page titles itself with
// its name and atomic number; a compound's with its name alone.
func headerOf(text string) string {
	above := text
	if at := strings.Index(text, "<PRE>"); at >= 0 {
		above = text[:at]
	}
	bold, ok := betweenLast(above, "<B>", "</B>")
	if !ok {
		return ""
	}
	return textOf(bold)
}

// Constants is one material's row of Table 1 or Table 2, the constants
// NIST assumed in its evaluations: the mean ratio of atomic number to
// mass Z/A and the density, with the row's own text.
//
// Z and Symbol are the element's, and none for a compound; Name is the
// row's own name. NIST warns of the densities in both tables: "Some
// density values are only nominal."
type Constants struct {
	Z       int
	Symbol  string
	Name    string
	OverA   float64 // the mean ratio of atomic number to mass
	Density float64 // in g/cm^3
	Row     string  // the row's own cells, joined
}

// Material is a material's row of constants, read from the folder the
// archive's pages were fetched into, by the name the table writes: an
// element's symbol, such as "Pb" or "Al", or a compound's name, such
// as "Water, Liquid" or "Adipose Tissue (ICRU-44)".
//
// It refuses a material neither table holds, by name, a page the folder
// does not hold, by name, and a row whose numbers will not read, naming
// the page.
func Material(pages fs.FS, name string) (Constants, error) {
	elements, err := constantsRows(pages, elementTable)
	if err != nil {
		return Constants{}, err
	}
	for _, each := range elements {
		if each.Symbol == name {
			return each, nil
		}
	}
	compounds, err := constantsRows(pages, compoundTable)
	if err != nil {
		return Constants{}, err
	}
	for _, each := range compounds {
		if each.Name == name {
			return each, nil
		}
	}
	return Constants{}, refuse("holds no material %s", name)
}

// constantsRows are the rows of a page of material constants, each as
// the page records it. Table 1's rows hold an element's atomic number,
// symbol, name, Z/A, mean excitation energy and density.
//
// Table 2's rows hold a compound's name, Z/A, mean excitation energy,
// density and composition. A row of any other shape is a page heading
// or rule, and is passed over.
func constantsRows(pages fs.FS, page string) ([]Constants, error) {
	raw, err := fs.ReadFile(pages, page)
	if err != nil {
		return nil, refuse("holds no %s", page)
	}
	var rows []Constants
	for _, row := range strings.Split(string(raw), "</TR>") {
		cells := cellsOf(row)
		if len(cells) != elementCells && len(cells) != compoundCells {
			continue
		}
		constants, data, err := constantsOf(page, cells)
		if err != nil {
			return nil, err
		}
		if !data {
			continue
		}
		rows = append(rows, constants)
	}
	return rows, nil
}

// constantsOf is one row of material constants, from its cells, and
// whether the row is one. A row of Table 1's shape is an element's,
// read by its atomic number; one of Table 2's is a compound's.
//
// A row in the shape of one whose identifying cell names no material
// is a page heading, and is not one. A row that names a material and
// whose numbers will not read is refused, with the page and row name.
func constantsOf(page string, cells []string) (Constants, bool, error) {
	var constants Constants
	var overA, density string
	if len(cells) == elementCells {
		z, err := strconv.Atoi(cells[0])
		if err != nil || z < hydrogen || z > uranium {
			return Constants{}, false, nil
		}
		constants.Z, constants.Symbol, constants.Name =
			z, cells[symbolCell], cells[symbolCell+1]
		overA, density = cells[symbolCell+2], cells[elementCells-1]
	} else {
		if cells[nameCell] == "" {
			return Constants{}, false, nil
		}
		constants.Name = cells[nameCell]
		overA, density = cells[nameCell+1], cells[compoundCells-2]
	}
	ratio, ratioErr := strconv.ParseFloat(overA, bits)
	mass, massErr := strconv.ParseFloat(density, bits)
	if err := errors.Join(ratioErr, massErr); err != nil {
		return Constants{}, false, wrap(page, constants.Name, err)
	}
	constants.OverA, constants.Density = ratio, mass
	constants.Row = strings.Join(cells, " ")
	return constants, true, nil
}

// wrap is a parse's error, or none, naming the page and the row.
func wrap(page, name string, err error) error {
	if err == nil {
		return nil
	}
	return refuse("page %s has a row, %s, that will not read: %w", page,
		name, err)
}

// cellsOf are the cells of a row of a page's table, each as its own
// text. A row is what the page writes between one TR and its end; a
// cell is what it writes between a TD and its end.
//
// An empty cell is not one: the tables lay their first row out with
// rowspan spacer cells between the values, and those hold nothing.
func cellsOf(row string) []string {
	var cells []string
	for {
		at := strings.Index(row, "<TD")
		if at < 0 {
			return cells
		}
		opened := strings.Index(row[at:], ">")
		if opened < 0 {
			return cells
		}
		rest := row[at+opened+1:]
		end := strings.Index(rest, "</TD>")
		if end < 0 {
			return cells
		}
		if text := textOf(rest[:end]); text != "" {
			cells = append(cells, text)
		}
		row = rest[end+len("</TD>"):]
	}
}

// between is the text between two marks, when the text holds both, the
// first before the second.
func between(text, from, to string) (string, bool) {
	at := strings.Index(text, from)
	if at < 0 {
		return "", false
	}
	rest := text[at+len(from):]
	end := strings.Index(rest, to)
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

// betweenLast is the text between the last of a mark and the closing
// mark after it, when the text holds both.
func betweenLast(text, from, to string) (string, bool) {
	at := strings.LastIndex(text, from)
	if at < 0 {
		return "", false
	}
	rest := text[at+len(from):]
	end := strings.Index(rest, to)
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

// textOf is the text of a page's own words: its tags taken out, a line
// break for each BR, its entities read, each line trimmed with its
// whitespace folded, and the empty lines left out.
func textOf(raw string) string {
	broken := strings.ReplaceAll(raw, "<BR>", "\n")
	broken = strings.ReplaceAll(broken, "<br>", "\n")
	var words strings.Builder
	for at := 0; at < len(broken); at++ {
		if broken[at] == '<' {
			if end := strings.Index(broken[at:], ">"); end >= 0 {
				at += end
				continue
			}
		}
		words.WriteByte(broken[at])
	}
	var kept []string
	for _, line := range strings.Split(
		html.UnescapeString(words.String()), "\n") {
		if folded := strings.Join(
			strings.Fields(line), " "); folded != "" {
			kept = append(kept, folded)
		}
	}
	return strings.Join(kept, "\n")
}
