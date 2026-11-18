package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	celestialdata "github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/tnm"
	"github.com/janearc/puzzlemouth/pzm"
)

// terrainFetch downloads only products selected from a retained TNM catalogue.
// Each request is recorded before bytes are requested, and completed transfers
// get a size and hash. The ordinary fetch path refuses existing destinations.
func (tool command) terrainFetch(call pzm.Call) error {
	if len(call.Args()) != 0 || call.Value("into") == "" ||
		call.Value("index") == "" {
		return fmt.Errorf("terrain-fetch needs --index, --into and " +
			"no arguments")
	}
	products, err := tnm.Products(call.Value("index"))
	if err != nil {
		return err
	}
	if call.DryRun() {
		for _, p := range products {
			fmt.Fprintln(call.Out(), p.Name(), p.URL())
		}
		return nil
	}
	get := tool.get
	if get == nil {
		get, err = tool.client()
		if err != nil {
			return err
		}
	}
	get = tool.gentle(get)
	if err = os.MkdirAll(call.Value("into"), 0755); err != nil {
		return err
	}
	record, err := os.OpenFile(
		filepath.Join(call.Value("into"), "FETCHED.md"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer record.Close()
	for _, p := range products {
		if _, err = fmt.Fprintf(record,
			"\n## Request %s\n\nAdapter tnm build "+
				"%s; source ID %s.\n\nURL: "+
				"%s\n\nDestination: %s\n\nStatus: "+
				"pending.\n",
			time.Now().UTC().Format(time.RFC3339), build, p.ID(),
			p.URL(), p.Name()); err != nil {
			return err
		}
		if err = record.Sync(); err != nil {
			return err
		}
		served, headErr := get.Head(p.URL())
		if headErr != nil {
			fmt.Fprintln(record, "HEAD failed:", headErr)
			return headErr
		}
		fmt.Fprintf(record, "\nHEAD observed %d bytes. GET must "+
			"match this representation.\n", served)
		if err = record.Sync(); err != nil {
			return err
		}
		observed, err := tnm.ServedProduct(p, served)
		if err != nil {
			return err
		}
		existing := filepath.Join(call.Value("into"), p.Name())
		if info, statErr := os.Stat(existing); statErr == nil {
			digest, size, hashErr := terrainDigest(existing)
			prior, readErr := os.ReadFile(
				filepath.Join(call.Value("into"), "FETCHED.md"))
			if hashErr != nil || readErr != nil ||
				info.Size() != served || size != served ||
				!strings.Contains(string(prior),
					"SHA256 "+digest+".") {
				return fmt.Errorf(
					"terrain-fetch: existing file has no "+
						"matching transfer "+
						"record: %s", existing)
			}
			fmt.Fprintln(record,
				"\nStatus: kept prior verified transfer; "+
					"SHA256 "+digest+".")
			fmt.Fprintln(call.Out(), "kept", existing)
			continue
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
		name, fetchErr := celestialdata.Fetch(get, observed,
			call.Value("into"))
		if fetchErr != nil {
			fmt.Fprintln(record, "\nStatus: failed:", fetchErr)
			return fetchErr
		}
		digest, size, err := terrainDigest(name)
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintf(record,
			"\nStatus: completed %s; %d bytes; "+
				"SHA256 %s.\n",
			time.Now().UTC().Format(time.RFC3339),
			size, digest); err != nil {
			return err
		}
		if err = record.Sync(); err != nil {
			return err
		}
		fmt.Fprintln(call.Out(), name)
	}
	return nil
}

// terrainDigest is a file's sha256 sum and byte count, read from disk.
func terrainDigest(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	sum := sha256.New()
	n, err := io.Copy(sum, f)
	return fmt.Sprintf("%x", sum.Sum(nil)), n, err
}
