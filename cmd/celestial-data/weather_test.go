package main

import "testing"

// weatherFile is the file the grissom set takes into weather.
const weatherFile = "ECMWF_ERA-40_subset.nc"

// The grissom set holds the one weather file, in the folder named
// weather, as the set's other files are declared.
func TestTheGrissomSetHoldsTheWeatherFile(t *testing.T) {
	get := serving(t, colourwayPages())
	made, err := grissomSet(through(t, get), get)
	if err != nil {
		t.Fatal(err)
	}
	var held []string
	for _, each := range made.parts {
		if each.into != "weather" {
			continue
		}
		held = append(held, namesOf(t, each, made.near)...)
	}
	if len(held) != 1 || held[0] != weatherFile {
		t.Errorf("weather holds %q, not %q", held, weatherFile)
	}
}

// Anti-test: no other folder of the set holds the weather file.
func TestNoOtherFolderHoldsTheWeatherFile(t *testing.T) {
	get := serving(t, colourwayPages())
	made, err := grissomSet(through(t, get), get)
	if err != nil {
		t.Fatal(err)
	}
	for _, each := range made.parts {
		if each.into == "weather" {
			continue
		}
		for _, name := range namesOf(t, each, made.near) {
			if name == weatherFile {
				t.Errorf("%s also holds %s", each.into, name)
			}
		}
	}
}
