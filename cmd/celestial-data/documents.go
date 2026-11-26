package main

// The documents verb brings home named primary documents from a local
// catalogue, with a record of why. It uses celestial-data's web;
// primarydocs validates the documents it may fetch.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/janearc/celestial-data/caltech"
	"github.com/janearc/celestial-data/primarydocs"
	"github.com/janearc/celestial-data/web"
	"github.com/janearc/puzzlemouth/pzm"
	"github.com/janearc/puzzlemouth/verb"
	"github.com/janearc/puzzlemouth/yamlfile"
)

// documentsPage is what documents does, as its help says it.
const documentsPage = "Fetches only explicitly selected public HTTPS " +
	"documents from a local YAML or JSON catalogue. Records intent " +
	"before each GET and hash and size after it. Existing files are " +
	"refused. A host with an adapter of its own, arXiv or ADS, is " +
	"refused: ask it through papers or ads. A dry run validates and " +
	"names selected documents without requests or writes."

// documentsFlags are the flags documents takes.
func documentsFlags() []pzm.Flag {
	return []pzm.Flag{
		pzm.Value("catalog", verb.NoShort,
			"local YAML or JSON catalogue"),
		pzm.Value("into", verb.NoShort, "retained source directory"),
		pzm.Value("adapter-commit", verb.NoShort,
			"full adapter source commit"),
		pzm.Value("command", verb.NoShort,
			"exact invoking shell command"),
		pzm.Value("command-file", verb.NoShort,
			"file containing exact invoking shell "+
				"command"),
		pzm.Value("purpose", verb.NoShort, "why documents are fetched"),
		pzm.Value("consumer", verb.NoShort,
			"code or findings using documents"),
	}
}

// documentsVerb brings the catalogue's named documents home.
func (tool command) documentsVerb(call pzm.Call) error {
	return tool.documents(call, "primarydocs", func(doc primarydocs.Document) error {
		_, err := doc.Product()
		return err
	}, primarydocs.Fetch)
}

// caltechVerb retains a catalogue's Caltech documents through the same gate.
func (tool command) caltechVerb(call pzm.Call) error {
	return tool.documents(call, "caltech", caltech.Check, caltech.Fetch)
}

// documents validates the catalogue and records each selected request.
func (tool command) documents(call pzm.Call, adapter string,
	check func(primarydocs.Document) error,
	fetch func(web.Web, primarydocs.Document, string) (string, error),
) error {
	catalog, into := call.Value("catalog"), call.Value("into")
	commit, command := call.Value("adapter-commit"), call.Value("command")
	commandFile := call.Value("command-file")
	purpose, consumer := call.Value("purpose"), call.Value("consumer")
	if commandFile != "" {
		if command != "" {
			return fmt.Errorf("use command or command-file, " +
				"not both")
		}
		b, err := os.ReadFile(commandFile)
		if err != nil {
			return err
		}
		command = strings.TrimSpace(string(b))
	}
	if catalog == "" || into == "" || command == "" || purpose == "" ||
		consumer == "" || len(call.Args()) == 0 {
		return fmt.Errorf("catalog, into, command, purpose, " +
			"consumer and document IDs are required")
	}
	decoded, err := hex.DecodeString(commit)
	if err != nil || len(decoded) != 20 {
		return fmt.Errorf("adapter-commit must be a full git SHA-1")
	}
	b, err := os.ReadFile(catalog)
	if err != nil {
		return err
	}
	var docs []primarydocs.Document
	if err := yamlfile.Read(catalog, b, &docs); err != nil {
		return err
	}
	byID := map[string]primarydocs.Document{}
	for _, d := range docs {
		if _, exists := byID[d.ID]; exists {
			return fmt.Errorf("duplicate catalogue ID %q", d.ID)
		}
		if err := check(d); err != nil {
			return err
		}
		byID[d.ID] = d
	}
	var chosen []primarydocs.Document
	for _, id := range call.Args() {
		d, ok := byID[id]
		if !ok {
			return fmt.Errorf("unknown document %q", id)
		}
		chosen = append(chosen, d)
	}
	if call.DryRun() {
		for _, d := range chosen {
			fmt.Fprintf(call.Out(), "would fetch %s into %s\n",
				d.ID, filepath.Join(into, d.Name))
		}
		return nil
	}
	if err := os.MkdirAll(into, 0755); err != nil {
		return err
	}
	client, err := tool.documentWeb(adapter)
	if err != nil {
		return err
	}
	failed := false
	for _, d := range chosen {
		intent := fmt.Sprintf(
			"\n## %s — %s\n\n- Started UTC: %s\n- "+
				"Exact command: `%s`\n- Adapter: "+
				"celestial-data/%s at `%s`\n- "+
				"Catalogue: `%s`\n- Archive query: "+
				"direct document GET `%s` (no search "+
				"query or archive job)\n- Purpose: %s\n- "+
				"Consumer: %s\n- Retained path: `%s`\n- "+
				"Status: fetch started; completion "+
				"follows\n", d.ID, d.Title,
			time.Now().UTC().Format(time.RFC3339), command, adapter, commit,
			catalog, d.URL, purpose, consumer,
			filepath.Join(into, d.Name))
		if err := appendLog(into, intent); err != nil {
			return err
		}
		path, err := fetch(client, d, into)
		if err != nil {
			failed = true
			if logErr := appendLog(into, fmt.Sprintf(
				"\n### Completion for %s\n\n- URL: "+
					"`%s`\n- Intended path: `%s`\n- "+
					"Completed UTC: %s\n- Status: "+
					"FAILED — "+
					"%s\n", d.ID, d.URL,
				filepath.Join(into, d.Name),
				time.Now().UTC().Format(time.RFC3339),
				err)); logErr != nil {
				return logErr
			}
			fmt.Fprintf(call.Out(), "%s: %v\n", d.ID, err)
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		if err := appendLog(into, fmt.Sprintf(
			"\n### Completion for %s\n\n- URL: "+
				"`%s`\n- Retained path: `%s`\n- "+
				"Completed UTC: %s\n- Status: SUCCESS\n- "+
				"Bytes: %d\n- SHA-256: `%x`\n",
			d.ID, d.URL, path,
			time.Now().UTC().Format(time.RFC3339),
			len(b), sum)); err != nil {
			return err
		}
		fmt.Fprintf(call.Out(), "retained %s (%d bytes, %x)\n",
			d.ID, len(b), sum)
	}
	if failed {
		return fmt.Errorf("one or more document fetches failed; " +
			"see FETCHED.md")
	}
	return nil
}

// documentWeb refuses automatic redirects for Caltech, so a redirected
// request cannot bypass the per-host gate or the catalogue's host check.
func (tool command) documentWeb(adapter string) (web.Web, error) {
	if adapter != "caltech" || tool.get != nil {
		return tool.within(documentsWhole)
	}
	wait, err := tool.picked.Get("wait")
	if err != nil {
		return nil, err
	}
	return tool.gentle(web.Direct(web.Seconds(wait), documentsWhole, agent)), nil
}
