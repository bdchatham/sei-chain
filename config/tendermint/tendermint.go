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

// InstrumentationSectionName is the metrics listener config.toml carries.
const InstrumentationSectionName = "instrumentation"

// Registration puts the config.toml sections in the registry.
//
// tmcfg's types are registered directly rather than through a schema written here, because their
// mapstructure tags are what the decode itself reads. That is the opposite of the app.toml sections,
// where the tags are inert and the readers use their own literals: here a tag that named something else
// would not be drift, it would be the key.
func init() {
	registry.RegisterSection(InstrumentationSectionName, &tmcfg.InstrumentationConfig{},
		instrumentationBaseline)

	registry.DeclareDecodedNotLookedUp(InstrumentationSectionName,
		"decoded into tendermint config.InstrumentationConfig by the boot's own handler, which reads "+
			"config.toml once into a struct; nothing looks these keys up afterwards")
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
