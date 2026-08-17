package querier_test

import (
	"testing"

	"github.com/sei-protocol/sei-chain/testutil/configtest"
	"github.com/sei-protocol/sei-chain/testutil/fuzzing"
	"github.com/sei-protocol/sei-chain/x/evm/querier"
)

// evmQueryKeys is the [evm_query] section's read-site manifest: one key, guarded
// and checked.
//
// The default of 300000 is the same literal the app.toml template renders for
// [wasm] query_gas_limit, and both are unrelated to the wasmd in-code default of
// 3,000,000. Pinning the value here means a change to one cannot quietly be
// mistaken for a change to the other.
var evmQueryKeys = []configtest.KeySpec{
	{
		Key: "evm_query.evm_query_gas_limit", Path: "GasLimit", Cast: configtest.CastUint64,
		Checked: true,
		Why:     "default 300000; bounds gas for EVM state queries served from the Cosmos querier",
	},
}

func readEVMQuery(opts configtest.AppOpts) (any, error) { return querier.ReadConfig(opts) }

// FuzzReadConfig pins the [evm_query] gas limit against arbitrary raw values.
//
// The unsigned cast is the interesting part. cast refuses a negative into an
// unsigned conversion, and this read is checked, so an operator who writes -1
// expecting "unlimited" gets a boot failure naming the key rather than a limit of
// 0 or a wrapped 2^64. Refusing is the safe direction here — a gas limit of 0
// would fail every EVM state query on a node that came up clean.
func FuzzReadConfig(f *testing.F) {
	seeds := configtest.NewSeeds(f, fuzzing.ConfigValue)
	seeds.Add(fuzzing.KindInt64, "", int64(300000), false)
	seeds.Add(fuzzing.KindNumericString, "", int64(500000), false)
	seeds.Add(fuzzing.KindInt64, "", int64(-1), false)
	seeds.Add(fuzzing.KindString, "not-a-number", int64(0), false)
	seeds.Add(fuzzing.KindNil, "", int64(0), false)
	seeds.Add(fuzzing.KindFloat64, "", int64(7), false)

	configtest.FuzzSection(f, "evm_query", evmQuerySection(), seeds)

	f.Fuzz(func(t *testing.T, kind uint8, s string, n int64, b bool) {
		configtest.CheckRow(t, "evm_query", readEVMQuery, evmQueryKeys[0], fuzzing.ConfigValue(kind, s, n, b))
	})
}

// TestEVMQuerySection is this section's whole coverage, in one call.
//
// One call rather than six. Six call sites are six things a later edit can remove one of while
// every remaining check still passes, which is why a record of the wiring had to exist. What each
// check asserts is named by its subtest.
func TestEVMQuerySection(t *testing.T) {
	configtest.CheckSection(t, "evm_query", evmQuerySection())
}

// evmQuerySection states this section for both the test and the fuzz target.
//
// Shared so the two cannot describe the same section differently, which is how a fuzz target ends
// up driving a manifest the tests never checked.
func evmQuerySection() configtest.Section {
	return configtest.Section{
		Read:     readEVMQuery,
		Defaults: querier.DefaultConfig,

		Keys: evmQueryKeys,
	}
}
