package votable_test

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/votable"
	"github.com/janearc/celestial-data/web"
	"pgregory.net/rapid"
)

// The numbers these tests use.
const (
	first      = 0  // defined: the first row
	second     = 1  // defined: the second row
	rows       = 5  // defined: the TOP 5 the recorded queries asked for
	fields     = 5  // defined: the columns the recorded query asked for
	countWidth = 4  // defined: VOTable 1.4, a variable array's count
	scalar     = 1  // defined: one element
	floatBits  = 64 // defined: strconv's size for a float64
	singleBits = 32 // defined: strconv's size for a float32
	decimal    = 10 // defined: the base numbers are written in
	shortest   = -1 // defined by strconv: the fewest digits that read back
	none       = 0  // defined: no bytes, no elements
	most       = 8  // defined: the most elements a drawn array holds
	cut        = 1  // defined: one byte off the end of a stream
	gone       = -1 // defined: a row that is not there
)

// The numbers the hand-written tables hold.
const (
	flux   = 1.5e-12 // defined: the flux cellsTable writes
	answer = 42      // defined: the number a name is replaced with
	low    = 1.5     // defined: the first of arrayTable's pair
	high   = 2.5     // defined: the second of arrayTable's pair
	tenth  = 0.1     // defined: a float32 holds it only near, and a float64 writes that long
)

// recorded is a reply a service really gave, kept in testdata.
func recorded(t *testing.T, name string) *votable.Table {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	table, err := votable.Read(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return table
}

// expected is a table as another decoder wrote it, kept in testdata as
// comma-separated values with the names on the first line.
func expected(t *testing.T, name string) [][]string {
	t.Helper()
	file, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	lines, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return lines
}

// HEASARC's five longest NuSTAR observations within 0.1 degrees of Sgr A*
// read as the CSV beside them says. Another decoder wrote that CSV from
// the VOTable standard alone.
func TestReadsHEASARCsBinaryStream(t *testing.T) {
	table := recorded(t, "heasarc-numaster-sgra.xml")
	lines := expected(t, "heasarc-numaster-sgra.csv")
	names := lines[first]
	if len(names) != fields ||
		table.Len() != len(lines)-scalar || table.Len() != rows {
		t.Fatalf("%q, %d rows", names, table.Len())
	}
	for at := range table.Len() {
		for i, name := range names {
			got, _ := table.Text(at, name)
			if wanted := lines[at+scalar][i]; got != wanted {
				t.Errorf("row %d, %s: %q, want %q", at, name, got,
					wanted)
			}
		}
	}
}

// Chandra's obsid is an int where NuSTAR's is text; both read as text.
func TestReadsAnIntegerAsText(t *testing.T) {
	table := recorded(t, "heasarc-chanmaster-sgra.xml")
	obsid, err := table.Text(first, "obsid")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := strconv.Atoi(obsid); err != nil {
		t.Errorf("obsid %q", obsid)
	}
	exposure, err := table.Number(first, "exposure")
	if err != nil || !(exposure > none) {
		t.Errorf("exposure %v, %v", exposure, err)
	}
}

// Anti-tests: a service's own error is the error, in its own words.
func TestAServicesErrorIsTheError(t *testing.T) {
	raw, err := os.ReadFile("testdata/heasarc-error-no-column.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, err = votable.Read(bytes.NewReader(raw))
	if !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "instrument") {
		t.Errorf("%v", err)
	}
}

// cellsTable is a VOTable with the rows written as XML cells.
const cellsTable = `<?xml version="1.0"?>
<VOTABLE version="1.4"><RESOURCE type="results">
<INFO name="QUERY_STATUS" value="OK"/>
<TABLE>
<FIELD name="name" datatype="char" arraysize="*"/>
<FIELD name="count" datatype="int"/>
<FIELD name="flux" datatype="double"/>
<FIELD name="seen" datatype="boolean"/>
<DATA><TABLEDATA>
<TR><TD>Sgr A*</TD><TD>3392</TD><TD>1.5e-12</TD><TD>T</TD></TR>
<TR><TD> </TD><TD></TD><TD>NaN</TD><TD>?</TD></TR>
</TABLEDATA></DATA></TABLE></RESOURCE></VOTABLE>`

func TestReadsXMLCells(t *testing.T) {
	table, err := votable.Read(strings.NewReader(cellsTable))
	if err != nil {
		t.Fatal(err)
	}
	name, _ := table.Text(first, "name")
	count, _ := table.Text(first, "count")
	value, _ := table.Number(first, "flux")
	seen, _ := table.Text(first, "seen")
	if name != "Sgr A*" || count != "3392" || value != flux ||
		seen != "true" {
		t.Errorf("%q %q %v %q", name, count, value, seen)
	}
	empty, _ := table.Text(second, "count")
	unknown, _ := table.Text(second, "seen")
	blank, _ := table.Text(second, "name")
	if empty != "" || unknown != "" || blank != "" {
		t.Errorf("empty cells read as %q, %q, %q", empty, unknown, blank)
	}
	if _, err := table.Number(second, "count"); err == nil {
		t.Error("an empty cell is a number")
	}
}

// voTable wraps fields and a BINARY stream as a VOTable.
func voTable(fields, encoding string, stream []byte) string {
	text := base64.StdEncoding.EncodeToString(stream)
	return fmt.Sprintf(`<VOTABLE><RESOURCE><TABLE>%s<DATA><BINARY>`+
		`<STREAM encoding="%s">%s</STREAM></BINARY></DATA></TABLE>`+
		`</RESOURCE></VOTABLE>`, fields, encoding, text)
}

// refused reads a VOTable that must be refused with words that say so,
// and says what came if it is not.
func refused(t *testing.T, says, text string) {
	t.Helper()
	_, err := votable.Read(strings.NewReader(text))
	if !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), says) {
		t.Errorf("%s: %v", says, err)
	}
}

// Anti-tests: what this package does not read is refused, by name.
func TestWhatIsNotReadIsRefused(t *testing.T) {
	short := []byte{none, none, none, rows}
	refused(t, `datatype "bit"`,
		voTable(`<FIELD name="x" datatype="bit"/>`, "base64", short))
	refused(t, `a stream in "gzip"`,
		voTable(`<FIELD name="x" datatype="int"/>`, "gzip", short))
	refused(t, `arraysize "2x3"`,
		voTable(`<FIELD name="x" datatype="int" arraysize="2x3"/>`,
			"base64", short))
	refused(t, `arraysize "many"`,
		voTable(`<FIELD name="x" datatype="int" arraysize="many"/>`,
			"base64", short))
	refused(t, "bytes wanted",
		voTable(`<FIELD name="x" datatype="double"/>`, "base64", short))
	refused(t, "a count cut short",
		voTable(`<FIELD name="x" datatype="char" arraysize="*"/>`,
			"base64", short[:countWidth-cut]))
	refused(t, "no table", `<VOTABLE><RESOURCE/></VOTABLE>`)
}

// changed is cellsTable with one piece of text replaced.
func changed(old, new string) string {
	return strings.Replace(cellsTable, old, new, scalar)
}

// read is Read of a text.
func read(text string) (*votable.Table, error) {
	return votable.Read(strings.NewReader(text))
}

func TestWhatIsNotAVOTableIsAnError(t *testing.T) {
	oneInt := voTable(`<FIELD name="x" datatype="int"/>`, "base64",
		[]byte{none})
	for why, text := range map[string]string{
		"not XML":     "photons",
		"cut short":   `<VOTABLE><RESOURCE>`,
		"not base64":  strings.Replace(oneInt, "AA==", "!!!", scalar),
		"cells short": changed("<TD>T</TD>", ""),
		"bad number":  changed("3392", "many"),
		"cell array":  changed(`"int"/>`, `"int" arraysize="2"/>`),
	} {
		if _, err := read(text); err == nil {
			t.Errorf("%s: read", why)
		}
	}
}

func TestAskingForWhatIsNotThereIsAnError(t *testing.T) {
	table := recorded(t, "heasarc-numaster-sgra.xml")
	if _, err := table.Text(gone, "obsid"); err == nil {
		t.Error("a row before the first")
	}
	if _, err := table.Text(rows, "obsid"); err == nil {
		t.Error("a row after the last")
	}
	if _, err := table.Number(first, "nothing"); err == nil {
		t.Error("a field that is not there")
	}
}

// arrayTable is a stream of one row. It holds a double array of two, a
// truth and text in UCS-2.
func arrayTable() string {
	var stream bytes.Buffer
	binary.Write(&stream, binary.BigEndian, float64(low))
	binary.Write(&stream, binary.BigEndian, float64(high))
	stream.WriteByte('F')
	binary.Write(&stream, binary.BigEndian, uint32(scalar+scalar))
	binary.Write(&stream, binary.BigEndian, []uint16{'O', 'K'})
	return voTable(`<FIELD name="pair" datatype="double" arraysize="2"/>`+
		`<FIELD name="kept" datatype="boolean"/>`+
		`<FIELD name="word" datatype="unicodeChar" arraysize="*"/>`,
		"base64", stream.Bytes())
}

func TestReadsArraysTruthsAndWideText(t *testing.T) {
	table, err := read(arrayTable())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := table.Text(first, "pair"); err == nil {
		t.Error("an array read as text")
	}
	if _, err := table.Number(first, "pair"); err == nil {
		t.Error("an array read as one number")
	}
	if _, err := table.Number(first, "kept"); err == nil {
		t.Error("a truth read as a number")
	}
	kept, _ := table.Text(first, "kept")
	word, _ := table.Text(first, "word")
	if kept != "false" || word != "OK" {
		t.Errorf("%q, %q", kept, word)
	}
}

// integerTable is a stream of one row. It holds an unsigned byte, an int
// array of two and a truth array of two. Then it holds one more int.
func integerTable() string {
	var stream bytes.Buffer
	stream.WriteByte(byte(rows))
	binary.Write(&stream, binary.BigEndian, []int32{-scalar, rows})
	stream.Write([]byte("T?"))
	binary.Write(&stream, binary.BigEndian, int32(rows))
	return voTable(`<FIELD name="small" datatype="unsignedByte"/>`+
		`<FIELD name="pair" datatype="int" arraysize="2"/>`+
		`<FIELD name="marks" datatype="boolean" arraysize="2"/>`+
		`<FIELD name="last" datatype="int"/>`,
		"base64", stream.Bytes())
}

// Arrays are stepped over by their width. So what comes after each reads
// true. Arrays are neither text nor numbers.
func TestStepsOverArrays(t *testing.T) {
	table, err := read(integerTable())
	if err != nil {
		t.Fatal(err)
	}
	small, _ := table.Number(first, "small")
	last, _ := table.Number(first, "last")
	if small != rows || last != rows {
		t.Errorf("%v, %v", small, last)
	}
	for _, name := range []string{"pair", "marks"} {
		if _, err := table.Text(first, name); err == nil {
			t.Errorf("%s read as text", name)
		}
		if _, err := table.Number(first, name); err == nil {
			t.Errorf("%s read as a number", name)
		}
	}
}

func TestTextThatWritesANumberIsANumber(t *testing.T) {
	written := fmt.Sprintf(" %d ", answer)
	table, err := read(changed("Sgr A*", written))
	if err != nil {
		t.Fatal(err)
	}
	if number, err := table.Number(first, "name"); err != nil ||
		number != answer {
		t.Errorf("%v, %v", number, err)
	}
}

// singleTable is a VOTable with one float field, once as a stream and once
// as a cell. Each holds a float32 that a float64 cannot write short.
func singleTable(t *testing.T) (stream, cells *votable.Table) {
	t.Helper()
	near := float32(tenth)
	var raw bytes.Buffer
	binary.Write(&raw, binary.BigEndian, near)
	declared := `<FIELD name="x" datatype="float"/>`
	written := voTable(declared, "base64", raw.Bytes())
	stream, err := read(written)
	if err != nil {
		t.Fatal(err)
	}
	cells, err = read(fmt.Sprintf(`<VOTABLE><RESOURCE><TABLE>%s<DATA>`+
		`<TABLEDATA><TR><TD>%v</TD></TR></TABLEDATA></DATA></TABLE>`+
		`</RESOURCE></VOTABLE>`, declared, near))
	if err != nil {
		t.Fatal(err)
	}
	return stream, cells
}

// Floats are written at their own width, as the archive wrote them.
func TestAFloatIsWrittenAtItsWidth(t *testing.T) {
	stream, cells := singleTable(t)
	for _, each := range []*votable.Table{stream, cells} {
		text, _ := each.Text(first, "x")
		number, err := each.Number(first, "x")
		if text != strconv.FormatFloat(tenth, 'g', shortest, singleBits) ||
			err != nil || number != float64(float32(tenth)) {
			t.Errorf("%q, %v, %v", text, number, err)
		}
	}
}

// column is one drawn column. It keeps its declaration and how each value
// is written into a stream. It keeps the text each should read back as.
type column struct {
	declared string
	written  [][]byte
	text     []string
}

// drawColumn draws a column of a datatype for a number of rows.
func drawColumn(t *rapid.T, at, count int) column {
	kind := rapid.SampledFrom([]string{"int", "long", "double", "float",
		"short", "text"}).Draw(t, "kind")
	name := fmt.Sprintf("c%d", at)
	drawn := column{declared: fmt.Sprintf(`<FIELD name="%s" datatype="%s"/>`,
		name, kind)}
	for range count {
		written, text := drawValue(t, kind)
		drawn.written = append(drawn.written, written)
		drawn.text = append(drawn.text, text)
	}
	if kind == "text" {
		drawn.declared = fmt.Sprintf(`<FIELD name="%s" datatype="char" `+
			`arraysize="*"/>`, name)
	}
	return drawn
}

// drawValue draws one value of a kind: its bytes in a stream, and its
// text.
func drawValue(t *rapid.T, kind string) ([]byte, string) {
	var stream bytes.Buffer
	switch kind {
	case "int":
		value := rapid.Int32().Draw(t, "int")
		binary.Write(&stream, binary.BigEndian, value)
		return stream.Bytes(), strconv.Itoa(int(value))
	case "long":
		value := rapid.Int64().Draw(t, "long")
		binary.Write(&stream, binary.BigEndian, value)
		return stream.Bytes(), strconv.FormatInt(value, decimal)
	case "short":
		value := rapid.Int16().Draw(t, "short")
		binary.Write(&stream, binary.BigEndian, value)
		return stream.Bytes(), strconv.Itoa(int(value))
	case "float":
		value := rapid.Float32().Draw(t, "float")
		binary.Write(&stream, binary.BigEndian, value)
		wide := float64(value)
		return stream.Bytes(), strconv.FormatFloat(wide, 'g', shortest,
			singleBits)
	case "double":
		value := rapid.Float64().Draw(t, "double")
		binary.Write(&stream, binary.BigEndian, value)
		return stream.Bytes(), strconv.FormatFloat(value, 'g', shortest,
			floatBits)
	}
	value := rapid.StringMatching(`[A-Za-z0-9 _*+-]{0,8}`).Draw(t, "text")
	length := uint32(len(value))
	binary.Write(&stream, binary.BigEndian, length)
	stream.WriteString(value)
	return stream.Bytes(), strings.TrimRight(value, " ")
}

// roundTrips is the property of TestEveryDrawnTableReadsBack, for one
// draw.
func roundTrips(t *rapid.T) {
	count := rapid.IntRange(none, most).Draw(t, "rows")
	width := rapid.IntRange(scalar, most).Draw(t, "columns")
	columns := make([]column, width)
	var fields strings.Builder
	for i := range columns {
		columns[i] = drawColumn(t, i, count)
		fields.WriteString(columns[i].declared)
	}
	var stream bytes.Buffer
	for at := range count {
		for _, each := range columns {
			stream.Write(each.written[at])
		}
	}
	text := voTable(fields.String(), "base64", stream.Bytes())
	table, err := read(text)
	if err != nil {
		t.Fatal(err)
	}
	for at := range count {
		for i, each := range columns {
			got, _ := table.Text(at, fmt.Sprintf("c%d", i))
			if got != each.text[at] {
				t.Fatalf("row %d column %d: %q, want %q", at, i, got,
					each.text[at])
			}
		}
	}
}

// Property: a table written as a BINARY stream by the VOTable rules reads
// back to what was written, row by row and column by column.
func TestEveryDrawnTableReadsBack(t *testing.T) {
	rapid.Check(t, roundTrips)
}

// Only the first table is read: the second's fields are not its.
func TestOnlyTheFirstTableIsRead(t *testing.T) {
	second := strings.Replace(cellsTable, "</TABLE>", "</TABLE><TABLE>"+
		`<FIELD name="later" datatype="int"/><DATA><TABLEDATA>`+
		"<TR><TD>1</TD></TR></TABLEDATA></DATA></TABLE>", scalar)
	table, err := read(second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := table.Text(first, "later"); err == nil ||
		table.Len() != scalar+scalar {
		t.Errorf("%d rows, later: %v", table.Len(), err)
	}
}

// Services' errors are their words alone, not text that follows them.
func TestAServicesWordsEndWithTheirElement(t *testing.T) {
	text := `<VOTABLE><RESOURCE><INFO name="QUERY_STATUS" value="ERROR">` +
		`no such table</INFO>later words<TABLE/></RESOURCE></VOTABLE>`
	_, err := read(text)
	if err == nil || !strings.HasSuffix(err.Error(), "says: no such table") {
		t.Errorf("%v", err)
	}
}

// Errors in a row say which row, counting from one.
func TestARowsErrorSaysWhichRow(t *testing.T) {
	short := []byte{none, none, none, rows}
	for what, text := range map[string]string{
		"stream": voTable(`<FIELD name="x" datatype="double"/>`, "base64",
			short),
		"cells":  changed("<TD>T</TD>", ""),
		"number": changed("3392", "many"),
	} {
		if _, err := read(text); err == nil ||
			!strings.Contains(err.Error(), "row 1") {
			t.Errorf("%s: %v", what, err)
		}
	}
}

// Arrays of no elements are read as such, and the field after them too.
func TestAnEmptyArrayIsAnArray(t *testing.T) {
	var stream bytes.Buffer
	binary.Write(&stream, binary.BigEndian, int32(rows))
	text := voTable(`<FIELD name="none" datatype="int" `+
		`arraysize="0"/><FIELD name="x" datatype="int"/>`, "base64",
		stream.Bytes())
	table, err := read(text)
	if err != nil {
		t.Fatal(err)
	}
	if x, err := table.Number(first, "x"); err != nil || x != rows {
		t.Errorf("%v, %v", x, err)
	}
}

// A stream that ends right after a count of no elements is whole: the
// count is all its row holds, and is not cut short.
func TestACountOfNoElementsEndingTheStreamIsWhole(t *testing.T) {
	text := voTable(`<FIELD name="word" datatype="char" arraysize="*"/>`,
		"base64", make([]byte, countWidth))
	table, err := read(text)
	if err != nil {
		t.Fatal(err)
	}
	word, err := table.Text(first, "word")
	if table.Len() != scalar || err != nil || word != "" {
		t.Errorf("%d rows, %q, %v", table.Len(), word, err)
	}
}

// Numbers asked of a row that is not there say so.
func TestANumberOfNoRowSaysSo(t *testing.T) {
	table := recorded(t, "heasarc-numaster-sgra.xml")
	if _, err := table.Number(rows, "obsid"); err == nil ||
		!strings.Contains(err.Error(), "there is no row") {
		t.Errorf("%v", err)
	}
}

// Anti-test: a stream whose rows take no bytes is refused, not read for
// ever. The read is given the picked patience to end.
func TestARowOfNoBytesIsRefused(t *testing.T) {
	text := voTable(`<FIELD name="none" datatype="int" arraysize="0"/>`,
		"base64", []byte{none})
	ended := make(chan error, scalar)
	go readInto(ended, text)
	select {
	case err := <-ended:
		if !errors.Is(err, celestialdata.Refused()) ||
			!strings.Contains(err.Error(), "a row of no bytes") {
			t.Errorf("%v", err)
		}
	case <-time.After(patience(t)):
		t.Fatal("the read did not end")
	}
}

// readInto reads a VOTable and sends what Read said.
func readInto(ended chan<- error, text string) {
	_, err := read(text)
	ended <- err
}

// patience is how long a test waits for Read to end.
func patience(t *testing.T) time.Duration {
	t.Helper()
	raw, err := os.ReadFile("testdata/numbers.yaml")
	if err != nil {
		t.Fatal(err)
	}
	wait, err := celestialdata.NewMeasured("numbers.yaml", raw).Get("patience")
	if err != nil {
		t.Fatal(err)
	}
	return web.Seconds(wait)
}

// XML that stops short is not a VOTable, and says so.
func TestXMLCutShortIsNotAVOTable(t *testing.T) {
	if _, err := read(`<VOTABLE><RESOURCE>`); err == nil ||
		!strings.Contains(err.Error(), "not a VOTable") {
		t.Errorf("%v", err)
	}
}
