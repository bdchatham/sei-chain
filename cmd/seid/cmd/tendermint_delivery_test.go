package cmd

import (
	"testing"

	"github.com/sei-protocol/sei-chain/testutil/configtest"
)

// The second delivery, end to end through a real boot.
//
// A section of app.toml is delivered by installing its resolved value into the application options,
// where a reader looks it up. config.toml is read once into a struct before any of that, so installing
// reaches nothing and the value has to be decoded into the struct instead. These drive that path the way
// an operator reaches it: a value in sei.toml, and the setting the node runs.

// TestASeiTomlValueReachesTheTendermintConfig is the property the whole config.toml migration rests on.
func TestASeiTomlValueReachesTheTendermintConfig(t *testing.T) {
	configtest.Isolate(t)
	ctx := bootWithSeiToml(t, "schema_version = 1\nnode_mode = \"validator\"\n\n"+
		"[instrumentation]\nprometheus = true\nmax-open-connections = 41\n")

	if ctx.Config == nil {
		t.Fatal("the boot produced no tendermint configuration")
	}
	if !ctx.Config.Instrumentation.Prometheus {
		t.Error("sei.toml turned the metrics listener on and the node's configuration says it is off. " +
			"The value was resolved and installed into the application options, which nothing reading " +
			"config.toml ever consults")
	}
	if got := ctx.Config.Instrumentation.MaxOpenConnections; got != 41 {
		t.Errorf("sei.toml set max-open-connections to 41 and the node runs %d", got)
	}
}

// TestAnUnwrittenTendermintKeyKeepsWhatTheNodeHad is the other half of the same property.
//
// Every key of a declared section resolves, so the delivery writes all of them whether or not the
// operator named any. A key they did not write has to arrive at the value it already had, or declaring
// the section would move settings nobody chose.
func TestAnUnwrittenTendermintKeyKeepsWhatTheNodeHad(t *testing.T) {
	configtest.Isolate(t)
	ctx := bootWithSeiToml(t, "schema_version = 1\nnode_mode = \"validator\"\n\n"+
		"[instrumentation]\nprometheus = true\n")

	// Written, so it moves.
	if !ctx.Config.Instrumentation.Prometheus {
		t.Fatal("the written key did not arrive, so this test cannot tell the two cases apart")
	}
	// Not written, so it keeps the upstream default the handler's own decode left it at.
	if got := ctx.Config.Instrumentation.PrometheusListenAddr; got != ":26660" {
		t.Errorf("an unwritten key arrived as %q, want the :26660 the node already had. Declaring a "+
			"section must not move a setting the operator did not name", got)
	}
	if got := ctx.Config.Instrumentation.Namespace; got != "tendermint" {
		t.Errorf("an unwritten key arrived as %q, want the tendermint the node already had", got)
	}
}

// TestATendermintValueTheNodeRefusesLeavesTheConfigurationAlone holds the one promise this manager makes.
//
// A value that cannot be decoded must not leave the configuration half written. mapstructure gathers
// errors and keeps going, so the guard is that nothing is decoded into the node's own configuration
// until the same values have decoded into a throwaway of the same type.
func TestATendermintValueTheNodeRefusesLeavesTheConfigurationAlone(t *testing.T) {
	configtest.Isolate(t)
	ctx := bootWithSeiToml(t, "schema_version = 1\nnode_mode = \"validator\"\n\n"+
		"[instrumentation]\nprometheus = true\nmax-open-connections = \"not a number\"\n")

	if ctx.Config == nil {
		t.Fatal("a value the decode refuses stopped the boot; this manager may not do that")
	}
	if got := ctx.Config.Instrumentation.MaxOpenConnections; got != 3 {
		t.Errorf("max-open-connections is %d after a refused decode, want the 3 the node had. A "+
			"partially applied decode leaves settings nobody chose and nothing to compare against", got)
	}
	if ctx.Config.Instrumentation.Prometheus {
		t.Error("the value beside the refused one was applied, so the delivery published a partial " +
			"decode. Either all of a section's values arrive or none do")
	}
}

// TestTheDeliveryLeavesTheRootDirectoryAlone is what the root-directory exclusion buys.
//
// Five Tendermint sub-structs carry a RootDir tagged home, filled by SetRoot after the file is decoded.
// Were one declared, the delivery would decode its baseline over the top, and a node whose root is the
// empty string cannot find its data directory, its genesis file or its signing key.
func TestTheDeliveryLeavesTheRootDirectoryAlone(t *testing.T) {
	configtest.Isolate(t)
	ctx := bootWithSeiToml(t, "schema_version = 1\nnode_mode = \"validator\"\n\n"+
		"[instrumentation]\nprometheus = true\n")

	if !ctx.Config.Instrumentation.Prometheus {
		t.Fatal("the delivery did not run, so this test would pass with the root declared")
	}
	if ctx.Config.RootDir == "" {
		t.Error("the node's root directory is empty after the delivery")
	}
	if ctx.Config.PrivValidator.RootDir == "" {
		t.Error("the signing key's root directory is empty after the delivery. priv-validator is " +
			"declared, so its home key would be delivered at its baseline if it were not excluded")
	}
}
