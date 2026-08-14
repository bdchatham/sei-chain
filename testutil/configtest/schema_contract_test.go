package configtest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sei-protocol/sei-chain/config/registry"
)

// What CheckSchemaMatchesTheReader has to catch, provoked against a reader written to be wrong.
//
// The check exists for sections whose keys no derivation produces, so it is the only statement that a
// declared key reaches a setting at all. A check like that passing for the wrong reason costs a section
// its coverage silently, and nothing else in the suite would notice, which is why its failures are
// provoked here rather than trusted.

// schemaSetting is a reader's output: two settings, each reached by one key.
type schemaSetting struct {
	Alpha string
	Bravo int
}

// schemaSpelling declares the keys, the way a section with no usable type of its own does.
type schemaSpelling struct {
	Alpha string `mapstructure:"alpha"`
	Bravo int    `mapstructure:"bravo"`
}

const (
	schemaSection = "probe"
	alphaKey      = "probe.alpha"
	bravoKey      = "probe.bravo"
)

// registerSchemaSection registers the section above and returns a correct reader for it.
func registerSchemaSection(t *testing.T) func(AppOpts) (any, error) {
	t.Helper()
	registry.Reset()
	registry.RegisterSection(schemaSection, &schemaSpelling{}, func(registry.Mode) any {
		return schemaSpelling{Alpha: "a", Bravo: 1}
	})
	for _, d := range registry.Defects() {
		t.Fatalf("registering the probe section produced a defect: %v", d.Err)
	}
	t.Cleanup(registry.Reset)

	return func(opts AppOpts) (any, error) {
		// A value of another shape is ignored rather than refused, which is what a casting reader does
		// and what the shape check exists for: without that check the probe would silently reach
		// nothing, and the key would report as one the reader never looks up.
		out := schemaSetting{Alpha: "a", Bravo: 1}
		if v, readable := opts[alphaKey].(string); readable {
			out.Alpha = v
		}
		if v, readable := opts[bravoKey].(int); readable {
			out.Bravo = v
		}
		return out, nil
	}
}

// goodProbes are values a correct reader moves each setting to.
func goodProbes() map[string]any {
	return map[string]any{alphaKey: "z", bravoKey: 9}
}

// TestTheSchemaCheckPassesAReaderThatResolvesEveryKey is the baseline every case below is measured
// against. Without it a check that reported on everything would satisfy them all.
func TestTheSchemaCheckPassesAReaderThatResolvesEveryKey(t *testing.T) {
	read := registerSchemaSection(t)

	reported := capture(t, func(tb testing.TB) {
		CheckSchemaMatchesTheReader(tb, schemaSection, SchemaCheck{Read: read, Probe: goodProbes()})
	})
	if len(reported.failures) != 0 {
		t.Errorf("a correct reader was reported on:\n%s", strings.Join(reported.failures, "\n---\n"))
	}
}

// TestTheSchemaCheckCatchesAKeyTheReaderDoesNotResolve is the failure the check is for.
//
// A declared key its reader never looks up is an operator's value that reaches nothing. No other check
// in the suite sees it for these sections, because there is no derivation to compare the key against.
func TestTheSchemaCheckCatchesAKeyTheReaderDoesNotResolve(t *testing.T) {
	registerSchemaSection(t)
	ignoresBravo := func(opts AppOpts) (any, error) {
		out := schemaSetting{Alpha: "a", Bravo: 1}
		if v, readable := opts[alphaKey].(string); readable {
			out.Alpha = v
		}
		return out, nil
	}

	reported := capture(t, func(tb testing.TB) {
		CheckSchemaMatchesTheReader(tb, schemaSection, SchemaCheck{Read: ignoresBravo, Probe: goodProbes()})
	})

	msg := reported.only(t)
	if !strings.Contains(msg, bravoKey) || !strings.Contains(msg, "changed nothing") {
		t.Errorf("the report does not say which key reached nothing: %s", msg)
	}
}

// TestTheSchemaCheckCatchesAKeyThatMovesTwoSettings keeps the one-setting rule meaningful.
//
// A key that moves two settings has no single setting this check can name, so it cannot say the schema
// pairs it correctly. Accepting it would let a reader that writes an operator's value into a second
// place pass unreported.
func TestTheSchemaCheckCatchesAKeyThatMovesTwoSettings(t *testing.T) {
	registerSchemaSection(t)
	alphaAlsoMovesBravo := func(opts AppOpts) (any, error) {
		out := schemaSetting{Alpha: "a", Bravo: 1}
		if v, readable := opts[alphaKey].(string); readable {
			out.Alpha = v
			out.Bravo = 99
		}
		if v, readable := opts[bravoKey].(int); readable {
			out.Bravo = v
		}
		return out, nil
	}

	reported := capture(t, func(tb testing.TB) {
		CheckSchemaMatchesTheReader(tb, schemaSection,
			SchemaCheck{Read: alphaAlsoMovesBravo, Probe: goodProbes()})
	})

	msg := reported.only(t)
	if !strings.Contains(msg, alphaKey) || !strings.Contains(msg, "Alpha") ||
		!strings.Contains(msg, "Bravo") {
		t.Errorf("the report does not name the key and both settings it moved: %s", msg)
	}
	if !strings.Contains(msg, "skipping with the reason") {
		t.Errorf("the report does not say what to do about it, which is the one thing the reader of a "+
			"failure needs: %s", msg)
	}
}

// TestTheSchemaCheckCatchesAProbeOfTheWrongShape holds the probe to what an operator can deliver.
//
// The resolved configuration delivers the declared shape. A probe of another shape checks a value that
// cannot arrive, so a reader accepting only that other shape would look as though it accepted the
// declared one.
func TestTheSchemaCheckCatchesAProbeOfTheWrongShape(t *testing.T) {
	read := registerSchemaSection(t)

	reported := capture(t, func(tb testing.TB) {
		CheckSchemaMatchesTheReader(tb, schemaSection, SchemaCheck{
			Read:  read,
			Probe: map[string]any{alphaKey: 7, bravoKey: 9}, // alpha is declared as text
		})
	})

	msg := reported.only(t)
	if !strings.Contains(msg, alphaKey) || !strings.Contains(msg, "string") ||
		!strings.Contains(msg, "int") {
		t.Errorf("the report does not name the key and both shapes: %s", msg)
	}
}

// TestTheSchemaCheckCatchesAKeyNothingCovers is what stops coverage being lost by omission.
//
// A declared key with neither a probe nor a skip is a key this check silently steps over, and stepping
// over one reads exactly like covering it.
func TestTheSchemaCheckCatchesAKeyNothingCovers(t *testing.T) {
	read := registerSchemaSection(t)

	reported := capture(t, func(tb testing.TB) {
		CheckSchemaMatchesTheReader(tb, schemaSection, SchemaCheck{
			Read:  read,
			Probe: map[string]any{alphaKey: "z"}, // bravo is declared and neither probed nor skipped
		})
	})

	msg := reported.only(t)
	if !strings.Contains(msg, bravoKey) || !strings.Contains(msg, "no probe value") {
		t.Errorf("the report does not name the uncovered key: %s", msg)
	}
}

// TestTheSchemaCheckCatchesASkipWithNoReason keeps a skip from being an unexplained hole.
//
// A skip says another test covers the key. Without the reason there is nothing to check that claim
// against, and it is indistinguishable from a key nothing covers at all.
func TestTheSchemaCheckCatchesASkipWithNoReason(t *testing.T) {
	read := registerSchemaSection(t)

	reported := capture(t, func(tb testing.TB) {
		CheckSchemaMatchesTheReader(tb, schemaSection, SchemaCheck{
			Read:  read,
			Probe: map[string]any{alphaKey: "z"},
			Skip:  map[string]string{bravoKey: ""},
		})
	})

	msg := reported.only(t)
	if !strings.Contains(msg, bravoKey) || !strings.Contains(msg, "no reason") {
		t.Errorf("the report does not name the unexplained skip: %s", msg)
	}
}

// TestTheSchemaCheckGivesUpOnASectionItCannotRead reports rather than passing over nothing.
//
// A check that ran over a section with no keys would hold by covering nothing, and a section name that
// does not resolve is a test wired to a section that never registered. Both give up, because there is
// nothing later in the check that could still be meaningful.
func TestTheSchemaCheckGivesUpOnASectionItCannotRead(t *testing.T) {
	read := registerSchemaSection(t)

	t.Run("not registered", func(t *testing.T) {
		reported := capture(t, func(tb testing.TB) {
			CheckSchemaMatchesTheReader(tb, "never-registered", SchemaCheck{Read: read, Probe: goodProbes()})
		})
		if !reported.fatal {
			t.Error("an unregistered section was reported and carried on past, so the rest of the check " +
				"ran against a section that does not exist")
		}
		if msg := reported.only(t); !strings.Contains(msg, "not registered") {
			t.Errorf("the report does not say the section is unregistered: %s", msg)
		}
	})

	t.Run("reader refuses an empty configuration", func(t *testing.T) {
		reported := capture(t, func(tb testing.TB) {
			CheckSchemaMatchesTheReader(tb, schemaSection, SchemaCheck{
				Read:  func(AppOpts) (any, error) { return nil, fmt.Errorf("no") },
				Probe: goodProbes(),
			})
		})
		if !reported.fatal {
			t.Error("a reader that refuses an empty configuration was carried on past. Every comparison " +
				"below it reads that same empty configuration, so none of them mean anything")
		}
		if msg := reported.only(t); !strings.Contains(msg, "written\nnothing has") &&
			!strings.Contains(msg, "nothing has") {
			t.Errorf("the report does not say what configuration was refused: %s", msg)
		}
	})
}
