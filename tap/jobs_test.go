package tap_test

import (
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/tap"
	"go.uber.org/mock/gomock"
)

// The service the job tests ask, and what it names.
const (
	async = "https://gea.esac.esa.int/tap-server/tap/async"
	job   = async + "/1730"
	query = "SELECT 1"
)

// uws is a job's description in the phase given, as the Gaia archive
// writes one, with a message for a job that failed.
func uws(phase string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(`<?xml version="1.0"?>` +
		`<uws:job xmlns:uws="http://www.ivoa.net/xml/UWS/v1.0">` +
		`<uws:jobId>1730</uws:jobId><uws:phase>` + phase +
		`</uws:phase><uws:errorSummary><uws:message> column nope ` +
		`</uws:message></uws:errorSummary></uws:job>`))
}

// body is a body holding text.
func body(text string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(text))
}

// submitted is the form a job's query is submitted with.
func submitted() url.Values {
	return url.Values{"REQUEST": {"doQuery"}, "LANG": {"ADQL"},
		"FORMAT": {"fits"}, "QUERY": {query}, "PHASE": {"run"}}
}

// jobsOver is a web of jobs over a mock that asks at most three times,
// and tells what it says to nobody.
func jobsOver(t *testing.T) (*tap.Jobs, *mocks.MockWeb) {
	t.Helper()
	return jobsTelling(t, io.Discard)
}

// jobsTelling is a web of jobs over a mock that asks at most three
// times, and says each job's address to told.
func jobsTelling(t *testing.T, told io.Writer) (*tap.Jobs,
	*mocks.MockWeb) {
	t.Helper()
	under := mocks.NewMockWeb(gomock.NewController(t))
	jobs, err := tap.NewJobs(under, time.Nanosecond, 3, told)
	if err != nil {
		t.Fatal(err)
	}
	return jobs, under
}

func TestAJobIsSubmittedWaitedOnAndItsAnswerOpened(t *testing.T) {
	var told strings.Builder
	jobs, under := jobsTelling(t, &told)
	gomock.InOrder(
		under.EXPECT().Post(async, submitted()).Return(job, nil),
		under.EXPECT().Get(job).Return(uws("QUEUED"), int64(-1), nil),
		under.EXPECT().Get(job).Return(uws("COMPLETED"), int64(-1), nil),
		under.EXPECT().Get(job+"/results/result").Return(body("SIMPLE"),
			int64(6), nil))
	got, length, err := jobs.Get(tap.URL(async, query, "fits"))
	if err != nil || length != 6 {
		t.Fatalf("got %d, %v", length, err)
	}
	defer got.Close()
	if text, _ := io.ReadAll(got); string(text) != "SIMPLE" {
		t.Fatalf("got %q", text)
	}
	if want := "job " + job + " submitted\njob " + job + " complete\n"; told.String() != want {
		t.Fatalf("told %q, want %q", told.String(), want)
	}
}

func TestAJobThatFailsIsNeverToldComplete(t *testing.T) {
	var told strings.Builder
	jobs, under := jobsTelling(t, &told)
	under.EXPECT().Post(async, submitted()).Return(job, nil)
	under.EXPECT().Get(job).Return(uws("ERROR"), int64(-1), nil)
	if _, _, err := jobs.Get(tap.URL(async, query, "fits")); err == nil {
		t.Fatal("a failed job was opened")
	}
	if told.String() != "job "+job+" submitted\n" {
		t.Fatalf("told %q", told.String())
	}
}

func TestAJobThatFailsOrStopsSaysWhy(t *testing.T) {
	for _, phase := range []string{"ERROR", "ABORTED"} {
		jobs, under := jobsOver(t)
		under.EXPECT().Post(async, submitted()).Return(job, nil)
		under.EXPECT().Get(job).Return(uws(phase), int64(-1), nil)
		_, _, err := jobs.Get(tap.URL(async, query, "fits"))
		if err == nil || !strings.Contains(err.Error(), phase+": "+
			"column nope") {
			t.Errorf("%s: got %v", phase, err)
		}
	}
}

func TestAJobNotDoneAfterEveryAskIsGivenUpOn(t *testing.T) {
	jobs, under := jobsOver(t)
	under.EXPECT().Post(async, submitted()).Return(job, nil)
	under.EXPECT().Get(job).Times(3).DoAndReturn(func(string) (
		io.ReadCloser, int64, error) {
		return uws("EXECUTING"), -1, nil
	})
	_, _, err := jobs.Get(tap.URL(async, query, "fits"))
	if err == nil || !strings.Contains(err.Error(), "EXECUTING still") {
		t.Fatalf("got %v", err)
	}
}

func TestAJobThatCannotBeAskedAfterIsAnError(t *testing.T) {
	// each is refused for its own reason: no phase, a description cut
	// off, and no answer at all
	for reply, says := range map[string]string{"<uws:job/>": "says no phase",
		"<uws:job": "XML syntax error", "ask": "gone"} {
		jobs, under := jobsOver(t)
		under.EXPECT().Post(async, submitted()).Return(job, nil)
		if reply == "ask" {
			under.EXPECT().Get(job).Return(nil, int64(0),
				errors.New("gone"))
		} else {
			under.EXPECT().Get(job).Return(body(reply), int64(-1), nil)
		}
		if _, _, err := jobs.Get(tap.URL(async, query,
			"fits")); err == nil || !strings.Contains(err.Error(), says) {
			t.Errorf("%q gave %v, not %q", reply, err, says)
		}
	}
}

func TestASubmissionNotTakenAsAJobIsAnError(t *testing.T) {
	for _, taken := range []error{errors.New("POST: 400"), nil} {
		jobs, under := jobsOver(t)
		under.EXPECT().Post(async, submitted()).Return("", taken)
		if _, _, err := jobs.Get(tap.URL(async, query,
			"fits")); err == nil {
			t.Errorf("with %v, no job was taken as one", taken)
		}
	}
}

func TestAnythingButAJobIsAskedUnchanged(t *testing.T) {
	jobs, under := jobsOver(t)
	sync := "https://gea.esac.esa.int/tap-server/tap/sync"
	for _, address := range []string{tap.URL(sync, query, "fits"),
		async + "?REQUEST=getCapabilities", "https://a/%zz"} {
		under.EXPECT().Get(address).Return(body("x"), int64(1), nil)
		under.EXPECT().Head(address).Return(int64(1), nil)
		if _, length, err := jobs.Get(address); err != nil || length != 1 {
			t.Errorf("Get %s: %d, %v", address, length, err)
		}
		if length, err := jobs.Head(address); err != nil || length != 1 {
			t.Errorf("Head %s: %d, %v", address, length, err)
		}
	}
	under.EXPECT().Post(sync, nil).Return("", nil)
	if _, err := jobs.Post(sync, nil); err != nil {
		t.Error(err)
	}
}

func TestTheLengthOfAJobsAnswerIsUnsaidAndNothingIsRun(t *testing.T) {
	jobs, _ := jobsOver(t) // the mock takes no call
	length, err := jobs.Head(tap.URL(async, query, "fits"))
	if err != nil || length != -1 {
		t.Fatalf("got %d, %v", length, err)
	}
}

func TestNewJobsRefusesWhatCannotRunAJob(t *testing.T) {
	under := mocks.NewMockWeb(gomock.NewController(t))
	for _, each := range []struct {
		every time.Duration
		asks  int
	}{{0, 1}, {time.Second, 0}} {
		if _, err := tap.NewJobs(under, each.every, each.asks,
			io.Discard); err == nil {
			t.Errorf("%+v was taken", each)
		}
	}
	if _, err := tap.NewJobs(nil, time.Second, 1, io.Discard); err == nil {
		t.Error("jobs over no web were taken")
	}
	// one ask is enough to make jobs with
	if _, err := tap.NewJobs(under, time.Second, 1, io.Discard); err != nil {
		t.Errorf("jobs of one ask were refused: %v", err)
	}
	if _, err := tap.NewJobs(under, time.Second, 1, nil); err == nil {
		t.Error("jobs that tell no one were taken")
	}
}
