package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"
)

// The retention verbs offer the same recovery their stop messages name:
// an override asks once more, says so, and stops after a second refusal.
func TestRetentionVerbsHonorTheHostOverride(t *testing.T) {
	for _, verb := range []string{"noaa-crm", "documents"} {
		t.Run(verb, func(t *testing.T) {
			tool := askingHosts(t)
			tool.get.EXPECT().Get(gomock.Any()).Return(nil, int64(0),
				refusal(t, http.StatusTooManyRequests)).Times(2)
			args, _ := retentionArgs(t, verb)
			args = append(args, "--i-mean-it")
			out, errs, code := ran(tool.command, args...)
			if code != failed || !strings.Contains(out+errs, "the override was tried") ||
				!strings.Contains(errs, "asking once more") {
				t.Fatalf("exit %d, output %q, errors %q", code, out, errs)
			}
		})
	}
}

// With no override a refusal sends no second request. A dry run with the
// override still sends none and makes no destination folder.
func TestRetentionVerbsDoNotAskAgainWithoutTheOverride(t *testing.T) {
	for _, verb := range []string{"noaa-crm", "documents"} {
		t.Run(verb, func(t *testing.T) {
			tool := askingHosts(t)
			tool.get.EXPECT().Get(gomock.Any()).Return(nil, int64(0),
				refusal(t, http.StatusTooManyRequests))
			args, _ := retentionArgs(t, verb)
			out, errs, code := ran(tool.command, args...)
			if code != failed || !strings.Contains(out+errs,
				"to ask anyway, add --i-mean-it") {
				t.Fatalf("exit %d, output %q, errors %q", code, out, errs)
			}
			quiet := askingHosts(t)
			args, into := retentionArgs(t, verb)
			args = append(args, "--dry-run", "--i-mean-it")
			_, errs, code = ran(quiet.command, args...)
			if code != exitOK {
				t.Fatalf("dry run exit %d, errors %q", code, errs)
			}
			if _, err := os.Stat(into); !os.IsNotExist(err) {
				t.Fatalf("dry run made its output folder: %v", err)
			}
		})
	}
}

// retentionArgs is a valid request over local inputs and the existing
// NOAA metadata fixtures. Its destination starts absent.
func retentionArgs(t *testing.T, verb string) ([]string, string) {
	t.Helper()
	folder := t.TempDir()
	into := filepath.Join(folder, "absent")
	args := []string{verb, "--into", into, "--adapter-commit",
		strings.Repeat("a", 40), "--purpose", "test", "--consumer", "test"}
	if verb == "documents" {
		catalog := filepath.Join(folder, "catalog.json")
		contents := `[{"ID":"guide","Name":"guide.html",` +
			`"URL":"https://example.org/guide.html",` +
			`"Title":"Guide","Kind":"html"}]`
		if err := os.WriteFile(catalog, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		return append(args, "--catalog", catalog, "--command", "test", "guide"), into
	}
	box := filepath.Join(folder, "box.json")
	command := filepath.Join(folder, "command")
	region := `{"West":-156.2,"South":18.8,"East":-154.7,"North":20.4}`
	if err := os.WriteFile(box, []byte(region), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(command, []byte("test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return append(args, "--box", box, "--command-file", command,
		"--capabilities", "../../noaacrm/testdata/noaa-capabilities.xml",
		"--description", "../../noaacrm/testdata/noaa-description.xml",
		"--name", "fixture.tif"), into
}
