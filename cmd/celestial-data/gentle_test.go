package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
)

// elsewhere is a page on a host that is not arXiv's, for a test of the
// gentles of the hosts a run asks.
const elsewhere = "https://example.org/data"

// hosted is celestial-data through a mock web, with the gentles of its
// hosts keeping their records in a folder of the test's own, on a clock
// that never waits.
type hosted struct {
	command
	get    *mocks.MockWeb
	folder string
}

// askingHosts is celestial-data whose archives are asked through hosts.
func askingHosts(t *testing.T) hosted {
	t.Helper()
	control := gomock.NewController(t)
	folder := t.TempDir()
	random := mocks.NewMockRandom(control)
	random.EXPECT().Float64().Return(0.5).AnyTimes()
	get := mocks.NewMockWeb(control)
	hosts := web.NewHosts(folder, hourly(control), random, nil)
	return hosted{command: command{get: get, hosts: hosts, errs: io.Discard,
		picked: picked(t, numbersRaw)}, get: get, folder: folder}
}

// helpOf is what a verb's help says.
func helpOf(t *testing.T, verb string) string {
	t.Helper()
	out, errs, _ := ran(command{picked: picked(t, numbersRaw)}, verb,
		"--help")
	return out + errs
}

// The verbs that fetch from a server through the web take --i-mean-it,
// and its help says what it does.
func TestTheVerbsThatFetchTakeTheSwitch(t *testing.T) {
	for _, verb := range []string{"find", "fetch", "fetch-set", "papers"} {
		help := helpOf(t, verb)
		if !strings.Contains(help, "--i-mean-it") ||
			!strings.Contains(help, "asks a host again before its stop "+
				"has passed; says what it asked first") {
			t.Errorf("%s: %q", verb, help)
		}
	}
}

// Anti-test: a verb that asks no server through the web does not take the
// switch, so it never promises an override it cannot make. ads is not
// here now that it asks through its own gentle; ads_gate_test.go tests
// that it takes the switch.
func TestTheVerbsThatAskNothingTakeNoSwitch(t *testing.T) {
	for _, verb := range []string{"archives"} {
		if help := helpOf(t, verb); strings.Contains(help, "i-mean-it") {
			t.Errorf("%s: %q", verb, help)
		}
	}
}

// A command with hosts asks its web through them: each host is paced,
// and its record is kept in the folder when the run is done.
func TestTheWebIsAskedThroughTheHostsAndTheirRecordsKept(t *testing.T) {
	tool := askingHosts(t)
	tool.get.EXPECT().Get(elsewhere).DoAndReturn(served)
	through, err := tool.web()
	if err != nil {
		t.Fatal(err)
	}
	body, _, err := through.Get(elsewhere)
	if err != nil {
		t.Fatal(err)
	}
	body.Close()
	if code := tool.finish(ranOK, io.Discard); code != ranOK {
		t.Errorf("exit %d", code)
	}
	record := web.RecordPath(tool.folder, "example.org")
	if kept, err := os.ReadFile(record); err != nil ||
		!strings.HasPrefix(string(kept), "last ") {
		t.Errorf("%q, %v", kept, err)
	}
}

// Anti-test: a command with no hosts asks its web as it is, and keeps no
// record.
func TestTheWebIsBareWithNoHosts(t *testing.T) {
	get := quiet(t)
	tool := command{get: get, picked: picked(t, numbersRaw)}
	through, err := tool.web()
	if err != nil || through != get {
		t.Errorf("%v, %v", through, err)
	}
	if err := tool.release(); err != nil {
		t.Error(err)
	}
	if code := tool.finish(ranOK, io.Discard); code != ranOK {
		t.Errorf("exit %d", code)
	}
}

// Anti-test: a web that cannot be made is an error, not a gentle one.
func TestTheWebIsAnErrorWhenItCannotBeMade(t *testing.T) {
	tool := askingHosts(t)
	tool.command.get = nil
	tool.picked = picked(t, []byte("numbers: []\n"))
	if through, err := tool.web(); err == nil || through != nil {
		t.Errorf("%v, %v", through, err)
	}
}

// A host that asks us to stop is said to have, and the error ends by
// naming the switch that asks again anyway.
func TestAStopNamesTheSwitch(t *testing.T) {
	tool := askingHosts(t)
	tool.get.EXPECT().Get(elsewhere).Return(nil, pdfLength,
		refusal(t, http.StatusTooManyRequests))
	through, _ := tool.web()
	_, _, err := through.Get(elsewhere)
	var stopped *web.Stopped
	if err == nil || !errors.As(err, &stopped) ||
		!strings.HasSuffix(err.Error(), "; to ask anyway, add "+
			"--i-mean-it") {
		t.Errorf("%v", err)
	}
}

// Anti-test: a stop asked again anyway, and refused again, does not name
// the switch, which was tried. An error that is no stop is given as it
// is, and so is no error.
func TestAStopAlreadyTriedDoesNotNameTheSwitch(t *testing.T) {
	tool := askingHosts(t)
	var said bytes.Buffer
	tool.hosts.Insist(&said)
	tool.get.EXPECT().Get(elsewhere).Return(nil, pdfLength,
		refusal(t, http.StatusTooManyRequests)).Times(2)
	through, _ := tool.web()
	_, _, err := through.Get(elsewhere)
	if err == nil || strings.Contains(err.Error(), "--i-mean-it") ||
		!strings.Contains(err.Error(), "the override was tried") ||
		!strings.Contains(said.String(), "asking once more") {
		t.Errorf("%v, %q", err, said.String())
	}
	plain := errors.New("down")
	if nameFlag(plain) != plain || nameFlag(nil) != nil {
		t.Error("an error that is no stop was changed")
	}
}

// A stop whose switch is set, but whose request may not be asked again,
// does not name the switch either: it is no way on.
func TestAStopWithTheSwitchSetDoesNotNameIt(t *testing.T) {
	tool := askingHosts(t)
	tool.hosts.Insist(io.Discard)
	tool.get.EXPECT().Post(elsewhere, gomock.Any()).Return("",
		refusal(t, http.StatusTooManyRequests))
	through, _ := tool.web()
	_, err := through.Post(elsewhere, nil)
	if err == nil || strings.Contains(err.Error(), "--i-mean-it") ||
		!strings.Contains(err.Error(), "may not be asked again") {
		t.Errorf("%v", err)
	}
}

// Head and Post name the switch as Get does.
func TestHeadAndPostNameTheSwitchToo(t *testing.T) {
	tool := askingHosts(t)
	stop := refusal(t, http.StatusTooManyRequests)
	tool.get.EXPECT().Head(elsewhere).Return(int64(none), stop)
	through, _ := tool.web()
	_, err := through.Head(elsewhere)
	if err == nil || !strings.HasSuffix(err.Error(), "--i-mean-it") {
		t.Errorf("head: %v", err)
	}
	other := askingHosts(t)
	other.get.EXPECT().Post(elsewhere, gomock.Any()).Return("", stop)
	through, _ = other.web()
	_, err = through.Post(elsewhere, nil)
	if err == nil || !strings.HasSuffix(err.Error(), "--i-mean-it") {
		t.Errorf("post: %v", err)
	}
}

// A verb given --i-mean-it has the hosts insist: a stop is asked again
// once, and what the host asked for is said on the tool's stderr.
func TestFindInsistsWhenGivenTheSwitch(t *testing.T) {
	tool := askingHosts(t)
	tool.get.EXPECT().Get(numaster(t)).Return(nil, pdfLength,
		refusal(t, http.StatusTooManyRequests)).Times(2)
	words := append([]string{"find", "--archive", "nustar",
		"--i-mean-it"}, sgrA...)
	_, errs, code := ran(tool.command, words...)
	if code != failed || !strings.Contains(errs, "the override was "+
		"tried") || strings.Contains(errs, "to ask anyway, add") ||
		!strings.Contains(errs, "asking once more") {
		t.Errorf("%d %q", code, errs)
	}
}

// Anti-test: without the switch a stop is asked once, and the error
// names the switch.
func TestFindDoesNotInsistWithoutTheSwitch(t *testing.T) {
	tool := askingHosts(t)
	tool.get.EXPECT().Get(numaster(t)).Return(nil, pdfLength,
		refusal(t, http.StatusTooManyRequests))
	words := append([]string{"find", "--archive", "nustar"}, sgrA...)
	_, errs, code := ran(tool.command, words...)
	if code != failed || !strings.Contains(errs, "to ask anyway, add "+
		"--i-mean-it") {
		t.Errorf("%d %q", code, errs)
	}
}

// papers given --i-mean-it asks arXiv again though the wait it asked for
// has not passed, saying first what arXiv asked for.
func TestPapersInsistsAfterAWaitKept(t *testing.T) {
	record := recordIn(t)
	first := arxivOver(t, record, numbersRaw)
	busy := reply(http.StatusTooManyRequests, "no-such-paper.xml", farOff)
	first.search.EXPECT().RawSearch(gomock.Any(), query("1809.03507")).
		Return(busy, nil)
	ran(first.command, "papers", "1809.03507", "--into", t.TempDir())
	next := arxivOver(t, record, numbersRaw)
	answer := reply(http.StatusOK, "two-papers.xml", "")
	next.search.EXPECT().RawSearch(gomock.Any(), query("1809.03507")).
		Return(answer, nil)
	// the pace from the first run's last request has not passed
	next.clock.(*mocks.MockClock).EXPECT().Sleep(gomock.Any(),
		gomock.Any()).Return(nil)
	out, errs, code := ran(next.command, "papers", "1809.03507",
		"--into", t.TempDir(), "--dry-run", "--i-mean-it")
	if code != exitOK || !strings.Contains(out, "would fetch") ||
		!strings.Contains(errs, "asking anyway") ||
		!strings.Contains(errs, "its Retry-After was "+farOff) {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

// Anti-test: without the switch the same run is stopped, and the error
// names it.
func TestPapersNamesTheSwitchAfterAWaitKept(t *testing.T) {
	record := recordIn(t)
	first := arxivOver(t, record, numbersRaw)
	busy := reply(http.StatusTooManyRequests, "no-such-paper.xml", farOff)
	first.search.EXPECT().RawSearch(gomock.Any(), query("1809.03507")).
		Return(busy, nil)
	_, errs, code := ran(first.command, "papers", "1809.03507", "--into",
		t.TempDir())
	if code == exitOK || !strings.HasSuffix(strings.TrimSpace(errs),
		"--i-mean-it") {
		t.Errorf("%d %q", code, errs)
	}
}

// A shared record lost mid-run is said once. Requests continue at the
// host's pace in memory, and the release preserves the verb's exit code.
func TestARecordThatWillNotWriteWarnsAndKeepsPacing(t *testing.T) {
	tool := askingHosts(t)
	control := gomock.NewController(t)
	clock := stillClock(control)
	clock.EXPECT().Sleep(gomock.Any(), time.Second).Return(nil).Times(2)
	tool.hosts = web.NewHosts(tool.folder, clock, web.Unix(), nil)
	var said bytes.Buffer
	tool.hosts.Say(&said)
	tool.get.EXPECT().Get(elsewhere).DoAndReturn(served).Times(3)
	through, _ := tool.web()
	asksAndCloses(t, through)
	if err := os.RemoveAll(tool.folder); err != nil {
		t.Fatal(err)
	}
	asksAndCloses(t, through)
	asksAndCloses(t, through)
	if code := tool.finish(ranOK, &said); code != ranOK ||
		!strings.Contains(said.String(), tool.folder) ||
		strings.Count(said.String(), "could not lock ") != 1 {
		t.Errorf("%d %q", code, said.String())
	}
	if code := tool.finish(exitUsage, io.Discard); code != exitUsage {
		t.Errorf("a code of 2 became %d", code)
	}
}

// asksAndCloses makes one request, failing on an error, and releases its
// body so the next request can begin.
func asksAndCloses(t *testing.T, through web.Web) {
	t.Helper()
	body, _, err := through.Get(elsewhere)
	if err != nil {
		t.Fatal(err)
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
}

// The hosts keep their records in celestial-data's folder of the user's
// cache.
func TestTheHostsKeepTheirRecordsInTheUsersCache(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", "")
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if folder := cacheFolder(); folder != filepath.Join(cache,
		"celestial-data") {
		t.Errorf("%q", folder)
	} else if hosts, err := hostsOf(folder); err != nil || hosts == nil {
		t.Errorf("hosts: %v, %v", hosts, err)
	}
}

// Anti-test: a user with no cache folder has no folder, and the hosts
// are still paced, with no record kept.
func TestAUserWithNoCacheFolderKeepsNoRecords(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	if folder := cacheFolder(); folder != "" {
		t.Errorf("%q", folder)
	}
}
