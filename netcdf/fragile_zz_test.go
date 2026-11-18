//go:build fragile

package netcdf_test

import "testing"

// The last test: the count of what held and what broke, and the notes
// that are not breaks, logged for the report. It never fails.
func TestZTally(t *testing.T) {
	ledger.Lock()
	defer ledger.Unlock()
	t.Logf("TALLY cases %d, held %d, broke %d", ledger.cases, ledger.held,
		ledger.broke)
	t.Logf("TALLY mutants %d, mutants that broke a limit %d",
		ledger.mutants, ledger.mutantsBroke)
	for _, each := range ledger.accepted {
		t.Logf("ACCEPTED %s", each)
	}
	for _, each := range ledger.vague {
		t.Logf("VAGUE %s", each)
	}
	for at, each := range ledger.untyped {
		if at < examplesKept {
			t.Logf("UNTYPED %s", each)
		}
	}
	t.Logf("UNTYPED total %d", len(ledger.untyped))
	for _, each := range ledger.heavy {
		t.Logf("HEAVY %s", each)
	}
}
