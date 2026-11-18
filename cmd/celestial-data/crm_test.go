package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNOAACRMRefusesUnknownFlagsAndMissingProvenance(t *testing.T) {
	for _, args := range [][]string{{"noaa-crm", "--arbitrary-url", "https://example.org"}, {"noaa-crm"}, {"tiff"}} {
		var out, errs bytes.Buffer
		if run(args, strings.NewReader(""), &out, &errs) == 0 {
			t.Fatal("accepted missing/unknown arguments")
		}
	}
}

func TestNOAACRMDryRunValidatesRealRetainedMetadataWithoutWriting(t *testing.T) {
	tmp := t.TempDir()
	box, command, into := filepath.Join(tmp, "box.json"), filepath.Join(tmp, "command"), filepath.Join(tmp, "absent")
	os.WriteFile(box, []byte(`{"West":-156.2,"South":18.8,"East":-154.7,"North":20.4}`), 0600)
	os.WriteFile(command, []byte("fixture dry run\n"), 0600)
	args := []string{"noaa-crm", "--dry-run", "--box", box, "--capabilities", "../../noaacrm/testdata/noaa-capabilities.xml", "--description", "../../noaacrm/testdata/noaa-description.xml", "--name", "fixture.tif", "--into", into, "--adapter-commit", strings.Repeat("a", 40), "--command-file", command, "--purpose", "test", "--consumer", "test"}
	var out, errs bytes.Buffer
	if code := run(args, strings.NewReader(""), &out, &errs); code != 0 {
		t.Fatalf("code=%d: %s", code, errs.String())
	}
	if !strings.Contains(out.String(), "format=GeoTIFF_Float") || !strings.Contains(out.String(), "request=GetCoverage") {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(into); !os.IsNotExist(err) {
		t.Fatal("dry run wrote output")
	}
	os.WriteFile(box, []byte(`{"West":-156.2,"South":18.8,"East":-154.7,"North":20.4,"Unknown":true}`), 0600)
	if run(args, strings.NewReader(""), &out, &errs) == 0 {
		t.Fatal("accepted unknown configuration field")
	}
}

// The Big Island, asked for and not fetched: the box fetched on
// 2026-11-09 and NOAA's two model descriptions from noaacrm's testdata.
// A dry run names the request and writes nothing.
func Example_crm() {
	dir, err := os.MkdirTemp("", "noaa-crm")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer os.RemoveAll(dir)
	box, command := filepath.Join(dir, "box.json"), filepath.Join(dir, "command")
	os.WriteFile(box, []byte(`{"West":-156.2,"South":18.8,"East":-154.7,"North":20.4}`), 0600)
	os.WriteFile(command, []byte("celestial-data noaa-crm --dry-run\n"), 0600)
	var out, errs bytes.Buffer
	code := run([]string{"noaa-crm", "--dry-run", "--box", box,
		"--capabilities", "../../noaacrm/testdata/noaa-capabilities.xml",
		"--description", "../../noaacrm/testdata/noaa-description.xml",
		"--name", "big-island.tif", "--into", filepath.Join(dir, "terrain"),
		"--adapter-commit", strings.Repeat("a", 40), "--command-file", command,
		"--purpose", "the island a scene stands on", "--consumer", "a scene"},
		strings.NewReader(""), &out, &errs)
	fmt.Println(code, strings.Contains(out.String(), "bbox=-156.2%2C18.8%2C-154.7%2C20.4"),
		strings.Contains(out.String(), "format=GeoTIFF_Float"))
	_, written := os.Stat(filepath.Join(dir, "terrain"))
	fmt.Println("wrote nothing:", os.IsNotExist(written))
	// Output:
	// 0 true true
	// wrote nothing: true
}

func Example_tiff() {
	var out, errs bytes.Buffer
	code := run([]string{"tiff"}, strings.NewReader(""), &out, &errs)
	fmt.Printf("missing local file refused: %t\n", code != 0)
	// Output: missing local file refused: true
}
