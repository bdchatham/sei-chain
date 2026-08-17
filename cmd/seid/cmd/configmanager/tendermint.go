package configmanager

import (
	"log/slog"
	"sort"
	"strings"

	"github.com/spf13/viper"

	"github.com/sei-protocol/seilog"

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
	applyLogLevel(values, log)
}

// logLevelKey is the one delivered setting the struct is not the end of.
const logLevelKey = "log-level"

// applyLogLevel hands a delivered log level to the logger, which the struct alone does not reach.
//
// The boot's handler reads the level off the struct and sets it before any of this runs, so a value
// decoded afterwards moves the field and changes no logging. A setting that appears to take and does not
// is the thing this key space exists to remove, so the level is applied here rather than the key being
// left undeclared.
//
// Which value arrives is already decided. The resolution ranks a flag over the environment over the file,
// which is the order the handler reaches for by hand, so this applies whatever won rather than choosing
// again.
//
// A level that does not parse is reported and skipped. The handler refuses a boot over one; this manager
// may not, and the node keeps the level it already had.
func applyLogLevel(values map[string]any, log *slog.Logger) {
	value, delivered := values[logLevelKey]
	if !delivered {
		return
	}
	text, isText := value.(string)
	if !isText {
		log.Warn("the resolved log level is not text; the node keeps the level it already had",
			"value", value)
		return
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(text)); err != nil {
		log.Warn("the resolved log level cannot be read; the node keeps the level it already had",
			"level", text, "err", err)
		return
	}
	seilog.SetDefaultLevel(level, true)
	log.Info("resolved log level applied", "level", text)
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
