package configcli

import (
	"strings"

	"github.com/spf13/pflag"
	"go.opentelemetry.io/otel/sdk/trace"

	"github.com/sei-protocol/sei-chain/sei-cosmos/server"
)

// StartFlagDefaults reads the default of every flag the start command binds.
//
// A bound flag's default is a layer of a node's configuration, and the lowest one: the resolution
// reaches it only when nothing else answered, so a key absent from app.toml and config.toml resolves
// to it. A node whose files omit such a key has been running that default all along, which is why an
// adopted file has to write it rather than whatever the key would resolve to with no flag bound.
//
// Read off the start command itself rather than restated here, so the two cannot disagree about what
// a default is. The command is built and never run; only its flag set is read.
func StartFlagDefaults(home string) func(key string) (string, bool) {
	defaults := map[string]string{}
	server.StartCmd(nil, home, []trace.TracerProviderOption{}).Flags().
		VisitAll(func(f *pflag.Flag) {
			defaults[strings.ToLower(f.Name)] = f.DefValue
		})
	return func(key string) (string, bool) {
		text, bound := defaults[strings.ToLower(key)]
		return text, bound
	}
}
