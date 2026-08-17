package blocktest_test

import (
	"testing"

	"github.com/sei-protocol/sei-chain/testutil/configtest"
	"github.com/sei-protocol/sei-chain/testutil/fuzzing"
	"github.com/sei-protocol/sei-chain/x/evm/blocktest"
)

// ethBlockTestKeys is the [eth_blocktest] section's read-site manifest. Both keys
// are guarded and checked.
//
// This section's name exists twice under two spellings, and the harmless one is the
// generated file. The app.toml template hardcodes the header [eth_blocktest] and both
// eth_blocktest_* key names as literal text, resolving only the values through
// {{ .ETHBlockTest.Field }}, which text/template walks by Go field name. So a generated
// app.toml carries exactly the spellings the reader below looks up and round-trips
// correctly.
//
// The divergent spelling is the mapstructure tag on CustomAppConfig.ETHBlockTest,
// eth_block_test, which names a section the template never writes and the reader never
// reads. It is inert rather than dangerous, and it is worth recording because it is the
// spelling a manager that drove generation from the struct tags — the obvious way to
// build one — would emit, producing a file this reader ignores entirely.
var ethBlockTestKeys = []configtest.KeySpec{
	{
		Key: "eth_blocktest.eth_blocktest_enabled", Path: "Enabled", Cast: configtest.CastBool,
		Checked: true,
		Why:     "default false; the spelling both the template and the reader use",
	},
	{
		Key: "eth_blocktest.eth_blocktest_test_data_path", Path: "TestDataPath", Cast: configtest.CastString,
		Checked: true,
		Why:     "default is a tilde path, expanded by the consumer rather than here",
	},
}

func readETHBlockTest(opts configtest.AppOpts) (any, error) { return blocktest.ReadConfig(opts) }

func FuzzReadConfig(f *testing.F) {
	seeds := configtest.NewSeeds(f, fuzzing.ConfigValue)
	seeds.AddRow(uint(0), fuzzing.KindBool, "true", int64(1), true)
	seeds.AddRow(uint(1), fuzzing.KindString, "~/testdata/", int64(0), false)
	seeds.AddRow(uint(0), fuzzing.KindString, "on", int64(0), false)
	seeds.AddRow(uint(1), fuzzing.KindAnySlice, "", int64(0), false)
	seeds.AddRow(uint(0), fuzzing.KindNil, "", int64(0), false)

	configtest.FuzzSection(f, "eth_blocktest", ethBlockTestSection(), seeds)

	f.Fuzz(func(t *testing.T, keyIdx uint, kind uint8, s string, n int64, b bool) {
		spec := configtest.Pick(ethBlockTestKeys, keyIdx)
		configtest.CheckRow(t, "eth_blocktest", readETHBlockTest, spec, fuzzing.ConfigValue(kind, s, n, b))
	})
}

// TestStructTagSpellingIsInert records that the mapstructure spelling on
// CustomAppConfig.ETHBlockTest (eth_block_test) is not the one the reader looks up
// (eth_blocktest), so a value written under it resolves to nothing.
//
// The generated app.toml is not affected: the template writes the reader's spelling. What
// this pins is that the struct tag cannot be treated as the section name, which is the
// mistake a manager generating config from the tags would make.
func TestStructTagSpellingIsInert(t *testing.T) {
	cfg, err := blocktest.ReadConfig(configtest.AppOpts{
		"eth_block_test.eth_blocktest_enabled": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Enabled {
		t.Fatal("the eth_block_test spelling resolved a value; if the tag and the read prefix " +
			"were unified on purpose, update this test and ship a migration")
	}
}

// TestETHBlockTestSection is this section's whole coverage, in one call.
//
// One call rather than six. Six call sites are six things a later edit can remove one of while
// every remaining check still passes, which is why a record of the wiring had to exist. What each
// check asserts is named by its subtest.
func TestETHBlockTestSection(t *testing.T) {
	configtest.CheckSection(t, "eth_blocktest", ethBlockTestSection())
}

// ethBlockTestSection states this section for both the test and the fuzz target.
//
// Shared so the two cannot describe the same section differently, which is how a fuzz target ends
// up driving a manifest the tests never checked.
func ethBlockTestSection() configtest.Section {
	return configtest.Section{
		Read:     readETHBlockTest,
		Defaults: blocktest.DefaultConfig,

		Keys: ethBlockTestKeys,
	}
}
