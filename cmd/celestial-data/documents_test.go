package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentsRefusesUnknownFlagsAndIDs(t *testing.T) {
	for _, args := range [][]string{{"documents", "--arbitrary-url", "https://example.edu"}, {"documents"}} {
		var out, errs bytes.Buffer
		if run(args, strings.NewReader(""), &out, &errs) == 0 {
			t.Fatal("accepted missing or unknown options")
		}
	}
}

func TestDocumentsDryRunReadsStrictCatalogueWithoutWriting(t *testing.T) {
	tmp := t.TempDir()
	catalog := filepath.Join(tmp, "papers.json")
	into := filepath.Join(tmp, "absent")
	good := `[{"ID":"paper","Name":"paper.pdf","URL":"https://example.edu/paper.pdf","Title":"Paper","Kind":"pdf"}]`
	os.WriteFile(catalog, []byte(good), 0600)
	args := []string{"documents", "--dry-run", "--catalog", catalog, "--into", into, "--adapter-commit", strings.Repeat("a", 40), "--command", "fixture dry run", "--purpose", "test", "--consumer", "test", "paper"}
	var out, errs bytes.Buffer
	if code := run(args, strings.NewReader(""), &out, &errs); code != 0 {
		t.Fatalf("code %d: %s", code, errs.String())
	}
	if !strings.Contains(out.String(), "would fetch paper") {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(into); !os.IsNotExist(err) {
		t.Fatal("dry run wrote output directory")
	}
	os.WriteFile(catalog, []byte(strings.Replace(good, `"Kind":"pdf"`, `"Kind":"pdf","Unknown":true`, 1)), 0600)
	out.Reset()
	errs.Reset()
	if code := run(args, strings.NewReader(""), &out, &errs); code == 0 {
		t.Fatal("accepted unknown configuration field")
	}
}

// One document of a catalogue, named and not fetched: a dry run says
// what would be brought home and writes nothing. And a catalogue that
// lists a paper at arXiv is refused: arXiv is asked through papers.
func Example_documentsDryRun() {
	dir, err := os.MkdirTemp("", "documents")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer os.RemoveAll(dir)
	catalog := filepath.Join(dir, "catalogue.json")
	asking := func(address string) (int, string) {
		os.WriteFile(catalog, []byte(`[{"ID":"guide","Name":"guide.html","URL":"`+address+
			`","Title":"a server's guide","Kind":"html"}]`), 0600)
		var out, errs bytes.Buffer
		code := run([]string{"documents", "--dry-run", "--catalog", catalog,
			"--into", filepath.Join(dir, "kept"), "--adapter-commit", strings.Repeat("a", 40),
			"--command", "celestial-data documents --dry-run", "--purpose", "what a claim rests on",
			"--consumer", "a README", "guide"}, strings.NewReader(""), &out, &errs)
		return code, out.String() + errs.String()
	}
	code, said := asking("https://docs.unidata.ucar.edu/tds/current/userguide/wcs_ref.html")
	fmt.Println(code, strings.HasPrefix(said, "would fetch guide into "))
	code, said = asking("https://arxiv.org/abs/2408.14425")
	fmt.Println(code != 0, strings.Contains(said, "ask it through papers"))
	// Output:
	// 0 true
	// true true
}

func Example_documents() {
	var out, errs bytes.Buffer
	code := run([]string{"documents", "--dry-run"}, strings.NewReader(""), &out, &errs)
	fmt.Printf("missing provenance refused: %t\n", code != 0)
	// Output: missing provenance refused: true
}
