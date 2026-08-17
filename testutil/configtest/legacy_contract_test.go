package configtest

import (
	"testing"

	"github.com/sei-protocol/sei-chain/config/registry"
)

// What CheckLegacyKeysAreDeclared has to refuse, provoked against lists written to be wrong.
//
// The record it keeps is a count of remaining work, and every way that count can be quietly wrong runs
// through the inert list: a key excused without a reason, a key excused that a section already declares,
// and a name left behind after the template stopped writing it. Each makes the number smaller than the
// work, which is the one direction that matters.

// registerLegacyProbe registers a section declaring probe.alpha and nothing else.
func registerLegacyProbe(t *testing.T) {
	t.Helper()
	registry.Reset()
	registry.RegisterSection("probe", &struct {
		Alpha string `mapstructure:"alpha"`
	}{}, func(registry.Mode) any {
		return struct {
			Alpha string `mapstructure:"alpha"`
		}{Alpha: "a"}
	})
	for _, d := range registry.Defects() {
		t.Fatalf("registering the probe section produced a defect: %v", d.Err)
	}
	t.Cleanup(registry.Reset)
}

func TestTheLegacyRecordRefusesAnInertKeyWithNoReason(t *testing.T) {
	registerLegacyProbe(t)

	reported := capture(t, func(tb testing.TB) {
		CheckLegacyKeysAreDeclared(tb, "probe", []string{"probe.alpha", "probe.dead"},
			map[string]string{"probe.dead": ""})
	})

	if len(reported.mentioning("no reason")) != 1 {
		t.Errorf("the unexplained exclusion was not reported: %v", reported.failures)
	}
}

func TestTheLegacyRecordRefusesAnInertKeyASectionDeclares(t *testing.T) {
	registerLegacyProbe(t)

	reported := capture(t, func(tb testing.TB) {
		CheckLegacyKeysAreDeclared(tb, "probe", []string{"probe.alpha"},
			map[string]string{"probe.alpha": "claimed to reach nothing"})
	})

	if len(reported.mentioning("undercounts")) != 1 {
		t.Errorf("a key both declared and called inert was not reported: %v", reported.failures)
	}
}

func TestTheLegacyRecordRefusesAnInertNameNoFileCarries(t *testing.T) {
	registerLegacyProbe(t)

	reported := capture(t, func(tb testing.TB) {
		CheckLegacyKeysAreDeclared(tb, "probe", []string{"probe.alpha"},
			map[string]string{"probe.gone": "the template stopped writing this a release ago"})
	})

	if len(reported.mentioning("probe.gone")) != 1 {
		t.Errorf("a name for a key no file carries was not reported: %v", reported.failures)
	}
}

// TestTheLegacyRecordGivesUpOnAnEmptyReading keeps an empty answer from reading as a finished one.
func TestTheLegacyRecordGivesUpOnAnEmptyReading(t *testing.T) {
	registerLegacyProbe(t)

	reported := capture(t, func(tb testing.TB) {
		CheckLegacyKeysAreDeclared(tb, "probe", nil, nil)
	})

	if !reported.fatal {
		t.Error("a reading with no keys was carried on past, so the record would be written empty and " +
			"the remaining work would read as none")
	}
}
