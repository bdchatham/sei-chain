package tendermint_test

import (
	"strings"
	"testing"

	"github.com/sei-protocol/sei-chain/config/registry"
	_ "github.com/sei-protocol/sei-chain/config/tendermint"
)

// TestNoSectionDeclaresTheRootDirectory is the one exclusion that would break a node.
//
// Five Tendermint sub-structs carry a RootDir field tagged home, and Config.SetRoot fills each after the
// file is decoded. Declaring one would put an empty root in an operator's file, and delivering that would
// leave the node unable to find its own data directory. The check is over every declared key rather than
// the sections declared today, so the next section carrying that field fails here instead of in a boot.
func TestNoSectionDeclaresTheRootDirectory(t *testing.T) {
	for _, key := range registry.Keys() {
		if strings.HasSuffix(key, ".home") || key == "home" {
			t.Errorf("%q is declared. It is the node's root directory, which SetRoot fills after the "+
				"decode, so an operator's file would carry an empty one and delivering it would lose "+
				"every path derived from it. Exclude it at registration with the reason", key)
		}
	}
}

// TestTheDeclaredTendermintSectionsAreAllDecodedNotLookedUp keeps the delivery wired to all of them.
//
// A section of config.toml that is declared and not marked as decoded is resolved, installed into the
// application options, and read by nothing. Its keys would be settings an operator can write in sei.toml
// that never reach the node, which is the defect this whole workstream exists to remove.
func TestTheDeclaredTendermintSectionsAreAllDecodedNotLookedUp(t *testing.T) {
	sections := []string{"instrumentation", "self-remediation", "priv-validator", "statesync",
		"mempool", "rpc", "p2p", "consensus", "tx-index", "node"}
	for _, name := range sections {
		if _, ok := registry.Lookup(name); !ok {
			t.Errorf("%s is not registered", name)
			continue
		}
		if !registry.DecodedNotLookedUp(name) {
			t.Errorf("%s is declared and not marked as decoded, so its resolved values are installed "+
				"where nothing reads them and every key in it is inert", name)
		}
	}
}

func TestRegisteringTheTendermintSectionsProducedNoDefect(t *testing.T) {
	for _, defect := range registry.Defects() {
		switch defect.Section {
		case "instrumentation", "self-remediation", "priv-validator", "statesync", "mempool", "rpc",
			"p2p", "consensus", "tx-index", "node":
			t.Errorf("registering %s was refused: %v\n\nThe section is absent from the registry, so "+
				"every key it declares silently reads from the legacy path instead",
				defect.Section, defect.Err)
		}
	}
}
