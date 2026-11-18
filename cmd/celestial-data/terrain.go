package main

import (
	"fmt"

	celestialdata "github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/tnm"
	"github.com/janearc/puzzlemouth/pzm"
)

// terrainIndex retains the archive's query response before terrain is chosen.
func (tool command) terrainIndex(call pzm.Call) error {
	if len(call.Args()) != 0 || call.Value("into") == "" {
		return fmt.Errorf("terrain-index needs --into and no arguments")
	}
	product, err := tnm.HawaiiIndex()
	if err != nil {
		return err
	}
	if call.DryRun() {
		fmt.Fprintln(call.Out(),
			"USGS Hawaii elevation index: no download")
		return nil
	}
	// National Map is public; it needs no astronomical archive session.
	get := tool.get
	if get == nil {
		get, err = tool.client()
		if err != nil {
			return err
		}
	}
	get = tool.gentle(get)
	path, err := celestialdata.Fetch(get, product, call.Value("into"))
	if err == nil {
		fmt.Fprintln(call.Out(), path)
		err = tnm.CheckIndex(path)
	}
	return err
}
