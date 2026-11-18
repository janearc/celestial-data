package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/earth"
	"github.com/janearc/celestial-data/web"
	"github.com/janearc/puzzlemouth/pzm"
	"github.com/janearc/puzzlemouth/verb"
)

const (
	earthSourcesPage = "Lists terrestrial sources, showing the search " +
		"inputs for each adapter and catalog references for the rest."
	earthFindPage = "Finds one terrestrial source's products. Name the " +
		"source and give either --bounds or --point as it takes."
	earthFetchPage = "Fetches one terrestrial source's products into " +
		"its own subfolder under --into."
)

// declareEarth adds the geographic catalog, search and fetch verbs.
func (tool command) declareEarth(declared *pzm.Tool) {
	flags := []pzm.Flag{
		pzm.Value("bounds", verb.NoShort,
			"west,south,east,north in degrees"),
		pzm.Value("point", verb.NoShort,
			"latitude,longitude in degrees"),
		pzm.Value("text", verb.NoShort,
			"search NASA GIBS layers by name"),
		pzm.Value("most", verb.NoShort,
			"maximum catalog results, 1 to 50"),
	}
	declared.Verb("earth-sources", verb.NoShort, "lists terrestrial "+
		"sources", earthSourcesPage, "Example_earthSources", listing,
		tool.earthSources)
	declared.Verb("earth-find", verb.NoShort, "finds terrestrial products",
		earthFindPage, "Example_earthFind", asking,
		tool.earthFind, flags...)
	into := pzm.Value("into", verb.NoShort, "folder for the files")
	declared.Verb("earth-fetch", verb.NoShort, "fetches terrestrial "+
		"products", earthFetchPage, "Example_earthFetch", fetching,
		tool.earthFetch, append(flags, into)...)
}

// earthSources prints the terrestrial catalog without using the web.
func (tool command) earthSources(call pzm.Call) error {
	for _, source := range earth.Sources() {
		inputs := source.Queries
		if inputs == "" {
			inputs = "catalog reference"
		}
		fmt.Fprintf(call.Out(), "%-22s %s (%s)\n    %s\n",
			source.Name, source.Title, inputs, source.Reference)
	}
	return nil
}

// earthFind prints the products from the named terrestrial source.
func (tool command) earthFind(call pzm.Call) error {
	get, err := tool.earthWeb()
	if err != nil {
		return err
	}
	products, err := askEarth(call, get)
	if err != nil {
		return err
	}
	for _, product := range products {
		say(call.Out(), product)
	}
	return nil
}

// earthFetch brings products into the named source's subfolder.
func (tool command) earthFetch(call pzm.Call) error {
	into := call.Value("into")
	if into == "" {
		return celestialdata.Refuse("earth-fetch needs --into")
	}
	get, err := tool.earthWeb()
	if err != nil {
		return err
	}
	products, err := askEarth(call, get)
	if err != nil {
		return err
	}
	folder := filepath.Join(into, call.Args()[0])
	return errors.Join(fetchEach(call, get, products, folder, "")...)
}

// earthWeb uses the regular web client without signing in to Gaia.
func (tool command) earthWeb() (web.Web, error) {
	if tool.get != nil {
		return tool.get, nil
	}
	return tool.client()
}

// askEarth reads one source and its query from the command line.
func askEarth(call pzm.Call,
	get web.Web) ([]*celestialdata.Product, error) {
	args := call.Args()
	if len(args) != 1 {
		return nil, celestialdata.Refuse(
			"name one source from earth-sources")
	}
	query := earth.Query{Text: call.Value("text")}
	if words := call.Value("bounds"); words != "" {
		query.Bounds = new([4]float64)
		if err := coordinates(words, query.Bounds[:]); err != nil {
			return nil, err
		}
	}
	if words := call.Value("point"); words != "" {
		query.Point = new([2]float64)
		if err := coordinates(words, query.Point[:]); err != nil {
			return nil, err
		}
	}
	if words := call.Value("most"); words != "" {
		most, err := strconv.Atoi(words)
		if err != nil || most < 1 {
			return nil, celestialdata.Refuse(
				"--most needs a positive integer")
		}
		query.Most = most
	}
	return earth.Find(args[0], get, query)
}

// coordinates reads a fixed number of comma-separated decimal degrees.
func coordinates(words string, values []float64) error {
	fields := strings.Split(words, ",")
	if len(fields) != len(values) {
		return celestialdata.Refuse(
			"expected %d comma-separated coordinates", len(values))
	}
	for at, field := range fields {
		number, err := strconv.ParseFloat(strings.TrimSpace(field), 64)
		if err != nil {
			return celestialdata.Refuse(
				"invalid coordinate %q", field)
		}
		values[at] = number
	}
	return nil
}
