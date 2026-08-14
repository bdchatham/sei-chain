package configcli_test

import (
	"testing"

	"github.com/sei-protocol/sei-chain/cmd/seid/cmd/configcli"
	"github.com/sei-protocol/sei-chain/config/registry"
)

// flagDefaults answers as a start command whose flags carry these defaults, held as text the way pflag
// holds every default whatever the flag's type.
func flagDefaults(defaults map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		text, bound := defaults[key]
		return text, bound
	}
}

// TestAFileValueBeatsABoundFlagsDefault is the order the node itself resolves in.
//
// A bound flag's default is the last thing the resolution reaches, below both configuration files. An
// adoption that put it above them would overwrite a value an operator wrote with one nobody chose,
// which is the opposite of what adopting is for.
func TestAFileValueBeatsABoundFlagsDefault(t *testing.T) {
	registerTyped(t)

	got, err := configcli.Adopt(configcli.Existing{
		Files:       existing{"probe.workers": 64},
		FlagDefault: flagDefaults(map[string]string{"probe.workers": "7"}),
		LookupEnv:   noEnv,
	}, registry.ModeValidator)
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}

	written, err := got.File.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if written["probe.workers"] != int64(64) {
		t.Errorf("probe.workers adopted as %#v, want the 64 the file holds. The flag's default is "+
			"below the files, so it answers only where they are silent", written["probe.workers"])
	}
	if !holdsKey(got.Carried, "probe.workers") {
		t.Errorf("probe.workers is reported as %v, want it carried from the files", got.Carried)
	}
	if holdsKey(got.FromFlagDefault, "probe.workers") {
		t.Error("probe.workers is reported as coming from a flag default while the file supplied it")
	}
	assertEveryKeyAccountedFor(t, got)
}

// TestABoundFlagsDefaultIsWhatAnOmittedKeyAdoptsAs is the layer this wiring adds.
//
// A key absent from both configuration files resolves to the default of a flag bound to it, so a node
// whose files omit it has been running that default all along. Writing anything else moves the node
// off a value it was running, silently, which is exactly what adoption exists to prevent.
func TestABoundFlagsDefaultIsWhatAnOmittedKeyAdoptsAs(t *testing.T) {
	registerTyped(t)

	got, err := configcli.Adopt(configcli.Existing{
		Files: existing{},
		FlagDefault: flagDefaults(map[string]string{
			"probe.workers":  "7",
			"probe.enabled":  "false", // the validator baseline is true, so this has to be visible
			"probe.endpoint": "flag:9000",
			"probe.ratio":    "0.75",
			"probe.timeout":  "2m",
		}),
		LookupEnv: noEnv,
	}, registry.ModeValidator)
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}

	written, err := got.File.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	for key, want := range map[string]any{
		"probe.workers":  int64(7),
		"probe.enabled":  false,
		"probe.endpoint": "flag:9000",
		"probe.ratio":    0.75,
		"probe.timeout":  "2m0s",
	} {
		if written[key] != want {
			t.Errorf("%s adopted as %#v, want the %#v its bound flag defaults to", key,
				written[key], want)
		}
	}
	if len(got.FromFlagDefault) != 5 {
		t.Errorf("reported %v as coming from a flag default, want the five that are bound",
			got.FromFlagDefault)
	}
	if len(got.Carried) != 0 {
		t.Errorf("reported %v as carried, but the files hold nothing", got.Carried)
	}
	// The one key no flag is bound to still falls through to what an absent key resolves to.
	if !holdsKey(got.Unsupplied, "probe.peers") {
		t.Errorf("probe.peers is reported as %v, want it unsupplied: no layer holds it",
			got.Unsupplied)
	}
	assertEveryKeyAccountedFor(t, got)
}

// TestAFlagDefaultThatCannotBeReadIsReportedAndFallsThrough keeps our own mistake out of the file.
//
// A flag's default failing to read as its key's declared type is a defect in this binary rather than
// anything an operator did. Writing it anyway produces a file the node refuses at its next boot, so
// the key falls through to its absent value and the report names it.
func TestAFlagDefaultThatCannotBeReadIsReportedAndFallsThrough(t *testing.T) {
	registerTyped(t)

	got, err := configcli.Adopt(configcli.Existing{
		Files:       existing{},
		FlagDefault: flagDefaults(map[string]string{"probe.workers": "lots"}),
		LookupEnv:   noEnv,
	}, registry.ModeValidator)
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}

	if len(got.Unconvertible) != 1 || got.Unconvertible[0].Key != "probe.workers" {
		t.Fatalf("reported %+v, want probe.workers refused", got.Unconvertible)
	}
	written, err := got.File.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if written["probe.workers"] != int64(4) {
		t.Errorf("probe.workers adopted as %#v from an unreadable flag default, want the 4 an absent "+
			"key resolves to", written["probe.workers"])
	}
	if holdsKey(got.FromFlagDefault, "probe.workers") {
		t.Error("a flag default that could not be read is still reported as having supplied the value")
	}
	assertEveryKeyAccountedFor(t, got)
}

// TestAnUnsuppliedKeyAdoptsAsWhatTheReaderResolves is why adoption reads absent values at all.
//
// Most readers keep their default when a key is absent, and for those the absent value is the
// baseline. A reader that assigns straight from its lookup resolves the zero instead, and the default
// beside it is one the node has never run. Writing the baseline for such a key is the change that
// turns adopting a working node into changing it.
func TestAnUnsuppliedKeyAdoptsAsWhatTheReaderResolves(t *testing.T) {
	registerTyped(t)
	registry.DeclareZeroWhenAbsent("probe", "probe.workers")
	for _, d := range registry.Defects() {
		t.Fatalf("declaring probe.workers produced a defect: %v", d.Err)
	}

	got, err := configcli.Adopt(from(existing{}), registry.ModeValidator)
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}

	written, err := got.File.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if written["probe.workers"] != int64(0) {
		t.Errorf("probe.workers adopted as %#v, want the 0 its reader resolves when nothing supplies "+
			"it. The baseline 4 is a value this node has never run", written["probe.workers"])
	}
	// A key with no such declaration still takes its baseline, or the test above would pass against an
	// adoption that wrote zeroes throughout.
	if written["probe.endpoint"] != "localhost:8545" {
		t.Errorf("probe.endpoint adopted as %#v, want its baseline: its reader keeps its default when "+
			"the key is absent", written["probe.endpoint"])
	}
}

// TestStartFlagDefaultsReadsTheRealStartCommand keeps the layer from answering for nothing.
//
// The flags are read off the start command rather than restated, so this holds that the reading works
// at all: a flag cosmos binds is found, with the default it was registered with, and a key no flag
// carries is reported as unbound rather than as an empty default.
func TestStartFlagDefaultsReadsTheRealStartCommand(t *testing.T) {
	bound := configcli.StartFlagDefaults(t.TempDir())

	text, found := bound("pruning")
	if !found {
		t.Error("the start command binds a pruning flag and StartFlagDefaults did not find it")
	}
	if text != "default" {
		t.Errorf("pruning defaults to %q, want the \"default\" the flag registers. This is the value a "+
			"node whose app.toml omits pruning has been running", text)
	}
	if _, found := bound("state-store.ss-enable"); found {
		t.Error("state-store.ss-enable is reported as flag-bound. No start flag carries it, and " +
			"treating it as bound would write a flag default over what its reader resolves")
	}
	if _, found := bound("nothing.binds.this"); found {
		t.Error("a key no flag binds is reported as bound, so every unsupplied key would adopt as an " +
			"empty default")
	}
}
