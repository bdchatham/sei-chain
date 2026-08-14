package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sei-protocol/sei-chain/cmd/seid/cmd/configcli"
	serverconfig "github.com/sei-protocol/sei-chain/sei-cosmos/server/config"
	tmcfg "github.com/sei-protocol/sei-chain/sei-tendermint/config"
	"github.com/sei-protocol/sei-chain/testutil/configtest"
)

// How much of a node's existing configuration the migration can still not carry.
//
// A migration builds sei.toml from the keys the registry declares. Every key an operator's files hold
// that no section owns is a value it reads past, so the file it writes is missing something the
// operator chose. That has to be empty before the migration ships, and until then it is the remaining
// work stated as a number rather than an estimate.

// freshNodeConfig writes the app.toml and config.toml a newly initialised node gets.
//
// Rendered from the same templates and defaults the boot itself uses, so this measures the file an
// operator actually receives rather than what a struct's tags suggest one would contain. The two
// disagree: ninety-two operator-facing keys reach their field through a spelling their tags do not
// produce, and it is the rendered file the operator edits.
func freshNodeConfig(t *testing.T) string {
	t.Helper()
	home := configtest.NewHome(t)
	dir := filepath.Join(home.Root, "config")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := tmcfg.WriteConfigFile(home.Root, tmcfg.DefaultConfig()); err != nil {
		t.Fatalf("render config.toml: %v", err)
	}
	template, appConfig := initAppConfig()
	serverconfig.SetConfigTemplate(template)
	serverconfig.WriteConfigFile(filepath.Join(dir, "app.toml"), appConfig)

	for _, name := range []string{"app.toml", "config.toml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s was not rendered, so the reading below covers only one file: %v", name, err)
		}
	}
	return home.Root
}

// TestEveryKeyANodesFilesCarryHasASectionThatOwnsIt counts what the migration would still drop.
func TestEveryKeyANodesFilesCarryHasASectionThatOwnsIt(t *testing.T) {
	configtest.Isolate(t)
	home := freshNodeConfig(t)

	// Read through the migration's own source, so this measures what it would see. A separate reading
	// would drift from it, and the drift would be invisible: both would still produce a plausible list.
	existing, err := configcli.LegacySource(home)
	if err != nil {
		t.Fatalf("read the rendered configuration: %v", err)
	}

	// The record name stays a literal. The wiring record reads it from this call's second argument.
	configtest.CheckLegacyKeysAreDeclared(t, "legacy_keyspace", existing.AllKeys())
}
