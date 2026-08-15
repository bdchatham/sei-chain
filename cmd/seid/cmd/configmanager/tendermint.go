package configmanager

import (
	"log/slog"
	"sort"
	"strings"

	"github.com/spf13/viper"

	"github.com/sei-protocol/sei-chain/config/registry"
	"github.com/sei-protocol/sei-chain/sei-cosmos/server"
	tmcfg "github.com/sei-protocol/sei-chain/sei-tendermint/config"
)

// deliverDecodedSections puts the resolved values of the decoded sections into the Tendermint config.
//
// Installing a value into the application options is the whole delivery for a section a reader looks up
// key by key. It is no delivery at all for config.toml, which the boot's handler reads once into a
// struct before this runs. Those values are decoded into that struct instead, which is the same
// mechanism the handler used and therefore the same casts, the same tags and the same hooks.
//
// Only the declared keys are decoded, out of a container built for them. Decoding the whole source again
// would be a different input from the one the handler decoded: it merges app.toml into that source
// afterwards, and its flag binding copies configuration values into flags as text, which viper then
// ranks above the file. A duration an operator wrote as a bare number decodes on the handler's pass and
// is refused on a second one. Narrowing the input removes that, and removes the dependency on app.toml
// and config.toml never using the same key name.
//
// Nothing here can stop a node starting, which is the one promise this manager makes.
func deliverDecodedSections(ctx *server.Context, resolved registry.Resolved, log *slog.Logger) {
	values := registry.KeysDecodedNotLookedUp(resolved)
	if len(values) == 0 {
		return
	}
	if ctx.Config == nil {
		log.Warn("no tendermint configuration to deliver into; these keys read as they always have",
			"count", len(values))
		return
	}

	source := viper.New()
	for key, value := range values {
		source.Set(key, value)
	}

	// Decoded into a throwaway first. mapstructure gathers errors and keeps going, so a value it refuses
	// leaves the target half written, with no result to publish and no way back to the one the node had.
	// The throwaway is the same type, so what decodes into it decodes into the real one.
	if err := source.Unmarshal(tmcfg.DefaultConfig()); err != nil {
		log.Warn("resolved tendermint settings could not be decoded; every one of them reads as it "+
			"always has", "keys", strings.Join(sortedKeys(values), ","), "err", err)
		return
	}
	if err := source.Unmarshal(ctx.Config); err != nil {
		// Unreachable through the throwaway above, which decodes the same values into the same type. It
		// is reported rather than ignored because reaching it would mean that reasoning is wrong.
		log.Warn("resolved tendermint settings decoded into a fresh configuration and not into this "+
			"node's; some of them may have been applied", "err", err)
		return
	}
	log.Info("resolved tendermint settings applied", "count", len(values))
}

// sortedKeys returns a map's keys in a fixed order, so a log line does not vary between runs.
func sortedKeys(values map[string]any) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
