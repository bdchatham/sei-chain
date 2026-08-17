package config_test

import (
	"testing"

	"github.com/sei-protocol/sei-chain/giga/executor/config"
	"github.com/sei-protocol/sei-chain/testutil/configtest"
	"github.com/sei-protocol/sei-chain/testutil/fuzzing"
)

// gigaKeys is the [giga_executor] section's read-site manifest. Both keys are
// guarded and checked, so an absent key keeps the in-code default (true for
// both) and a value that will not convert to a bool is a boot error rather than a
// silent false.
//
// The defaults matter beyond this section: giga_executor.enabled also decides
// whether app.New flips the process-global atomic that relaxes consensus
// LastResultsHash validation, so a regression that let an absent or malformed key
// resolve to false would change consensus behavior without changing app.toml.
var gigaKeys = []configtest.KeySpec{
	{
		Key: config.FlagEnabled, Path: "Enabled", Cast: configtest.CastBool,
		Checked: true,
		Why:     "default true; also gates the SkipLastResultsHashValidation atomic in app.New",
	},
	{
		Key: config.FlagOCCEnabled, Path: "OCCEnabled", Cast: configtest.CastBool,
		Checked: true,
		Why:     "default true; absent must not silently disable optimistic concurrency",
	},
}

func readGiga(opts configtest.AppOpts) (any, error) { return config.ReadConfig(opts) }

// FuzzReadConfig drives every [giga_executor] key through arbitrary raw values
// and holds ReadConfig to its contract: an absent or nil key keeps the default,
// a convertible value is adopted verbatim, and an inconvertible one errors
// instead of resolving to false.
func FuzzReadConfig(f *testing.F) {
	// Seeds cover the shapes an operator can actually produce: a TOML bool, the
	// string spellings viper hands back from an environment variable, a numeric
	// spelling cast accepts, and the malformed value that must not become false.
	seeds := configtest.NewSeeds(f, fuzzing.ConfigValue)
	seeds.AddRow(uint(0), fuzzing.KindBool, "true", int64(1), true)
	seeds.AddRow(uint(1), fuzzing.KindBoolString, "false", int64(0), false)
	seeds.AddRow(uint(0), fuzzing.KindString, "maybe", int64(0), false)
	seeds.AddRow(uint(1), fuzzing.KindString, "1", int64(1), true)
	seeds.AddRow(uint(0), fuzzing.KindNil, "", int64(0), false)
	seeds.AddRow(uint(1), fuzzing.KindMap, "nested", int64(0), false)

	configtest.FuzzSection(f, "giga_executor", gigaExecutorSection(), seeds)

	f.Fuzz(func(t *testing.T, keyIdx uint, kind uint8, s string, n int64, b bool) {
		spec := configtest.Pick(gigaKeys, keyIdx)
		configtest.CheckRow(t, "giga_executor", readGiga, spec, fuzzing.ConfigValue(kind, s, n, b))
	})
}

// TestGigaExecutorSection is this section's whole coverage, in one call.
//
// One call rather than six. Six call sites are six things a later edit can remove one of while
// every remaining check still passes, which is why a record of the wiring had to exist. What each
// check asserts is named by its subtest.
func TestGigaExecutorSection(t *testing.T) {
	configtest.CheckSection(t, "giga_executor", gigaExecutorSection())
}

// gigaExecutorSection states this section for both the test and the fuzz target.
//
// Shared so the two cannot describe the same section differently, which is how a fuzz target ends
// up driving a manifest the tests never checked.
func gigaExecutorSection() configtest.Section {
	return configtest.Section{
		Read:     readGiga,
		Defaults: config.DefaultConfig,

		Keys: gigaKeys,
	}
}
