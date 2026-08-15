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
)

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

	registry.RegisterSectionExcluding(PrivValidatorSectionName, &tmcfg.PrivValidatorConfig{},
		privValidatorBaseline, map[string]string{
			PrivValidatorSectionName + ".home": rootDirectoryIsNotConfiguration,
		})

	for section, into := range map[string]string{
		InstrumentationSectionName: "config.InstrumentationConfig",
		SelfRemediationSectionName: "config.SelfRemediationConfig",
		PrivValidatorSectionName:   "config.PrivValidatorConfig",
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
