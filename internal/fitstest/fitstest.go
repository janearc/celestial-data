// Package fitstest writes the FITS files the survey readers are tested
// on: a primary array of nothing, then one binary table of the columns
// and rows a test lays out, as the FITS Standard 4.0 lays them.
package fitstest

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// The standard's numbers the files are written by, each defined by the
// FITS Standard 4.0, as recalled.
const (
	block      = 2880 // defined: a block's bytes
	cardWidth  = 80   // defined: a card's characters
	keyWidth   = 8    // defined: a keyword's characters
	firstFrom  = 1    // defined: TTYPEn counts columns from one
	oneValue   = 1    // defined: a form with no count holds one value
	lastOf     = 1    // defined: a string's last byte is one before its length
	nothing    = 0    // defined: no bytes
	padding    = ' '  // defined: a header is padded with spaces
	oneByte    = 1    // defined: an L, B or A value's bytes
	twoBytes   = 2    // defined: an I value's bytes
	fourBytes  = 4    // defined: a J or E value's bytes
	eightBytes = 8    // defined: a K or D value's bytes
)

// Column is a column of the table: its name, its TFORM, its TUNIT, or
// none, and its TNULL, or none.
type Column struct {
	name, form, unit string
	null             *int64
}

// NewColumn is a column of a name and a TFORM, with no unit.
func NewColumn(name, form string) Column {
	return Column{name: name, form: form}
}

// In is the column with a TUNIT.
func (column Column) In(unit string) Column {
	column.unit = unit
	return column
}

// Nulled is the column with a TNULL: the whole number that stands for
// none.
func (column Column) Nulled(null int64) Column {
	column.null = &null
	return column
}

// Name is the column's name.
func (column Column) Name() string { return column.name }

// width is the bytes a column takes in a row: its count, one when the
// form gives none, times its kind's width.
func (column Column) width() int {
	letter := column.form[len(column.form)-lastOf]
	digits := strings.TrimSuffix(column.form, string(letter))
	count, err := strconv.Atoi(digits)
	if err != nil {
		count = oneValue
	}
	return count * widthOf(letter)
}

// widthOf is the bytes one value of a kind takes, by its TFORM letter.
func widthOf(letter byte) int {
	switch letter {
	case 'K', 'D':
		return eightBytes
	case 'J', 'E':
		return fourBytes
	case 'I':
		return twoBytes
	}
	return oneByte
}

// File is a FITS file of a primary array of nothing and a binary table
// of the columns and rows given, each row's bytes as a Row wrote them.
func File(columns []Column, rows ...*Row) []byte {
	primary := hdu([]string{card("SIMPLE", "T"), card("BITPIX", "8"),
		card("NAXIS", "0")}, nil)
	var data bytes.Buffer
	for _, row := range rows {
		data.Write(row.written.Bytes())
	}
	header := cards(len(rows), columns)
	table := hdu(header, data.Bytes())
	return append(primary, table...)
}

// cards are the table's header cards for so many rows of the columns.
func cards(rows int, columns []Column) []string {
	width := nothing
	for _, each := range columns {
		width += each.width()
	}
	fields := fmt.Sprint(len(columns))
	written := []string{card("XTENSION", "'BINTABLE'"), card("BITPIX", "8"),
		card("NAXIS", "2"), card("NAXIS1", fmt.Sprint(width)),
		card("NAXIS2", fmt.Sprint(rows)), card("PCOUNT", "0"),
		card("GCOUNT", "1"), card("TFIELDS", fields)}
	for at, each := range columns {
		number := at + firstFrom
		written = append(written, quoted("TTYPE", number, each.name),
			quoted("TFORM", number, each.form))
		if each.null != nil {
			key, null := fmt.Sprintf("TNULL%d", number), fmt.Sprint(*each.null)
			written = append(written, card(key, null))
		}
		if each.unit != "" {
			written = append(written, quoted("TUNIT", number, each.unit))
		}
	}
	return written
}

// card is one header card: a keyword and its value.
func card(key, value string) string {
	return fmt.Sprintf("%-*s= %s", keyWidth, key, value)
}

// quoted is a card of a numbered keyword with a string value.
func quoted(prefix string, number int, value string) string {
	return card(fmt.Sprintf("%s%d", prefix, number), fmt.Sprintf("'%s'", value))
}

// hdu is a header's cards and its data as the standard lays them: each
// card padded to its width, the header ended and padded to whole blocks
// with spaces, the data padded with zeros.
func hdu(cards []string, data []byte) []byte {
	var out bytes.Buffer
	for _, each := range append(cards, "END") {
		fmt.Fprintf(&out, "%-*s", cardWidth, each)
	}
	for out.Len()%block != nothing {
		out.WriteByte(padding)
	}
	out.Write(data)
	for out.Len()%block != nothing {
		out.WriteByte(nothing)
	}
	return out.Bytes()
}

// Row is a row's bytes, written a value at a time in the table's
// column order, big-endian as the standard stores numbers.
type Row struct{ written bytes.Buffer }

// NewRow is a row with nothing written yet.
func NewRow() *Row { return &Row{} }

// Long writes a K value.
func (row *Row) Long(value int64) *Row { return row.put(value) }

// Int writes a J value.
func (row *Row) Int(value int32) *Row { return row.put(value) }

// Single writes an E value.
func (row *Row) Single(value float32) *Row { return row.put(value) }

// Double writes a D value.
func (row *Row) Double(value float64) *Row { return row.put(value) }

// Text writes an nA value: the text, padded with spaces to its width.
func (row *Row) Text(value string, width int) *Row {
	fmt.Fprintf(&row.written, "%-*s", width, value)
	return row
}

// Skip writes so many bytes of nothing, for a column the reader steps
// over.
func (row *Row) Skip(bytes int) *Row {
	row.written.Write(make([]byte, bytes))
	return row
}

// put writes a number big-endian.
func (row *Row) put(value any) *Row {
	binary.Write(&row.written, binary.BigEndian, value)
	return row
}
