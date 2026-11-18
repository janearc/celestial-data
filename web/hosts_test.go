package web_test

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
)

// hostsAt is the gentles of many hosts, with the example host's own
// policy under its address's name, on a clock at the start.
func hostsAt(t *testing.T, folder string) *web.Hosts {
	t.Helper()
	clock, _ := clockAt(t, start)
	return web.NewHosts(folder, clock, randomOf(t, half),
		map[string]web.Policy{"example.org": example(t)})
}

// heldHostsAt keeps each host's record for the run.
func heldHostsAt(t *testing.T, folder string) *web.Hosts {
	t.Helper()
	clock, _ := clockAt(t, start)
	policy := example(t)
	policy.Keeping = web.KeepsForTheRun
	return web.NewHosts(folder, clock, randomOf(t, half),
		map[string]web.Policy{"example.org": policy})
}

// forAddress is the gentle of an address's host.
func forAddress(t *testing.T, hosts *web.Hosts, where string) *web.Gentle {
	t.Helper()
	gentle, err := hosts.For(where)
	if err != nil {
		t.Fatal(err)
	}
	return gentle
}

// One host has one gentle, and another host another, and a host with a
// policy of its own is asked by it.
func TestEachHostHasOneGentleOfItsOwn(t *testing.T) {
	hosts := hostsAt(t, "")
	one := forAddress(t, hosts, "https://example.org/a")
	again := forAddress(t, hosts, "https://example.org:8080/b")
	other := forAddress(t, hosts, "https://archive.example.net/a")
	if one != again || one == other {
		t.Errorf("%p %p %p", one, again, other)
	}
	if got := refuse(t, one, http.StatusTooManyRequests, "30"); got.Defaulted {
		t.Errorf("a host with a policy: %v", got)
	}
}

// A host with no policy of its own is asked by the default, and its stops
// say the pace was the default.
func TestAHostWithoutAPolicyIsAskedByTheDefault(t *testing.T) {
	other := forAddress(t, hostsAt(t, ""), "https://archive.example.net/a")
	got := refuse(t, other, http.StatusTooManyRequests, "30")
	if !got.Defaulted || got.Host != "archive.example.net" ||
		!strings.Contains(got.Error(), "default pace") {
		t.Errorf("%v", got)
	}
}

// Anti-test: one host's stop is not another's, and its circuit does not
// open the other's.
func TestOneHostsStopIsNotAnothers(t *testing.T) {
	hosts := hostsAt(t, "")
	one := forAddress(t, hosts, "https://example.org/a")
	other := forAddress(t, hosts, "https://archive.example.net/a")
	refuse(t, one, http.StatusTooManyRequests, "3000")
	one.GiveUp()
	one.GiveUp()
	askOnce(t, other)
	askOnce(t, other)
}

// Anti-test: an address that is no address, or names no host, and a policy
// that cannot be asked by, and defaults that will not read, are errors.
func TestAnAddressThatNamesNoHostIsRefused(t *testing.T) {
	hosts := hostsAt(t, "")
	for _, where := range []string{"://bad", "/just/a/path"} {
		if _, err := hosts.For(where); err == nil {
			t.Errorf("%q gave a gentle", where)
		}
	}
	clock, _ := clockAt(t, start)
	broken := web.NewHosts("", clock, randomOf(t, half),
		map[string]web.Policy{"example.org": {}})
	if _, err := broken.For("https://example.org/"); err == nil {
		t.Error("a policy of nothing")
	}
	defer web.SwapDefaults([]byte("numbers: ["))()
	if _, err := hosts.For("https://archive.example.net/"); err == nil {
		t.Error("defaults that will not read")
	}
}

// Anti-test: a web within the hosts refuses an address that names none,
// on each of its three requests.
func TestAWebWithinHostsRefusesAnAddressThatNamesNoHost(t *testing.T) {
	inner := mocks.NewMockWeb(gomock.NewController(t))
	wrapped, err := web.Within(t.Context(), inner, hostsAt(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := wrapped.Get("/path"); err == nil {
		t.Error("Get")
	}
	if _, err := wrapped.Head("/path"); err == nil {
		t.Error("Head")
	}
	if _, err := wrapped.Post("/path", nil); err == nil {
		t.Error("Post")
	}
}

// Each host's record is kept in the folder, by the host's name, and
// released together.
func TestEachHostsRecordIsKeptInTheFolderAndReleasedTogether(t *testing.T) {
	folder := t.TempDir()
	hosts := hostsAt(t, folder)
	askOnce(t, forAddress(t, hosts, "https://example.org/a"))
	askOnce(t, forAddress(t, hosts, "https://archive.example.net/a"))
	if err := hosts.Release(); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"example.org", "archive.example.net"} {
		path := web.RecordPath(folder, host)
		if path != filepath.Join(folder, host+"-gate") ||
			!strings.HasPrefix(recordOf(t, path), "last ") {
			t.Errorf("%s: %q", host, path)
		}
	}
}

// Anti-test: with no folder, none is kept; a record held by another run is
// refused; and a release that cannot write says so.
func TestNoFolderKeepsNoRecordAndAHeldRecordIsRefused(t *testing.T) {
	bare := hostsAt(t, "")
	askOnce(t, forAddress(t, bare, "https://example.org/a"))
	if err := bare.Release(); err != nil {
		t.Error(err)
	}
	folder := t.TempDir()
	first := heldHostsAt(t, folder)
	forAddress(t, first, "https://example.org/a")
	if _, err := heldHostsAt(t, folder).For("https://example.org/a"); err == nil {
		t.Error("a record held by another run")
	}
	if err := os.Chmod(folder, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(folder, 0o755)
	if err := first.Release(); err == nil {
		t.Error("a release that could not write")
	}
}

// Insisting has every host's gentle insist, those made before and those
// made after.
func TestInsistingReachesHostsMadeBeforeAndAfter(t *testing.T) {
	hosts := hostsAt(t, "")
	before := forAddress(t, hosts, "https://example.org/a")
	refuse(t, before, http.StatusTooManyRequests, "3000")
	var said bytes.Buffer
	hosts.Insist(&said)
	after := forAddress(t, hosts, "https://archive.example.net/a")
	refuse(t, after, http.StatusTooManyRequests, "3000")
	askOnce(t, before)
	askOnce(t, after)
	if strings.Count(said.String(), "asking anyway") != 2 {
		t.Errorf("%q", said.String())
	}
}

// A host's name is lower case with no dot at its end, so the ways an
// address may spell it are one host, with one policy and one record.
func TestAHostIsOneWhateverTheCaseOrTheTrailingDot(t *testing.T) {
	folder := t.TempDir()
	hosts := hostsAt(t, folder)
	plain := forAddress(t, hosts, "https://example.org/a")
	for _, spelled := range []string{"https://Example.ORG/a",
		"https://example.org./a", "https://EXAMPLE.org.:8080/a"} {
		if got := forAddress(t, hosts, spelled); got != plain {
			t.Errorf("%s is another host", spelled)
		}
	}
	got := refuse(t, plain, http.StatusTooManyRequests, "30")
	if got.Defaulted {
		t.Errorf("the host's own policy was not used: %v", got)
	}
	if err := hosts.Release(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.ToLower(entry.Name()) != entry.Name() ||
			strings.Contains(entry.Name(), ".-") {
			t.Errorf("a record named %q", entry.Name())
		}
	}
}

// Anti-test: two hosts that differ in more than case or a dot are two.
func TestHostsThatDifferInMoreThanCaseAreTwo(t *testing.T) {
	hosts := hostsAt(t, "")
	one := forAddress(t, hosts, "https://example.org/a")
	other := forAddress(t, hosts, "https://www.example.org/a")
	if one == other {
		t.Error("www.example.org is example.org")
	}
}
