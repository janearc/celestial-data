package noaacrm

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

// Tag preserves an IFD value without claiming a CRS/datum conversion.
// TIFF field types and tag numbers are format definitions, not measurements.
type Tag struct {
	Type   uint16
	Count  uint32
	Values []float64 `json:",omitempty"`
	Text   string    `json:",omitempty"`
}

// Info is the classic TIFF first-directory metadata, not a pixel decoder.
type Info struct {
	Bytes     int64
	ByteOrder string
	Width     int
	Height    int
	Tags      map[uint16]Tag
}

// Inspect bounds all offset reads by the actual file and limits directory and
// metadata sizes. It rejects non-classic TIFF, duplicate tags and truncation.
func Inspect(path string) (Info, error) {
	var info Info
	f, err := os.Open(path)
	if err != nil {
		return info, err
	}
	defer f.Close()
	s, err := f.Stat()
	if err != nil {
		return info, err
	}
	info.Bytes = s.Size()
	read := func(offset int64, n int) ([]byte, error) {
		if offset < 0 || n < 0 || offset > info.Bytes-int64(n) {
			return nil, fmt.Errorf("TIFF offset outside file")
		}
		b := make([]byte, n)
		_, err := f.ReadAt(b, offset)
		return b, err
	}
	header, err := read(0, 8)
	if err != nil {
		return info, err
	}
	var order binary.ByteOrder
	switch string(header[:2]) {
	case "II":
		order, info.ByteOrder = binary.LittleEndian, "little"
	case "MM":
		order, info.ByteOrder = binary.BigEndian, "big"
	default:
		return info, fmt.Errorf("not TIFF byte order")
	}
	if order.Uint16(header[2:4]) != 42 {
		return info, fmt.Errorf("not classic TIFF magic 42")
	}
	offset := int64(order.Uint32(header[4:8]))
	count, err := read(offset, 2)
	if err != nil {
		return info, err
	}
	n := int(order.Uint16(count))
	if n == 0 || n > 256 {
		return info, fmt.Errorf("unexpected TIFF directory size")
	}
	directory, err := read(offset+2, n*12+4)
	if err != nil {
		return info, err
	}
	info.Tags = map[uint16]Tag{}
	for i := 0; i < n; i++ {
		e := directory[i*12 : (i+1)*12]
		id, typ, count := order.Uint16(e), order.Uint16(e[2:]),
			order.Uint32(e[4:])
		if _, exists := info.Tags[id]; exists {
			return info, fmt.Errorf("duplicate TIFF tag %d", id)
		}
		width := map[uint16]uint64{
			1: 1, 2: 1, 3: 2, 4: 4, 11: 4, 12: 8,
		}[typ]
		tag := Tag{Type: typ, Count: count}
		if width == 0 {
			info.Tags[id] = tag
			continue
		}
		length := uint64(count) * width
		if length > 1024*1024 {
			return info, fmt.Errorf(
				"TIFF tag %d exceeds metadata "+
					"budget", id)
		}
		var raw []byte
		if length <= 4 {
			raw = e[8 : 8+int(length)]
		} else {
			raw, err = read(int64(order.Uint32(e[8:])), int(length))
			if err != nil {
				return info, err
			}
		}
		if typ == 2 {
			tag.Text = string(raw)
		} else {
			for j := 0; j < int(count); j++ {
				p := raw[j*int(width):]
				var value float64
				switch typ {
				case 1:
					value = float64(p[0])
				case 3:
					value = float64(order.Uint16(p))
				case 4:
					value = float64(order.Uint32(p))
				case 11:
					value = float64(math.Float32frombits(
						order.Uint32(p)))
				case 12:
					value = math.Float64frombits(
						order.Uint64(p))
				}
				tag.Values = append(tag.Values, value)
			}
		}
		info.Tags[id] = tag
	}
	if len(info.Tags[256].Values) != 1 || len(info.Tags[257].Values) != 1 {
		return info, fmt.Errorf("missing TIFF dimensions")
	}
	info.Width = int(info.Tags[256].Values[0])
	info.Height = int(info.Tags[257].Values[0])
	return info, nil
}

type tiffCheck struct {
	width, height int
	dx, dy        float64
}

// Check verifies the TIFF against the requested native grid and byte ceiling.
func (c tiffCheck) Check(path string) error {
	info, err := Inspect(path)
	if err != nil {
		return err
	}
	if info.Bytes > Ceiling || info.Width < max(1, c.width-6) ||
		info.Height < max(1, c.height-6) || info.Width > c.width ||
		info.Height > c.height {
		return fmt.Errorf("TIFF exceeds requested " +
			"native-grid dimensions " +
			"or byte budget")
	}
	if !single(info.Tags[258], 32) || !single(info.Tags[277], 1) ||
		!single(info.Tags[339], 3) {
		return fmt.Errorf("response is not single-band float32 TIFF")
	}
	if len(info.Tags[33550].Values) != 3 ||
		len(info.Tags[33922].Values) < 6 ||
		len(info.Tags[34735].Values) < 4 {
		return fmt.Errorf("response lacks GeoTIFF scale, " +
			"tiepoint or geokeys")
	}
	scale := info.Tags[33550].Values
	if math.Abs(scale[0]-c.dx) > c.dx*1e-5 ||
		math.Abs(scale[1]-c.dy) > c.dy*1e-5 {
		return fmt.Errorf("response changed the described " +
			"native grid spacing")
	}
	// Prove every declared compressed or uncompressed strip is present.
	// No pixel decoding is implied; an external libtiff read checks the
	// full raster.
	offsets, counts := info.Tags[273].Values, info.Tags[279].Values
	if len(offsets) == 0 || len(offsets) != len(counts) {
		return fmt.Errorf("response lacks matching TIFF " +
			"strip offsets/counts")
	}
	for i, offset := range offsets {
		if offset < 8 || counts[i] <= 0 ||
			offset > float64(info.Bytes)-counts[i] {
			return io.ErrUnexpectedEOF
		}
	}
	return nil
}

// single reports whether t has one value, equal to value.
func single(t Tag, value float64) bool {
	return len(t.Values) == 1 && t.Values[0] == value
}
