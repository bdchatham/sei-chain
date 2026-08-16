package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace"

	"github.com/sei-protocol/sei-chain/cmd/seid/cmd/configcli"
	"github.com/sei-protocol/sei-chain/cmd/seid/cmd/configmanager"
	"github.com/sei-protocol/sei-chain/config/registry"
	"github.com/sei-protocol/sei-chain/sei-cosmos/client"
	clientconfig "github.com/sei-protocol/sei-chain/sei-cosmos/client/config"
	"github.com/sei-protocol/sei-chain/sei-cosmos/server"
	"github.com/sei-protocol/sei-chain/testutil/configtest"
)

// The client settings, delivered through the source the reader that constructs from them looks at.
//
// These five reach a reader that runs once and then builds a keyring and an RPC client, so a value that
// arrives after it has run changes nothing. The delivery is the same one config.toml's sections use, and
// what makes it work is that the client configuration is now read after the manager rather than before.

// bootClientCtx runs a boot and returns the client context the rest of the command tree would see.
func bootClientCtx(t *testing.T, seiToml, clientToml string) client.Context {
	t.Helper()
	home := configtest.NewHome(t)
	dir := filepath.Join(home.Root, "config")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if clientToml != "" {
		if err := os.WriteFile(filepath.Join(dir, "client.toml"), []byte(clientToml), 0o600); err != nil {
			t.Fatalf("write client.toml: %v", err)
		}
	}
	if seiToml != "" {
		if err := os.WriteFile(filepath.Join(dir, "sei.toml"), []byte(seiToml), 0o600); err != nil {
			t.Fatalf("write sei.toml: %v", err)
		}
	}

	cmd := server.StartCmd(nil, home.Root, []trace.TracerProviderOption{})
	if err := cmd.Flags().Set("home", home.Root); err != nil {
		t.Fatalf("set --home: %v", err)
	}
	if _, err := runManager(t, configmanager.SeiConfigManager{}, cmd); err != nil {
		t.Fatalf("Apply refused the boot: %v", err)
	}
	// What the real tree does after the manager: read the client configuration out of the same source
	// the manager installed into.
	ctx := client.Context{}.WithHomeDir(home.Root)
	if sctx := server.GetServerContextFromCmd(cmd); sctx != nil && sctx.Viper != nil {
		ctx.Viper = sctx.Viper
	}
	resolved, err := clientconfig.ReadFromClientConfig(ctx)
	if err != nil {
		t.Fatalf("read the client configuration: %v", err)
	}
	return resolved
}

// TestASeiTomlValueReachesTheClientContext is the property the ordering change exists for.
func TestASeiTomlValueReachesTheClientContext(t *testing.T) {
	configtest.Isolate(t)
	ctx := bootClientCtx(t,
		"schema_version = 2\nnode_mode = \"validator\"\nchain-id = \"pacific-1\"\noutput = \"json\"\n",
		"chain-id = \"atlantic-2\"\noutput = \"text\"\n")

	if ctx.ChainID != "pacific-1" {
		t.Errorf("the client talks to chain %q with pacific-1 in sei.toml and atlantic-2 in client.toml. "+
			"sei.toml is the file an operator edits, so it has to win", ctx.ChainID)
	}
	if ctx.OutputFormat != "json" {
		t.Errorf("output is %q, want the json sei.toml asks for", ctx.OutputFormat)
	}
}

// TestAdoptionCarriesTheClientSettings is what makes sei.toml winning correct rather than lossy.
//
// Once these keys are declared, a complete sei.toml answers for them and client.toml is superseded. That
// is only safe if the migration carried what client.toml held, so this drives the migration's own reader
// over all three files and requires the client settings to arrive.
func TestAdoptionCarriesTheClientSettings(t *testing.T) {
	configtest.Isolate(t)
	home := configtest.NewHome(t)
	dir := filepath.Join(home.Root, "config")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	home.WriteAppTOML(t, []byte("minimum-gas-prices = \"0.01usei\"\n"))
	if err := os.WriteFile(filepath.Join(dir, "client.toml"),
		[]byte("chain-id = \"atlantic-2\"\noutput = \"json\"\nkeyring-backend = \"test\"\n"+
			"node = \"tcp://sei:26657\"\nbroadcast-mode = \"async\"\n"), 0o600); err != nil {
		t.Fatalf("write client.toml: %v", err)
	}

	existing, err := configcli.LegacySource(home.Root)
	if err != nil {
		t.Fatalf("read the existing configuration: %v", err)
	}
	adopted, err := configcli.Adopt(configcli.Existing{Files: existing}, registry.ModeValidator)
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	written, err := adopted.File.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}

	for key, want := range map[string]any{
		"chain-id":        "atlantic-2",
		"output":          "json",
		"keyring-backend": "test",
		"node":            "tcp://sei:26657",
		"broadcast-mode":  "async",
	} {
		if written[key] != want {
			t.Errorf("%s adopted as %#v, want the %#v client.toml holds. A migration that reads two of "+
				"a node's three files writes a file that loses the third", key, written[key], want)
		}
	}
}

// TestTheLegacyVariableStillAnswersAndTheCanonicalOneWins covers the compatibility this took on.
//
// Two of these keys answered to a variable before they were declared, under a prefix the client reader's
// own viper used. Declaring them would have silenced anyone who had one set.
func TestTheLegacyVariableStillAnswersAndTheCanonicalOneWins(t *testing.T) {
	t.Run("the legacy name still answers", func(t *testing.T) {
		configtest.Isolate(t)
		t.Setenv("SEI_OUTPUT", "json")
		ctx := bootClientCtx(t, "schema_version = 2\nnode_mode = \"validator\"\n", "output = \"text\"\n")
		if ctx.OutputFormat != "json" {
			t.Errorf("output is %q with SEI_OUTPUT set to json. That variable reached this key before it "+
				"was declared, and declaring it must not silence an operator who has one set",
				ctx.OutputFormat)
		}
	})

	t.Run("the canonical name wins over it", func(t *testing.T) {
		configtest.Isolate(t)
		t.Setenv("SEI_OUTPUT", "json")
		t.Setenv(registry.EnvName("output"), "text")
		ctx := bootClientCtx(t, "schema_version = 2\nnode_mode = \"validator\"\n", "")
		if ctx.OutputFormat != "text" {
			t.Errorf("output is %q with both variables set. The canonical name is the one this binary "+
				"documents, so it is the one that wins", ctx.OutputFormat)
		}
	})
}
