package main

// The noaa-crm and tiff verbs retain a bounded piece of NOAA's Hawaii
// Coastal Relief Model 2023 with a record of why, and read a retained
// TIFF's first directory. They use celestial-data's web; noaacrm
// validates the piece and the TIFF it receives.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/janearc/celestial-data/noaacrm"
	"github.com/janearc/celestial-data/web"
	"github.com/janearc/puzzlemouth/pzm"
	"github.com/janearc/puzzlemouth/verb"
	"github.com/janearc/puzzlemouth/yamlfile"
)

// crmPage and tiffPage are what noaa-crm and tiff do, as their help
// says it.
const (
	crmPage = "Validates retained NOAA WCS metadata and a strict " +
		"authored bbox file before forming a native-grid GetCoverage " +
		"request. Flushes provenance before GET. Enforces a 150 MiB " +
		"response ceiling and bounded single-band float32 GeoTIFF " +
		"metadata/strip checks. Existing " +
		"products are refused. Dry run " +
		"performs no writes or requests."
	tiffPage = "Reports classic TIFF first-directory tags as JSON, " +
		"without raster decoding or datum conversion. No network " +
		"request or file write."
)

// How long a whole request may take, to its last byte, for a piece of
// the model and for one document: the original standalone tools' limits.
// The command's own web gives a whole request no limit, which suits an
// archive's large files and not a page that should be there in seconds.
const (
	crmWhole       = 20 * time.Minute
	documentsWhole = 180 * time.Second
)

// within is a verb's web with a whole-request deadline: the test's web,
// or the network's with the wait its numbers say. Both pass through the
// hosts' gentles, as the command's own web does.
func (tool command) within(whole time.Duration) (web.Web, error) {
	if tool.get != nil {
		return tool.gentle(tool.get), nil
	}
	wait, err := tool.picked.Get("wait")
	if err != nil {
		return nil, err
	}
	return tool.gentle(web.New(web.Seconds(wait), whole, agent)), nil
}

// crmFlags are the flags noaa-crm takes.
func crmFlags() []pzm.Flag {
	return []pzm.Flag{
		pzm.Value("box", verb.NoShort,
			"local authored West/South/East/North "+
				"YAML or JSON file"),
		pzm.Value("capabilities", verb.NoShort,
			"retained NOAA GetCapabilities XML"),
		pzm.Value("description", verb.NoShort,
			"retained NOAA DescribeCoverage XML"),
		pzm.Value("name", verb.NoShort, "plain output TIFF filename"),
		pzm.Value("into", verb.NoShort, "retained source folder"),
		pzm.Value("adapter-commit", verb.NoShort, "full source commit"),
		pzm.Value("command-file", verb.NoShort,
			"exact invoking shell command file"),
		pzm.Value("purpose", verb.NoShort,
			"why this subset is fetched"),
		pzm.Value("consumer", verb.NoShort,
			"code or findings using this subset"),
	}
}

// crmVerb brings one bounded piece of the model home.
func (tool command) crmVerb(call pzm.Call) error {
	for _, key := range []string{"box", "capabilities", "description",
		"name", "into", "adapter-commit", "command-file", "purpose",
		"consumer"} {
		if call.Value(key) == "" {
			return fmt.Errorf("%s is required", key)
		}
	}
	if len(call.Args()) != 0 {
		return fmt.Errorf("fetch accepts no positional arguments")
	}
	commit := call.Value("adapter-commit")
	if decoded, err := hex.DecodeString(commit); err != nil ||
		len(decoded) != 20 {
		return fmt.Errorf("adapter-commit must be a full git SHA-1")
	}
	contents := map[string][]byte{}
	for _, key := range []string{"box", "capabilities", "description",
		"command-file"} {
		b, err := os.ReadFile(call.Value(key))
		if err != nil {
			return err
		}
		contents[key] = b
	}
	command := strings.TrimSpace(string(contents["command-file"]))
	if command == "" {
		return fmt.Errorf("command file is empty")
	}
	var box noaacrm.Box
	if err := yamlfile.Read(call.Value("box"), contents["box"],
		&box); err != nil {
		return err
	}
	m, err := noaacrm.ReadMetadata(contents["capabilities"],
		contents["description"])
	if err != nil {
		return err
	}
	name, into := call.Value("name"), call.Value("into")
	if filepath.Ext(name) != ".tif" {
		return fmt.Errorf("name must end in .tif")
	}
	product, err := box.Product(m, name)
	if err != nil {
		return err
	}
	if call.DryRun() {
		fmt.Fprintf(call.Out(), "would fetch %s into %s\n",
			product.URL(), filepath.Join(into, name))
		return nil
	}
	if err := os.MkdirAll(into, 0755); err != nil {
		return err
	}
	intent := fmt.Sprintf("\n## NOAA Hawaii CRM2023 native-grid "+
		"subset\n\n- Started UTC: %s\n- Adapter: "+
		"celestial-data/noaacrm at `%s`\n- Exact "+
		"command:\n\n```sh\n%s\n```\n\n- Archive "+
		"query: `%s`\n- Request: coverage z; "+
		"CRS84 bbox west,south,east,north; "+
		"GeoTIFF_Float; native grid (no "+
		"width/height/resampling arguments)\n- "+
		"Transfer ceiling: %d bytes (150 MiB); "+
		"estimated native dimensions checked "+
		"before GET\n- Purpose: %s\n- Consumer: "+
		"%s\n- Retained path: `%s`\n- Status: "+
		"fetch started; completion follows\n",
		time.Now().UTC().Format(time.RFC3339), commit, command,
		product.URL(), noaacrm.Ceiling, call.Value("purpose"),
		call.Value("consumer"), filepath.Join(into, name))
	for _, key := range []string{"box", "capabilities", "description",
		"command-file"} {
		intent += fmt.Sprintf("- Input %s: `%s`, %d bytes, SHA-256 "+
			"`%x`\n", key, call.Value(key), len(contents[key]),
			sha256.Sum256(contents[key]))
	}
	if err := appendLog(into, intent); err != nil {
		return err
	}
	client, err := tool.within(crmWhole)
	if err != nil {
		return err
	}
	path, err := noaacrm.Fetch(client, box, m, name, into)
	if err != nil {
		logErr := appendLog(into, fmt.Sprintf(
			"\n### Completion: %s\n\n- Completed "+
				"UTC: %s\n- Status: FAILED — %s\n", name,
			time.Now().UTC().Format(time.RFC3339), err))
		if logErr != nil {
			return fmt.Errorf("fetch: %v; provenance: %w",
				err, logErr)
		}
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	hash := sha256.New()
	n, err := io.Copy(hash, f)
	f.Close()
	if err != nil {
		return err
	}
	if err := appendLog(into, fmt.Sprintf(
		"\n### Completion: %s\n\n- Completed "+
			"UTC: %s\n- Retained path: `%s`\n- "+
			"Status: SUCCESS; bounded float32 TIFF "+
			"metadata/strip checks passed; "+
			"metadata/CRS interpretation remains a "+
			"separate read\n- Bytes: %d\n- SHA-256: "+
			"`%x`\n", name, time.Now().UTC().Format(time.RFC3339),
		path, n, hash.Sum(nil))); err != nil {
		return err
	}
	fmt.Fprintf(call.Out(), "retained %s (%d bytes, %x)\n",
		path, n, hash.Sum(nil))
	return nil
}

// tiffVerb reads a TIFF's first directory back.
func (tool command) tiffVerb(call pzm.Call) error {
	if call.Value("file") == "" || len(call.Args()) != 0 {
		return fmt.Errorf("tiff requires --file and no positional " +
			"arguments")
	}
	info, err := noaacrm.Inspect(call.Value("file"))
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(call.Out())
	encoder.SetIndent("", "  ")
	return encoder.Encode(info)
}

// appendLog adds to the record kept beside what is fetched, FETCHED.md
// in the folder it goes into, and has it on the disk before it returns:
// noaa-crm and documents both write what they are about to ask for
// before they ask.
func appendLog(into, text string) error {
	f, err := os.OpenFile(filepath.Join(into, "FETCHED.md"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	_, writeErr := f.WriteString(strings.ReplaceAll(text, "\r", ""))
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
