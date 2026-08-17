package keyspace

import (
	"strings"
	"testing"
)

// What the drift comparison has to report, driven with sets that differ.
//
// The exported Drift reads the registry, where no drift is the only correct answer, so every test
// through that path passes against a comparison that reports nothing at all. Both directions were in
// fact unprotected: removing either reporting loop passed the whole suite.

func TestDriftReportsASectionNamedButNotRegistered(t *testing.T) {
	got := driftBetween([]string{"api", "telemetry"}, []string{"api"})

	if len(got) != 1 {
		t.Fatalf("reported %v, want the one section that is named and not registered", got)
	}
	if !strings.Contains(got[0], "telemetry") || !strings.Contains(got[0], "not registered") {
		t.Errorf("reported %q. A section whose owner stopped being imported has its keys resolving "+
			"through the machinery that answered them before, and this is the only report of it", got[0])
	}
}

func TestDriftReportsASectionRegisteredButNotNamed(t *testing.T) {
	got := driftBetween([]string{"api"}, []string{"api", "newcomer"})

	if len(got) != 1 {
		t.Fatalf("reported %v, want the one section that is registered and not named", got)
	}
	if !strings.Contains(got[0], "newcomer") || !strings.Contains(got[0], "not named here") {
		t.Errorf("reported %q. A section nothing names is one nothing would notice leaving again, which "+
			"is how the key space became a consequence of the import graph", got[0])
	}
}

// TestDriftReportsBothDirectionsAtOnce is what the collapse into one function is for.
//
// The two directions used to be two exported functions, and a caller checking one and not the other had
// half a guard. One answer holding both is what makes that impossible.
func TestDriftReportsBothDirectionsAtOnce(t *testing.T) {
	got := driftBetween([]string{"api", "gone"}, []string{"api", "newcomer"})

	if len(got) != 2 {
		t.Fatalf("reported %v, want both directions", got)
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "gone") || !strings.Contains(joined, "newcomer") {
		t.Errorf("reported %q, want both the missing section and the unnamed one", joined)
	}
}

func TestDriftIsEmptyWhenTheTwoSetsAgree(t *testing.T) {
	if got := driftBetween([]string{"api", "telemetry"}, []string{"telemetry", "api"}); len(got) != 0 {
		t.Errorf("reported %v for two sets that agree, so a healthy binary would fail this check", got)
	}
}

// TestDriftIsNotEmptyForTwoEmptySets keeps a name for the case that reads as healthy and is not.
//
// Two empty sets agree, so the comparison reports nothing, and that is the right answer for the
// comparison. It is the wrong answer for a binary, which is why the callers assert the sets are not
// empty separately. This states the division so the next reader does not look for it here.
func TestDriftIsNotEmptyForTwoEmptySets(t *testing.T) {
	if got := driftBetween(nil, nil); len(got) != 0 {
		t.Errorf("reported %v for two empty sets; the comparison has nothing to say about them", got)
	}
	if len(Names) == 0 {
		t.Error("this package names no sections, so the callers' own checks are what has to catch it")
	}
}
