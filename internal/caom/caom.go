// Package caom asks MAST's API about the observations it keeps in its
// Common Archive Observation Model: those whose footprint meets a circle,
// and the files of each. It is what the tess and k2 adapters ask MAST
// through.
//
// The questions go to MAST's API by the CAOM cone search and then each
// observation's products, read 2026-11-06 from MAST's pages and its
// answers (sources.yaml).
//
// MAST answers a question it is still working on with EXECUTING, and is
// asked again until it is done. Its refusals are the adapter's that
// asks.
package caom

import (
	_ "embed"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/url"
	"strconv"

	"github.com/janearc/celestial-data"
)

// MAST's places: where its API is asked, and where a product is fetched
// by its data URI.
const (
	invoke   = "https://mast.stsci.edu/api/v0/invoke"
	download = "https://mast.stsci.edu/api/v0.1/Download/file"
)

// What MAST is asked and answers, as its pages and answers name them.
const (
	coneService     = "Mast.Caom.Cone"
	productsService = "Mast.Caom.Products"
	answerFormat    = "json"
	complete        = "COMPLETE"
	executing       = "EXECUTING"
)

// The numbers file, and the name in it of how many times a question is
// asked while MAST is still working on it, a number picked there.
const (
	numbersFile = "numbers.yaml"
	mostAsks    = "most-asks"
)

//go:embed numbers.yaml
var numbersRaw []byte

// Asker asks MAST's API for an adapter, whose refusals its are.
type Asker struct {
	base *celestialdata.Adapter
	most int
}

// New is an asker for the adapter base.
func New(base *celestialdata.Adapter) *Asker {
	numbers := celestialdata.NewMeasured(numbersFile, numbersRaw)
	return &Asker{base: base, most: int(numbers.Value(mostAsks))}
}

// Observation is a row of a cone search's answer.
type Observation struct {
	Collection string  `json:"obs_collection"`
	Provenance string  `json:"provenance_name"`
	Project    string  `json:"project"`
	Kind       string  `json:"dataproduct_type"`
	Target     string  `json:"target_name"`
	ObsID      int64   `json:"obsid"`
	Sequence   int64   `json:"sequence_number"`
	Exposure   float64 `json:"t_exptime"`
}

// File is a row of a products search's answer.
type File struct {
	Name string `json:"productFilename"`
	Kind string `json:"productType"`
	URI  string `json:"dataURI"`
	Size int64  `json:"size"`
}

// request is a question to MAST's API: its service and its parameters,
// written as JSON.
type request struct {
	service, params string
}

// written is the request as MAST's request object, its fields in a fixed
// order so the same question is always the same address. Its strings
// are the package's names and digits, which Go and JSON quote alike.
func (question request) written() string {
	return fmt.Sprintf(`{"service":%q,"params":%s,"format":%q}`,
		question.service, question.params, answerFormat)
}

// coneParams is a cone search's parameters, in degrees, each the
// shortest decimal that is the number: Near holds only finite numbers.
func coneParams(near *celestialdata.Near) string {
	centre := near.Centre()
	ra, dec := decimal(centre.RA()), decimal(centre.Dec())
	radius := decimal(near.Radius())
	return fmt.Sprintf(`{"ra":%s,"dec":%s,"radius":%s}`, ra, dec, radius)
}

// decimal is a number as its shortest decimal.
func decimal(number float64) string {
	return strconv.FormatFloat(number, 'g', shortest, bits)
}

// The decimal's form, each defined by strconv: the fewest digits that
// read back as the number, of a 64-bit float.
const (
	shortest = -1 // defined: strconv's precision for the fewest digits
	bits     = 64 // defined: a float64's bits
)

// Cone is each observation whose footprint meets the circle, in the
// order MAST gives them.
func (asker *Asker) Cone(near *celestialdata.Near) ([]Observation,
	error) {
	asked := request{service: coneService, params: coneParams(near)}
	var seen []Observation
	if err := asker.ask(asked, &seen); err != nil {
		return nil, err
	}
	return seen, nil
}

// Files is each file of an observation, in the order MAST gives them.
func (asker *Asker) Files(id int64) ([]File, error) {
	asked := request{service: productsService,
		params: fmt.Sprintf(`{"obsid":%q}`, fmt.Sprint(id))}
	var files []File
	if err := asker.ask(asked, &files); err != nil {
		return nil, err
	}
	return files, nil
}

// Download is the address a file is fetched from, by its data URI.
func Download(uri string) string {
	query := url.Values{"uri": {uri}}
	return download + "?" + query.Encode()
}

// ask asks MAST a question until it is done, and reads its rows into
// rows. It refuses an answer that is not MAST's, one MAST says failed,
// and a question still not done after the asks most-asks allows.
func (asker *Asker) ask(question request, rows any) error {
	query := url.Values{"request": {question.written()}}
	address := invoke + "?" + query.Encode()
	for range asker.most {
		said, err := asker.answer(address)
		if err != nil {
			return err
		}
		switch said.Status {
		case executing:
			continue
		case complete:
			if err := json.Unmarshal(said.Data, rows); err != nil {
				return asker.base.Fail("%s's rows: %w",
					question.service, err)
			}
			return nil
		}
		return asker.base.Refuse("%s says %s: %s", question.service,
			said.Status, said.Message)
	}
	return asker.base.Refuse("%s was still executing after %d asks",
		question.service, asker.most)
}

// answer is MAST's answer, its rows still to be read.
type answer struct {
	Status  string         `json:"status"`
	Message string         `json:"msg"`
	Data    jsontext.Value `json:"data"`
}

// answer is MAST's answer at an address, read whole.
func (asker *Asker) answer(address string) (*answer, error) {
	body, _, err := asker.base.Web().Get(address)
	if err != nil {
		return nil, asker.base.Wrap(err)
	}
	defer body.Close()
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, asker.base.Wrap(err)
	}
	var said answer
	if err := json.Unmarshal(raw, &said); err != nil {
		return nil, asker.base.Fail(
			"an answer that is not MAST's: %w", err)
	}
	return &said, nil
}
