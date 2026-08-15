// Package tendermint declares the configuration sections config.toml owns.
//
// These sections reach their reader differently from every other one in the key space. A section of
// app.toml is read by a lookup, one key at a time, whenever the code that needs it runs. config.toml is
// read once, by decoding the whole file into a struct before the application is built, and nothing looks
// a key up afterwards. So a resolved value installed into the application options reaches nothing here,
// and the boot delivers these by decoding them a second time into the struct it already built.
//
// That difference is declared rather than inferred, because it decides both how a value is delivered and
// why a census of lookups never sees these keys.
package tendermint

import (
	"github.com/sei-protocol/sei-chain/config/registry"
	tmcfg "github.com/sei-protocol/sei-chain/sei-tendermint/config"
)

// The sections config.toml carries.
const (
	InstrumentationSectionName = "instrumentation"
	SelfRemediationSectionName = "self-remediation"
	PrivValidatorSectionName   = "priv-validator"
	StateSyncSectionName       = "statesync"
	MempoolSectionName         = "mempool"
	RPCSectionName             = "rpc"
	P2PSectionName             = "p2p"
	ConsensusSectionName       = "consensus"
	TxIndexSectionName         = "tx-index"
	NodeSectionName            = "node"
)

// optionalWithNoDefault is why a key whose upstream default is a nil pointer is not declared.
//
// The registry gives every declared key a baseline, because a complete sei.toml stamps one for each. A
// field left nil by its own default has none to stamp, and choosing one here would invent a value
// upstream deliberately did not state. Such a key stays on the legacy path, where a hand-written value
// still reaches the decode.
const optionalWithNoDefault = "optional upstream, left nil by its own default. The registry stamps a " +
	"baseline for every declared key and this field has none, so declaring it would mean inventing the " +
	"value upstream chose not to state"

// rootDirectoryIsNotConfiguration is why the home key of a Tendermint sub-struct is never declared.
//
// Five of them carry a RootDir field tagged home, and Config.SetRoot fills every one after the file is
// decoded. It is derived state, not something an operator writes: the template never renders it, its
// default is the empty string, and delivering that would leave a node unable to find its own data
// directory.
const rootDirectoryIsNotConfiguration = "the node's root directory, which Config.SetRoot fills after " +
	"the file is decoded. The template never writes it and its default is empty, so declaring it would " +
	"put an empty root in an operator's file and delivering that would lose every path derived from it"

// Registration puts the config.toml sections in the registry.
//
// tmcfg's types are registered directly rather than through a schema written here, because their
// mapstructure tags are what the decode itself reads. That is the opposite of the app.toml sections,
// where the tags are inert and the readers use their own literals: here a tag that named something else
// would not be drift, it would be the key.
func init() {
	registry.RegisterSection(InstrumentationSectionName, &tmcfg.InstrumentationConfig{},
		instrumentationBaseline)

	registry.RegisterSection(SelfRemediationSectionName, &tmcfg.SelfRemediationConfig{},
		selfRemediationBaseline)

	// The one declared section carrying a list. mapstructure decodes a slice by replacing it to the
	// input's length, so a written rpc-servers replaces the node's servers rather than adding to them,
	// which is what an operator writing a list means.
	registry.RegisterSection(StateSyncSectionName, &tmcfg.StateSyncConfig{}, stateSyncBaseline)

	registry.RegisterSectionExcluding(PrivValidatorSectionName, &tmcfg.PrivValidatorConfig{},
		privValidatorBaseline, map[string]string{
			PrivValidatorSectionName + ".home": rootDirectoryIsNotConfiguration,
		})

	registry.RegisterSectionExcluding(MempoolSectionName, &tmcfg.MempoolConfig{},
		mempoolBaseline, map[string]string{
			MempoolSectionName + ".home": rootDirectoryIsNotConfiguration,
		})

	registry.RegisterSectionExcluding(RPCSectionName, &tmcfg.RPCConfig{},
		rpcBaseline, map[string]string{
			RPCSectionName + ".home": rootDirectoryIsNotConfiguration,
		})

	registry.RegisterSectionExcluding(P2PSectionName, &tmcfg.P2PConfig{},
		p2pBaseline, map[string]string{
			P2PSectionName + ".home":                     rootDirectoryIsNotConfiguration,
			P2PSectionName + ".max-outbound-connections": optionalWithNoDefault,
		})

	registry.RegisterSection(TxIndexSectionName, &tmcfg.TxIndexConfig{}, txIndexBaseline)

	// The keys at the top of config.toml, which carry no section of their own. The name is for lookups
	// and reports and is not part of any key, the same way base names app.toml's root keys.
	registry.RegisterRootKeysExcluding(NodeSectionName, &tmcfg.BaseConfig{}, nodeBaseline,
		nodeNotConfiguration())

	// The moniker's default is the machine's host name, so two nodes of the same release resolve
	// different ones and a record of the key space cannot hold the value.
	registry.DeclareHostDerived(NodeSectionName, "moniker", "the host name of the machine that asked")

	registry.RegisterSectionExcluding(ConsensusSectionName, &tmcfg.ConsensusConfig{},
		consensusBaseline, consensusNotConfiguration())

	for section, into := range map[string]string{
		InstrumentationSectionName: "config.InstrumentationConfig",
		SelfRemediationSectionName: "config.SelfRemediationConfig",
		PrivValidatorSectionName:   "config.PrivValidatorConfig",
		StateSyncSectionName:       "config.StateSyncConfig",
		MempoolSectionName:         "config.MempoolConfig",
		RPCSectionName:             "config.RPCConfig",
		P2PSectionName:             "config.P2PConfig",
		ConsensusSectionName:       "config.ConsensusConfig",
		TxIndexSectionName:         "config.TxIndexConfig",
		NodeSectionName:            "config.BaseConfig",
	} {
		registry.DeclareDecodedNotLookedUp(section,
			"decoded into tendermint "+into+" by the boot's own handler, which reads config.toml once "+
				"into a struct; nothing looks these keys up afterwards")
	}
}

// instrumentationBaseline is what this section resolves to for a node that has written nothing.
//
// The upstream defaults, which is what a node's config.toml is initialised with. The same values for
// every mode: whether a node serves its metrics listener is an operator's decision about monitoring, and
// no node mode implies one.
//
// Read out of the upstream default rather than restated, so a changed default moves both at once. The
// value is dereferenced because the section registers the struct and not the pointer, and a baseline
// that resolved to a pointer would compare unequal to every value written under it.
func instrumentationBaseline(registry.Mode) any {
	return *tmcfg.DefaultInstrumentationConfig()
}

// selfRemediationBaseline is what this section resolves to for a node that has written nothing.
//
// The upstream defaults, and the same for every mode. Every one of these is a window a node waits before
// restarting itself, which is an operator's judgement about their own network rather than something a
// node's role implies.
func selfRemediationBaseline(registry.Mode) any {
	return *tmcfg.DefaultSelfRemediationConfig()
}

// privValidatorBaseline is what this section resolves to for a node that has written nothing.
//
// The upstream defaults, and the same for every mode. Where a node keeps its signing key and whether it
// reaches a remote signer are decisions about how an operator holds their key material, and a validator
// makes them no differently from a full node; what differs is whether the key signs anything.
func privValidatorBaseline(registry.Mode) any {
	return *tmcfg.DefaultPrivValidatorConfig()
}

// stateSyncBaseline is what this section resolves to for a node that has written nothing.
//
// The upstream defaults, and the same for every mode. Whether a node catches up from a snapshot and
// which servers it trusts to verify one are decisions about how an operator brings a node into a
// network, not about what kind of node it becomes afterwards.
//
// Distinct from the state-sync section of app.toml, which is the same idea from the other side: that one
// says whether this node serves snapshots, this one says whether it consumes them.
func stateSyncBaseline(registry.Mode) any {
	return *tmcfg.DefaultStateSyncConfig()
}

// mempoolBaseline is what this section resolves to for a node that has written nothing.
//
// The upstream defaults, and the same for every mode. How many transactions a node holds and how long it
// holds them are decisions about the memory an operator is willing to spend, which a validator and a
// full node make on the same grounds.
//
// Two of these keys are not in the rendered file and are declared anyway. A key the template omits is
// still one the decode reads and an operator can write by hand, and leaving it undeclared is what makes
// a hand-written value disappear on migration.
func mempoolBaseline(registry.Mode) any {
	return *tmcfg.DefaultMempoolConfig()
}

// rpcBaseline is what this section resolves to for a node that has written nothing.
//
// The upstream defaults, and the same for every mode. Which interfaces a node serves and to whom is an
// operator's decision about exposure, and a node's role does not imply one: a validator behind a proxy
// and a public full node reach opposite answers for reasons this cannot know.
func rpcBaseline(registry.Mode) any {
	return *tmcfg.DefaultRPCConfig()
}

// p2pBaseline is what this section resolves to for a node that has written nothing.
//
// The upstream defaults, and the same for every mode. Which peers a node dials and how hard it works to
// stay connected are decisions about the network an operator is joining. A seed node's difference is
// expressed by the peers it is given rather than by a different default here.
func p2pBaseline(registry.Mode) any {
	return *tmcfg.DefaultP2PConfig()
}

// txIndexBaseline is what this section resolves to for a node that has written nothing.
//
// The upstream defaults, and the same for every mode. Whether a node indexes its transactions is a
// decision about the queries it will be asked to serve.
func txIndexBaseline(registry.Mode) any {
	return *tmcfg.DefaultTxIndexConfig()
}

// consensusBaseline is what this section resolves to for a node that has written nothing.
//
// The upstream defaults, and the same for every mode. Every remaining key here is a timing an operator
// tunes against their own network, and the ones that change how a node votes are the unsafe overrides,
// which a node running them has deliberately turned on.
func consensusBaseline(registry.Mode) any {
	return *tmcfg.DefaultConsensusConfig()
}

// consensusNotConfiguration names the fields of the consensus struct an operator is not given.
//
// The eight deprecated timeouts are the bulk of it. They are tagged, so the decode still reads them, and
// they are typed as pointers to anything so that a value written under one can be reported rather than
// silently applied. A key whose whole purpose is to be refused is not one to stamp into a new file.
func consensusNotConfiguration() map[string]string {
	const deprecated = "deprecated upstream and read only so a value written under it can be reported. " +
		"Stamping it into a new file would carry a setting forward whose only behaviour is to be refused"

	out := map[string]string{
		ConsensusSectionName + ".home":                                  rootDirectoryIsNotConfiguration,
		ConsensusSectionName + ".unsafe-bypass-commit-timeout-override": optionalWithNoDefault,
	}
	for _, key := range []string{
		"timeout-propose", "timeout-propose-delta", "timeout-prevote", "timeout-prevote-delta",
		"timeout-precommit", "timeout-precommit-delta", "timeout-commit", "skip-timeout-commit",
	} {
		out[ConsensusSectionName+"."+key] = deprecated
	}
	return out
}

// nodeBaseline is what the node-wide config.toml keys resolve to for a node that has written nothing.
//
// The upstream defaults, and the same for every mode. The node mode is among these keys and is not
// varied by it: sei.toml records the mode separately and a diagnostic compares the two, so resolving one
// from the other would make that comparison compare a thing with itself.
func nodeBaseline(registry.Mode) any {
	return tmcfg.DefaultBaseConfig()
}

// nodeNotConfiguration names the fields of the root config an operator is not given.
//
// log-level is the one worth reading twice. It is live and an operator writes it, but the boot reads it
// off the struct and hands it to the logger before the resolved values are delivered, so a value
// delivered afterwards would move the field and not the logging. A key that appears to take and does not
// is the defect this whole key space exists to remove, so it stays on the legacy path until the delivery
// can re-apply it.
func nodeNotConfiguration() map[string]string {
	const deprecated = "deprecated upstream, where the flag that carries it is marked deprecated and the " +
		"comment says it no longer has any effect"

	return map[string]string{
		"home": rootDirectoryIsNotConfiguration,
		"log-level": "read off the struct by the boot's own handler and handed to the logger before " +
			"resolved values are delivered, so delivering it would move the field and not the logging",
		"abci":         deprecated,
		"filter-peers": deprecated,
	}
}
