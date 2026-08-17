package cmd

import (
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace"

	"github.com/sei-protocol/sei-chain/cmd/seid/cmd/configmanager"
	"github.com/sei-protocol/sei-chain/config/registry"
	"github.com/sei-protocol/sei-chain/sei-cosmos/server"
	tmcfg "github.com/sei-protocol/sei-chain/sei-tendermint/config"
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
	ctx := bootWithSeiToml(t, "schema_version = 2\nnode_mode = \"validator\"\n\n"+
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

// TestAnUnwrittenTendermintKeyKeepsWhatConfigTomlSaid is the property that separates delivering a value
// from overwriting one.
//
// A section read by a lookup can be delivered whole: its reader has nowhere else to get a value from. A
// section read by a decode already holds what its own file said, put there by the boot's handler before
// any of this ran. So a key the operator's sei.toml does not mention has to arrive at whatever
// config.toml gave it, and delivering the baseline instead replaces their file with a default nobody
// chose, on every boot.
//
// The fixture turns the key on in config.toml, where the baseline is off, so the two disagree. Without
// that they agree and the overwrite is invisible, which is how it passed here once already.
func TestAnUnwrittenTendermintKeyKeepsWhatConfigTomlSaid(t *testing.T) {
	configtest.Isolate(t)
	home := configtest.NewHome(t)
	dir := filepath.Join(home.Root, "config")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := tmcfg.WriteConfigFile(home.Root, tmcfg.DefaultConfig()); err != nil {
		t.Fatalf("render config.toml: %v", err)
	}

	path := filepath.Join(dir, "config.toml")
	raw, err := os.ReadFile(path) // #nosec G304 -- a path this test just wrote
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	edited := regexp.MustCompile(`(?m)^prometheus = false$`).ReplaceAll(raw, []byte("prometheus = true"))
	if string(edited) == string(raw) {
		t.Fatal("the fixture left the key at its baseline, so an overwrite would be invisible")
	}
	if err := os.WriteFile(path, edited, 0o600); err != nil {
		t.Fatalf("write the fixture: %v", err)
	}
	// sei.toml says nothing about this section at all.
	if err := os.WriteFile(filepath.Join(dir, "sei.toml"),
		[]byte("schema_version = 2\nnode_mode = \"validator\"\n"), 0o600); err != nil {
		t.Fatalf("write sei.toml: %v", err)
	}

	cmd := server.StartCmd(nil, home.Root, []trace.TracerProviderOption{})
	if err := cmd.Flags().Set("home", home.Root); err != nil {
		t.Fatalf("set --home: %v", err)
	}
	ctx, err := runManager(t, configmanager.SeiConfigManager{}, cmd)
	if err != nil {
		t.Fatalf("Apply refused the boot: %v", err)
	}

	if !ctx.Config.Instrumentation.Prometheus {
		t.Error("config.toml turned the metrics listener on, sei.toml said nothing about it, and the " +
			"node runs with it off. The baseline was delivered over the operator's own file, which " +
			"happens on every boot for every key their sei.toml does not mention")
	}
}

// TestAnUnwrittenTendermintKeyKeepsTheDefaultWhenNoFileSaysOtherwise is the same property where the two
// agree, which is most of a fleet.
func TestAnUnwrittenTendermintKeyKeepsTheDefaultWhenNoFileSaysOtherwise(t *testing.T) {
	configtest.Isolate(t)
	ctx := bootWithSeiToml(t, "schema_version = 2\nnode_mode = \"validator\"\n\n"+
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
	ctx := bootWithSeiToml(t, "schema_version = 2\nnode_mode = \"validator\"\n\n"+
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
	ctx := bootWithSeiToml(t, "schema_version = 2\nnode_mode = \"validator\"\n\n"+
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

// TestAWrittenListReplacesTheOneTheNodeHad is the first declared key carrying a list.
//
// mapstructure decodes a slice by writing the input's elements into the existing one and cutting it to
// the input's length, so a shorter list truncates rather than leaving a tail behind. That is what an
// operator writing a list means: these servers, not these as well as the ones already there.
//
// The fixture gives config.toml three servers and sei.toml two, so a delivery that appended would leave
// five and one that merged would leave three. Only replacement leaves two.
func TestAWrittenListReplacesTheOneTheNodeHad(t *testing.T) {
	configtest.Isolate(t)
	home := configtest.NewHome(t)
	dir := filepath.Join(home.Root, "config")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := tmcfg.WriteConfigFile(home.Root, tmcfg.DefaultConfig()); err != nil {
		t.Fatalf("render config.toml: %v", err)
	}

	path := filepath.Join(dir, "config.toml")
	raw, err := os.ReadFile(path) // #nosec G304 -- a path this test just wrote
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	// Written the way the Tendermint template writes it, as one comma-joined string, because that is
	// what a node's file holds. Viper splits it back into a list on the boot's own decode. A fixture
	// using the list form exercises a shape no node has, and the advisory read of that file fails.
	edited := regexp.MustCompile(`(?m)^rpc-servers = .*$`).
		ReplaceAll(raw, []byte(`rpc-servers = "old-1:26657,old-2:26657,old-3:26657"`))
	if string(edited) == string(raw) {
		t.Fatal("the fixture set no servers in config.toml, so replacement cannot be told from anything")
	}
	if !strings.Contains(string(edited), `"old-1:26657,old-2:26657,old-3:26657"`) {
		t.Fatal("the fixture is not in the form the template writes, so it is exercising a file no node has")
	}
	if err := os.WriteFile(path, edited, 0o600); err != nil {
		t.Fatalf("write the fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sei.toml"), []byte("schema_version = 2\n"+
		"node_mode = \"validator\"\n\n[statesync]\nrpc-servers = [\"new-1:26657\", \"new-2:26657\"]\n"),
		0o600); err != nil {
		t.Fatalf("write sei.toml: %v", err)
	}

	cmd := server.StartCmd(nil, home.Root, []trace.TracerProviderOption{})
	if err := cmd.Flags().Set("home", home.Root); err != nil {
		t.Fatalf("set --home: %v", err)
	}
	ctx, err := runManager(t, configmanager.SeiConfigManager{}, cmd)
	if err != nil {
		t.Fatalf("Apply refused the boot: %v", err)
	}

	got := ctx.Config.StateSync.RPCServers
	want := []string{"new-1:26657", "new-2:26657"}
	if len(got) != len(want) {
		t.Fatalf("the node trusts %v, want %v. A longer list means the delivery added to the servers "+
			"config.toml named rather than replacing them, and a node verifying snapshots against a "+
			"server the operator removed is the failure that hides behind that", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("server %d is %q, want %q", i, got[i], want[i])
		}
	}
}

// TestADeliveredLogLevelReachesTheLogger is the one delivered setting the struct is not the end of.
//
// The boot's handler reads the level off the struct and hands it to the logger before resolved values
// are delivered, so a value decoded afterwards moves the field and changes no logging. A setting that
// appears to take and does not is what this key space exists to remove, so the delivery applies it.
//
// Read back through a probe logger, because seilog has a setter and no getter, and because the field
// alone would pass this test while the node still logged at the level it started with.
func TestADeliveredLogLevelReachesTheLogger(t *testing.T) {
	configtest.Isolate(t)
	before := configtest.LogDefaultLevel()
	if before == slog.LevelWarn {
		t.Fatalf("the logger already sits at %v, so this fixture cannot show it moving", before)
	}

	ctx := bootWithSeiToml(t, "schema_version = 2\nnode_mode = \"validator\"\nlog-level = \"warn\"\n")

	if got := ctx.Config.LogLevel; got != "warn" {
		t.Errorf("sei.toml set the log level to warn and the configuration says %q", got)
	}
	if got := configtest.LogDefaultLevel(); got != slog.LevelWarn {
		t.Errorf("the configuration carries the level and the logger sits at %v. The struct is not "+
			"where a log level takes effect, so an operator who set it would see no change in the logs",
			got)
	}
}

// TestEachChannelWinsForADecodedKeyToo is the precedence property, asserted where it lands.
//
// The channel tests elsewhere read ctx.Viper, which is the whole delivery for a section a reader looks
// up. It is not the delivery for a section read by a decode: the value has to reach the struct, and a
// key can be correct in the source and absent from the struct. So this drives the same three channels
// and reads the setting the node runs from.
func TestEachChannelWinsForADecodedKeyToo(t *testing.T) {
	const key = "rpc.laddr"
	const inFile = "tcp://0.0.0.0:11111"
	const inEnv = "tcp://0.0.0.0:22222"
	const onCommandLine = "tcp://0.0.0.0:33333"

	body := "schema_version = 2\nnode_mode = \"validator\"\n\n[rpc]\nladdr = \"" + inFile + "\"\n"

	t.Run("the file beats the baseline", func(t *testing.T) {
		configtest.Isolate(t)
		ctx := bootWithSeiToml(t, body)
		if got := ctx.Config.RPC.ListenAddress; got != inFile {
			t.Errorf("the node listens on %q with %q in sei.toml. The value resolved and never reached "+
				"the struct the node reads", got, inFile)
		}
	})

	t.Run("the environment beats the file", func(t *testing.T) {
		configtest.Isolate(t)
		t.Setenv(registry.EnvName(key), inEnv)
		ctx := bootWithSeiToml(t, body)
		if got := ctx.Config.RPC.ListenAddress; got != inEnv {
			t.Errorf("the node listens on %q with %q in the environment and %q in the file", got,
				inEnv, inFile)
		}
	})

	t.Run("a typed flag beats both", func(t *testing.T) {
		configtest.Isolate(t)
		t.Setenv(registry.EnvName(key), inEnv)
		ctx := bootWithSeiTomlAndFlags(t, body, map[string]string{key: onCommandLine})
		if got := ctx.Config.RPC.ListenAddress; got != onCommandLine {
			t.Errorf("the node listens on %q with %q typed on the command line, %q in the environment "+
				"and %q in the file. An operator's flag is the one channel that must never be buried",
				got, onCommandLine, inEnv, inFile)
		}
	})
}
