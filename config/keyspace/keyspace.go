// Package keyspace states which configuration sections a seid binary declares.
//
// A section reaches the registry by its owning package's initialisation, which happens only if
// something imports that package. That makes the key space a consequence of the import graph: a
// section whose owner nothing imports is absent from every diagnostic, and absent silently, since an
// undeclared key is left to the machinery that already answers it. Two of the sections were in fact
// reaching the registry only because a command package imported their owner for an unrelated reason,
// and the verb packages saw a fraction of the set.
//
// Importing this package is what makes the key space the same everywhere. The list below is what makes
// it stated, so a section that stops registering is a failure rather than a shorter report.
package keyspace

import (
	"sort"

	"github.com/sei-protocol/sei-chain/config/registry"

	_ "github.com/sei-protocol/sei-chain/admin"
	_ "github.com/sei-protocol/sei-chain/app"
	_ "github.com/sei-protocol/sei-chain/config/cosmosbase"
	_ "github.com/sei-protocol/sei-chain/evmrpc/config"
	_ "github.com/sei-protocol/sei-chain/giga/executor/config"
	_ "github.com/sei-protocol/sei-chain/sei-db/config"
	_ "github.com/sei-protocol/sei-chain/sei-wasmd/x/wasm"
	_ "github.com/sei-protocol/sei-chain/x/evm/blocktest"
	_ "github.com/sei-protocol/sei-chain/x/evm/querier"
	_ "github.com/sei-protocol/sei-chain/x/evm/replay"
)

// Names are the configuration sections a seid binary declares, sorted.
//
// Written out rather than read back from the registry. Reading it back would answer with whatever
// the import graph produced, which is the thing this list exists to check.
var Names = []string{
	"admin_server",
	"api",
	"base",
	"eth_blocktest",
	"eth_replay",
	"evm",
	"evm_query",
	"genesis",
	"giga_executor",
	"grpc",
	"light_invariance",
	"receipt-store",
	"state-commit",
	"state-store",
	"state-sync",
	"telemetry",
	"wasm",
}

// Drift returns every way the registered key space differs from the set named above, sorted, as lines
// ready to print. Empty is the only correct answer.
//
// One answer covering both directions, because they are one question asked from either end and a caller
// checking only one has half a guard. A section named here and not registered has its keys resolving
// through the machinery that answered them before the registry existed. A section registered and not
// named here entered the key space without being written down: not necessarily wrong, but nothing has
// said it should be there, so nothing would notice it leaving again.
func Drift() []string {
	registered := make([]string, 0, len(registry.Sections()))
	for _, section := range registry.Sections() {
		registered = append(registered, section.Name)
	}
	return driftBetween(Names, registered)
}

// driftBetween compares two sets of section names, in both directions.
//
// Separate from Drift so it can be driven with sets that differ. Drift reads the registry, where no
// drift is the only correct answer, so a test going through it cannot show that this detects anything:
// with both sets equal, a comparison that reported nothing at all would pass.
func driftBetween(named, registered []string) []string {
	inRegistry := map[string]bool{}
	for _, name := range registered {
		inRegistry[name] = true
	}
	isNamed := map[string]bool{}
	for _, name := range named {
		isNamed[name] = true
	}

	var drift []string
	for _, name := range named {
		if !inRegistry[name] {
			drift = append(drift, "named here and not registered: "+name)
		}
	}
	for _, name := range registered {
		if !isNamed[name] {
			drift = append(drift, "registered and not named here: "+name)
		}
	}
	sort.Strings(drift)
	return drift
}
