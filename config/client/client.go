// Package client declares the configuration section client.toml owns.
//
// These five settings describe how this machine talks to a chain rather than how a node runs one: which
// chain, which endpoint, which keyring, how to print, and how to broadcast. That difference is why the
// file exists separately, and it is not a reason for a second file. Any machine running any seid command
// gets a client.toml, because the reader writes one when it finds none, so a node has all three files
// whether or not the five settings mean anything on it.
//
// They are read once, before the application is built, by a reader that then constructs a keyring and an
// RPC client from them. So this section is delivered the way config.toml's are, by putting the resolved
// values where that reader looks rather than by installing them somewhere it never consults.
package client

import (
	"github.com/sei-protocol/sei-chain/config/registry"
	clientconfig "github.com/sei-protocol/sei-chain/sei-cosmos/client/config"
)

// SectionName is what these keys are looked up and reported under.
//
// The keys carry no prefix: they sit at the root of client.toml and are read as "chain-id" and the rest.
// The name is for lookups and reports, the same way base names app.toml's root keys.
const SectionName = "client"

// Registration puts the client section in the registry.
//
// clientconfig.ClientConfig is registered directly, because its mapstructure tags are what the decode
// itself reads. A tag naming something else here would not be drift, it would be the key.
func init() {
	registry.RegisterRootKeys(SectionName, &clientconfig.ClientConfig{}, baseline)

	registry.DeclareDecodedNotLookedUp(SectionName,
		"decoded into cosmos client config.ClientConfig, which is read once before the application is "+
			"built and then used to construct a keyring and an RPC client; nothing looks these keys up "+
			"afterwards")

	// Two of these answered to a variable before they were declared, and three did not. The reader's own
	// viper prefixes with SEI and translates nothing, so it looks for SEI_CHAIN-ID for chain-id, which a
	// shell cannot set. Only the two keys with no hyphen were ever reachable, and only those two need to
	// keep working.
	//
	// The other three gain an environment variable here for the first time, because a declared key's
	// variable is derived from the key with its punctuation replaced.
	// The chain a node joins is the one setting here where a typed flag disagreeing with the file is a
	// mistake rather than an override. Declaring the key put the upstream comparison out of reach, since
	// the flag now feeds the value that check reads.
	registry.RefuseConflictingFlag(SectionName, "chain-id",
		"a flag naming one chain and a file naming another is a node about to join a network nobody "+
			"chose, and the two cannot both be what the operator meant")

	for key, was := range map[string]string{"node": "SEI_NODE", "output": "SEI_OUTPUT"} {
		registry.DeclareLegacyEnvName(SectionName, key, was,
			"the variable this key answered to before it was declared, when the client reader's own "+
				"viper resolved it under a different prefix")
	}
}

// baseline is what this section resolves to for a machine that has written nothing.
//
// The upstream defaults, and the same for every mode. Which chain this machine talks to and how it signs
// are decisions about the machine rather than about the node it may or may not also be running, and a
// validator makes them no differently from a laptop.
func baseline(registry.Mode) any {
	return *clientconfig.DefaultClientConfig()
}
