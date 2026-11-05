// Package votable reads a VOTable. That is the table the Virtual
// Observatory's services answer with. It reads the fields and the rows.
// The rows come as XML cells or as a `BINARY` stream in base64.
//
// It reads what the archives here send. That is numbers and arrays of
// them in one dimension, text and truths. Anything else is an error
// that names it.
package votable

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/janearc/celestial-data"
)

// The widths of one element in bytes, by the VOTable 1.4 standard.
const (
	oneByte    = 1 // defined: a boolean, an unsigned byte or a char
	twoBytes   = 2 // defined: a short, or a unicodeChar in UCS-2
	fourBytes  = 4 // defined: an int, or an IEEE single float
	eightBytes = 8 // defined: a long, or an IEEE double
)

// widthOf is the width of one element of a datatype, and whether the
// datatype is one this package reads.
func widthOf(datatype string) (int, bool) {
	switch datatype {
	case "boolean", "unsignedByte", "char":
		return oneByte, true
	case "short", "unicodeChar":
		return twoBytes, true
	case "int", "float":
		return fourBytes, true
	case "long", "double":
		return eightBytes, true
	}
	return nothing, false
}

// The counts the decoding works with.
const (
	scalar     = 1  // defined: a field with no arraysize holds one
	countWidth = 4  // defined: VOTable 1.4, a variable array's count
	nothing    = 0  // defined: no bytes, no rows, no elements
	floatBits  = 64 // defined: strconv's size for a float64
	singleBits = 32 // defined: strconv's size for a float32
	intBits    = 64 // defined: strconv's size for an int64
	decimal    = 10 // defined: the base numbers are written in
	shortest   = -1 // defined by strconv: the fewest digits that read back
)

// errRefused is wrapped by each error about what this package does not
// read: a service's own error, a datatype or encoding it does not know
// and a row cut short. It is celestial-data's, as every refusal here is.
var errRefused = celestialdata.Refused()

// Table is the fields of a VOTable and its rows.
type Table struct {
	fields []field
	rows   [][]any
}

// field is one column as it is declared: its name, datatype and arraysize.
type field struct {
	name, datatype, arraysize string
}

// The elements the walk keeps the text of.
const (
	inNothing = ""
	inStatus  = "INFO"
	inStream  = "STREAM"
	inCell    = "TD"
)

// onlyTable is the count of tables the walk has opened while it is in the
// first. It's defined: Read reads the first table and no other.
const onlyTable = 1

// walk is what a walk through a VOTable's elements has kept so far.
type walk struct {
	votable, failed bool
	tables          int
	in              string
	fields          []field
	encoding        string
	stream, said    strings.Builder
	cell            strings.Builder
	row             []string
	rows            [][]string
}

// Read is the first table of a VOTable. When the service answered with an
// error, Read gives that error.
func Read(from io.Reader) (*Table, error) {
	decoder := xml.NewDecoder(from)
	kept := &walk{}
	token, err := decoder.Token()
	for err == nil {
		kept.take(token)
		token, err = decoder.Token()
	}
	if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("not a VOTable: %w", err)
	}
	return kept.table()
}

// take keeps what one token says.
func (kept *walk) take(token xml.Token) {
	switch each := token.(type) {
	case xml.StartElement:
		kept.open(each)
	case xml.EndElement:
		kept.close(each.Name.Local)
	case xml.CharData:
		kept.text(each)
	}
}

// open keeps what an element's start says. Of the table's elements, it
// keeps only the first table's.
func (kept *walk) open(element xml.StartElement) {
	name := element.Name.Local
	switch name {
	case "VOTABLE":
		kept.votable = true
	case "TABLE":
		kept.tables++
	case "INFO":
		kept.status(element)
	}
	if kept.tables != onlyTable {
		return
	}
	switch name {
	case "FIELD":
		kept.fields = append(kept.fields, field{
			name:      attribute(element, "name"),
			datatype:  attribute(element, "datatype"),
			arraysize: attribute(element, "arraysize"),
		})
	case inStream:
		kept.encoding = attribute(element, "encoding")
		kept.in = inStream
	case "TR":
		kept.row = []string{}
	case inCell:
		kept.cell.Reset()
		kept.in = inCell
	}
}

// status keeps a query status that says the query failed, and its words.
func (kept *walk) status(element xml.StartElement) {
	if attribute(element, "name") == "QUERY_STATUS" &&
		attribute(element, "value") == "ERROR" {
		kept.failed = true
		kept.in = inStatus
	}
}

// close ends what an element kept.
func (kept *walk) close(name string) {
	if kept.tables == onlyTable {
		switch name {
		case inCell:
			kept.row = append(kept.row, kept.cell.String())
		case "TR":
			kept.rows = append(kept.rows, kept.row)
		}
	}
	if name == kept.in {
		kept.in = inNothing
	}
}

// text keeps the text of the element the walk is in, if it keeps that.
func (kept *walk) text(data xml.CharData) {
	switch kept.in {
	case inStatus:
		kept.said.Write(data)
	case inStream:
		kept.stream.Write(data)
	case inCell:
		kept.cell.Write(data)
	}
}

// attribute is the value of an element's attribute, or nothing.
func attribute(element xml.StartElement, name string) string {
	for _, each := range element.Attr {
		if each.Name.Local == name {
			return each.Value
		}
	}
	return ""
}

// table reads what the walk kept. That is the service's error if it said
// one. Otherwise it is the first table's rows.
func (kept *walk) table() (*Table, error) {
	switch {
	case !kept.votable:
		return nil, fmt.Errorf("%w: not a VOTable", errRefused)
	case kept.failed:
		said := strings.TrimSpace(kept.said.String())
		return nil, fmt.Errorf("%w: the service says: %s", errRefused,
			said)
	case kept.tables == nothing:
		return nil, fmt.Errorf("%w: no table in the VOTable",
			errRefused)
	}
	for _, each := range kept.fields {
		if _, known := widthOf(each.datatype); !known {
			return nil, fmt.Errorf("%w: field %s has datatype %q",
				errRefused, each.name, each.datatype)
		}
	}
	read := &Table{fields: kept.fields}
	text := strings.TrimSpace(kept.stream.String())
	if text == "" {
		return read, read.fromCells(kept.rows)
	}
	if kept.encoding != "base64" {
		return nil, fmt.Errorf("%w: a stream in %q", errRefused,
			kept.encoding)
	}
	raw, err := base64.StdEncoding.DecodeString(squeezed(text))
	if err != nil {
		return nil, fmt.Errorf("the stream: %w", err)
	}
	return read, read.fromStream(raw)
}

// squeezed is base64 text without the spaces and line breaks a stream is
// wrapped with.
func squeezed(text string) string {
	return strings.Join(strings.Fields(text), "")
}

// fromStream reads the rows of a `BINARY` stream, one after another. Each
// row that takes no bytes from a stream with bytes left is refused: the
// rows after it would never end.
func (table *Table) fromStream(raw []byte) error {
	for len(raw) > nothing {
		before := len(raw)
		cells := make([]any, len(table.fields))
		for i, each := range table.fields {
			value, rest, err := each.decode(raw)
			if err != nil {
				at := len(table.rows) + scalar
				return fmt.Errorf("row %d, %s: %w", at,
					each.name, err)
			}
			cells[i], raw = value, rest
		}
		if len(raw) == before {
			return fmt.Errorf("%w: a row of no bytes, %d bytes "+
				"left", errRefused, before)
		}
		table.rows = append(table.rows, cells)
	}
	return nil
}

// count is how many elements the field holds, and whether a count comes
// before them in a stream.
func (each field) count() (elements int, counted bool, err error) {
	size := each.arraysize
	switch {
	case size == "":
		return scalar, false, nil
	case strings.HasSuffix(size, "*"):
		return nothing, true, nil
	case strings.Contains(size, "x"):
		return nothing, false, fmt.Errorf(
			"%w: an array of arraysize %q", errRefused, size)
	}
	elements, err = strconv.Atoi(size)
	if err != nil || elements < nothing {
		return nothing, false, fmt.Errorf("%w: arraysize %q",
			errRefused, size)
	}
	return elements, false, nil
}

// decode reads one cell of the field from the front of a stream and
// gives the rest.
func (each field) decode(raw []byte) (any, []byte, error) {
	elements, counted, err := each.count()
	if err != nil {
		return nil, nil, err
	}
	if counted {
		if len(raw) < countWidth {
			return nil, nil, fmt.Errorf("%w: a count cut short",
				errRefused)
		}
		elements = int(binary.BigEndian.Uint32(raw))
		raw = raw[countWidth:]
	}
	one, _ := widthOf(each.datatype)
	width := one * elements
	if width > len(raw) || width < nothing {
		return nil, nil, fmt.Errorf("%w: %d bytes wanted, %d left",
			errRefused, width, len(raw))
	}
	return each.value(raw[:width], elements), raw[width:], nil
}

// value is the cell held in exactly its bytes. Characters are text. Each
// scalar is one value. Arrays of numbers are stepped over: none of the
// archives here sends one celestial-data reads, so each is kept as an array.
func (each field) value(cell []byte, elements int) any {
	width, _ := widthOf(each.datatype)
	switch each.datatype {
	case "char":
		return strings.TrimRight(string(cell), "\x00 ")
	case "unicodeChar":
		units := make([]uint16, elements)
		for i := range units {
			units[i] = binary.BigEndian.Uint16(cell[i*width:])
		}
		text := string(utf16.Decode(units))
		return strings.TrimRight(text, "\x00 ")
	}
	if each.arraysize == "" {
		return element(each.datatype, cell)
	}
	return array{}
}

// element is one number or truth at the front of its bytes. The truth
// that is neither true nor false is nil.
func element(datatype string, cell []byte) any {
	switch datatype {
	case "boolean":
		return truth(cell[nothing])
	case "unsignedByte":
		return int64(cell[nothing])
	case "short":
		bits := binary.BigEndian.Uint16(cell)
		return int64(int16(bits))
	case "int":
		bits := binary.BigEndian.Uint32(cell)
		return int64(int32(bits))
	case "long":
		return int64(binary.BigEndian.Uint64(cell))
	case "float":
		bits := binary.BigEndian.Uint32(cell)
		return math.Float32frombits(bits)
	}
	bits := binary.BigEndian.Uint64(cell)
	return math.Float64frombits(bits)
}

// truth is a truth element. It is nil when it is neither true nor false.
func truth(mark byte) any {
	switch mark {
	case 'T', 't', '1':
		return true
	case 'F', 'f', '0':
		return false
	}
	return nil
}

// array is a cell that holds an array of numbers, stepped over.
type array struct{}

// fromCells reads the rows of `TABLEDATA`, one XML cell to a field.
func (table *Table) fromCells(rows [][]string) error {
	for i, cells := range rows {
		if len(cells) != len(table.fields) {
			at, wanted := i+scalar, len(table.fields)
			return fmt.Errorf("%w: row %d has %d cells for %d "+
				"fields", errRefused, at, len(cells), wanted)
		}
		values := make([]any, len(cells))
		for place, text := range cells {
			value, err := table.fields[place].parse(text)
			if err != nil {
				return fmt.Errorf("row %d, %s: %w", i+scalar,
					table.fields[place].name, err)
			}
			values[place] = value
		}
		table.rows = append(table.rows, values)
	}
	return nil
}

// parse is one XML cell of the field. Characters are text. The empty cell
// is nil. Anything else is the number or truth it writes.
func (each field) parse(text string) (any, error) {
	text = strings.TrimSpace(text)
	switch {
	case each.datatype == "char" || each.datatype == "unicodeChar":
		return text, nil
	case text == "":
		return nil, nil
	case each.arraysize != "":
		return nil, fmt.Errorf("%w: an array in a cell", errRefused)
	case each.datatype == "boolean":
		return truth(text[nothing]), nil
	case each.datatype == "float":
		number, err := strconv.ParseFloat(text, singleBits)
		return float32(number), err
	case each.datatype == "double":
		return strconv.ParseFloat(text, floatBits)
	}
	return strconv.ParseInt(text, decimal, intBits)
}

// Len is the number of rows.
func (table *Table) Len() int { return len(table.rows) }

// cell is the value in a row under a field's name.
func (table *Table) cell(at int, name string) (any, error) {
	if at < nothing || at >= len(table.rows) {
		return nil, fmt.Errorf("there is no row %d of %d", at,
			len(table.rows))
	}
	for i, each := range table.fields {
		if each.name == name {
			return table.rows[at][i], nil
		}
	}
	return nil, fmt.Errorf("there is no field %q", name)
}

// Text is the value in a row under a field's name, as text. Numbers are
// written in full and truths as true or false. Nothing is written as nothing.
func (table *Table) Text(at int, name string) (string, error) {
	value, err := table.cell(at, name)
	if err != nil {
		return "", err
	}
	switch kept := value.(type) {
	case string:
		return kept, nil
	case int64:
		return strconv.FormatInt(kept, decimal), nil
	case float64:
		return strconv.FormatFloat(kept, 'g', shortest, floatBits), nil
	case float32:
		wide := float64(kept)
		return strconv.FormatFloat(wide, 'g', shortest, singleBits), nil
	case bool:
		return strconv.FormatBool(kept), nil
	case nil:
		return "", nil
	}
	return "", fmt.Errorf("%s in row %d is an array", name, at)
}

// Number is the value in a row under a field's name, as a number. Text is
// read as a number, and nothing is an error.
func (table *Table) Number(at int, name string) (float64, error) {
	value, err := table.cell(at, name)
	if err != nil {
		return math.NaN(), err
	}
	switch kept := value.(type) {
	case int64:
		return float64(kept), nil
	case float32:
		return float64(kept), nil
	case float64:
		return kept, nil
	case string:
		return strconv.ParseFloat(strings.TrimSpace(kept), floatBits)
	}
	return math.NaN(), fmt.Errorf("%s in row %d is not a number", name, at)
}
