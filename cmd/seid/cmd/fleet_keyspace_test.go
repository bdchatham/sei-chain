package cmd

import (
	"path/filepath"
	"testing"

	"github.com/sei-protocol/sei-chain/cmd/seid/cmd/configcli"
	"github.com/sei-protocol/sei-chain/config/registry"
	"github.com/sei-protocol/sei-chain/testutil/configtest"
)

// What a node already in the field carries that this binary no longer reads.
//
// TestEveryKeyANodesFilesCarryHasASectionThatOwnsIt measures files this binary renders, and has to read
// empty: a key a current node can write and no section owns is a value the migration drops. These
// fixtures measure the other case. They are the app.toml and config.toml of three running arctic-1
// nodes, so they were written by an older release and hold keys this one has since renamed or stopped
// reading. A non-empty record here is expected, and each line is a key an operator's file can hold whose
// value no longer reaches anything.
//
// The fixtures are verbatim except for peer addresses and the state sync trust hash, which name internal
// hosts and are replaced. Every value shape a reading depends on is the node's own, including the
// comma-joined lists and the quoted durations.

// fleetNodes are the fixtures, with the mode each node runs under.
//
// The mode comes from the fixture rather than from a guess, because every baseline a value is compared
// against is one mode's, and adopting a full node's files as a validator produces a complete and wrong
// file. Each node's own config.toml states it.
var fleetNodes = []struct {
	name string
	mode registry.Mode
}{
	{"rpc", registry.ModeFull},
	{"seed", registry.ModeSeed},
	{"validator", registry.ModeValidator},
}

// TestEveryKeyAFleetNodesFilesCarryIsAccountedFor records what adoption would read past.
//
// One call per fixture, each naming its record with a literal. A loop over fleetNodes would collapse the
// three into one row of the wiring record, and deleting any of them would then leave that record
// unchanged.
func TestEveryKeyAFleetNodesFilesCarryIsAccountedFor(t *testing.T) {
	configtest.Isolate(t)

	configtest.CheckLegacyKeysAreDeclared(t, "fleet_rpc",
		fleetKeys(t, "rpc"), inertInTheRenderedFiles)
	configtest.CheckLegacyKeysAreDeclared(t, "fleet_seed",
		fleetKeys(t, "seed"), inertInTheRenderedFiles)
	configtest.CheckLegacyKeysAreDeclared(t, "fleet_validator",
		fleetKeys(t, "validator"), inertInTheRenderedFiles)
}

// TestEachFleetFixtureStatesTheModeItIsAdoptedAs holds the fixtures to the modes the tests use.
//
// Every baseline an adopted value is compared against belongs to one mode, so a fixture refreshed from a
// node running a different one would be adopted under the old mode and produce a record that still looks
// plausible. Each node's own config.toml is what settles it.
func TestEachFleetFixtureStatesTheModeItIsAdoptedAs(t *testing.T) {
	configtest.Isolate(t)

	for _, node := range fleetNodes {
		if got := fleetSource(t, node.name).Get("mode"); got != string(node.mode) {
			t.Errorf("the %s fixture's config.toml says mode = %v and the tests adopt it as %q",
				node.name, got, node.mode)
		}
	}
}

// TestAFleetNodesValuesAreAllReadableAsTheirDeclaredType is the property that makes adoption usable.
//
// A value adoption cannot read as its setting's type is written at what an absent key resolves to, so
// every one is a setting the node had and the new file gets wrong. On files this binary rendered that
// cannot happen, because the same code wrote and read them. These are files it did not render, and the
// shapes an older renderer chose are the ones nothing else here exercises.
func TestAFleetNodesValuesAreAllReadableAsTheirDeclaredType(t *testing.T) {
	for _, node := range fleetNodes {
		t.Run(node.name, func(t *testing.T) {
			configtest.Isolate(t)

			adoption, err := configcli.Adopt(configcli.Existing{
				Files: fleetSource(t, node.name),
				// No flag set and no environment. Both belong to the invocation rather than to the node,
				// so including this machine's would make the result depend on where the test ran.
			}, node.mode)
			if err != nil {
				t.Fatalf("adopt the %s fixture: %v", node.name, err)
			}
			for _, r := range adoption.Unconvertible {
				t.Errorf("%s held %#v, which adoption could not read as its declared type: %s",
					r.Key, r.Value, r.Reason)
			}
			if len(adoption.Carried) == 0 {
				t.Fatal("nothing was carried, so this would pass on a fixture that holds no settings")
			}
		})
	}
}

// TestAdoptionNamesTheKeysItReadPast holds adoption to reporting what it dropped.
//
// The count alone cannot show this: a report that named none would still say how many settings it
// carried, and an operator reading it has no way to tell a file that lost nothing from one that lost a
// setting the node was running. The fleet fixtures are where that is not hypothetical.
func TestAdoptionNamesTheKeysItReadPast(t *testing.T) {
	configtest.Isolate(t)

	adoption, err := configcli.Adopt(configcli.Existing{
		Files: fleetSource(t, "validator"),
	}, registry.ModeValidator)
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if len(adoption.Undeclared) == 0 {
		t.Fatal("the validator fixture holds keys no section declares, and adoption named none of them")
	}
	report := adoption.Report()
	for _, key := range adoption.Undeclared {
		if !contains(report, key) {
			t.Errorf("adoption read past %q and its report does not name it", key)
		}
	}
}

// fleetKeys reads every key one fixture carries.
func fleetKeys(t *testing.T, name string) []string {
	t.Helper()
	return fleetSource(t, name).AllKeys()
}

// fleetSource reads one fixture through the migration's own reader.
//
// The same call the command makes, so this measures what a migration would see. A separate reading here
// would drift from it, and both would still produce a plausible list.
func fleetSource(t *testing.T, name string) configcli.Source {
	t.Helper()
	home, err := filepath.Abs(filepath.Join("testdata", "fleet", name))
	if err != nil {
		t.Fatalf("resolve the %s fixture: %v", name, err)
	}
	existing, err := configcli.LegacySource(home)
	if err != nil {
		t.Fatalf("read the %s fixture: %v", name, err)
	}
	return existing
}

// contains reports whether the report names a key.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
