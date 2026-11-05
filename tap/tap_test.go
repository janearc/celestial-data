package tap_test

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/tap"
	"go.uber.org/mock/gomock"
	"pgregory.net/rapid"
)

// heasarc is HEASARC's sync endpoint, which the recorded replies came
// from.
const heasarc = "https://heasarc.gsfc.nasa.gov/xamin/vo/tap/sync"

// The rows of the recorded reply. They are defined by its query.
const (
	first = 0 // defined: the first row
	rows  = 5 // defined: the TOP the query asked for
	one   = 1 // defined: a search for the one row
)

// numaster is the query the recorded NuSTAR reply answers.
func numaster(t *testing.T) string {
	return "SELECT TOP 5 obsid, name, exposure_a, ra, dec FROM numaster " +
		"WHERE " + tap.Within(replay.Asked(t), "ra", "dec") +
		" AND exposure_a > 0 ORDER BY exposure_a DESC"
}

// The circle is written as ADQL reads it: no exponents, every digit kept.
func TestWithinWritesTheCircleInFull(t *testing.T) {
	want := "CONTAINS(POINT('ICRS', ra, dec), CIRCLE('ICRS', " +
		"266.4168371, -29.0078106, 0.1)) = 1"
	if got := tap.Within(replay.Asked(t), "ra", "dec"); got != want {
		t.Errorf("%s", got)
	}
}

func TestQueryReadsTheServicesAnswer(t *testing.T) {
	address := tap.URL(heasarc, numaster(t), "votable")
	get := replay.Web(t, map[string]string{address: "numaster-sgra.xml"})
	table, err := tap.Query(get, heasarc, numaster(t))
	if err != nil {
		t.Fatal(err)
	}
	obsid, _ := table.Text(first, "obsid")
	if table.Len() != rows || obsid != "30001002001" {
		t.Errorf("%d rows, the first %s", table.Len(), obsid)
	}
}

// Anti-tests: a service that says the query failed is an error. So is
// one that cannot be reached.
func TestQueryReportsWhatWentWrong(t *testing.T) {
	adql := "SELECT instrument FROM chanmaster"
	address := tap.URL(heasarc, adql, "votable")
	said := replay.Web(t, map[string]string{address: "error-no-column.xml"})
	_, err := tap.Query(said, heasarc, adql)
	if !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), heasarc) {
		t.Errorf("%v", err)
	}
	down := errors.New("down")
	unreached := mocks.NewMockWeb(gomock.NewController(t))
	unreached.EXPECT().Get(address).Return(nil, int64(rows), down)
	if _, err := tap.Query(unreached, heasarc, adql); !errors.Is(err, down) {
		t.Errorf("%v", err)
	}
}

// asksOnce is the property of TestURLAsksExactlyTheQuery, for one draw.
func asksOnce(t *rapid.T) {
	adql := rapid.String().Draw(t, "adql")
	format := rapid.SampledFrom([]string{"votable", tap.CSV}).
		Draw(t, "format")
	address, err := url.Parse(tap.URL(heasarc, adql, format))
	if err != nil {
		t.Fatal(err)
	}
	asked := address.Query()
	if address.Scheme+"://"+address.Host+address.Path != heasarc ||
		asked.Get("QUERY") != adql || asked.Get("FORMAT") != format ||
		asked.Get("LANG") != "ADQL" || asked.Get("REQUEST") != "doQuery" {
		t.Fatalf("%s", address)
	}
}

// Property: whatever the query says, the address asks exactly it. It
// asks in the format asked for, of the service named.
func TestURLAsksExactlyTheQuery(t *testing.T) {
	rapid.Check(t, asksOnce)
}

// Search writes each part only when it is asked for.
func TestSearchWritesOnlyWhatIsAsked(t *testing.T) {
	within := tap.Within(replay.Asked(t), "ra", "dec")
	for want, got := range map[string]string{
		"SELECT * FROM galcen_psc WHERE " + within: tap.Search(tap.Every,
			"*", "galcen_psc", replay.Asked(t), "", ""),
		"SELECT TOP 5 obsid FROM numaster WHERE " + within +
			" AND exposure_a > 0 ORDER BY exposure_a DESC": tap.Search(rows,
			"obsid", "numaster", replay.Asked(t), "exposure_a > 0",
			"exposure_a DESC"),
		"SELECT TOP 1 obsid FROM numaster WHERE " + within: tap.Search(one,
			"obsid", "numaster", replay.Asked(t), "", ""),
	} {
		if got != want {
			t.Errorf("%s", got)
		}
	}
}
