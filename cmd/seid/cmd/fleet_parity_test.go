package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sei-protocol/sei-chain/admin"
	"github.com/sei-protocol/sei-chain/app"
	"github.com/sei-protocol/sei-chain/cmd/seid/cmd/configcli"
	"github.com/sei-protocol/sei-chain/cmd/seid/cmd/configmanager"
	"github.com/sei-protocol/sei-chain/config/registry"
	evmrpcconfig "github.com/sei-protocol/sei-chain/evmrpc/config"
	gigaconfig "github.com/sei-protocol/sei-chain/giga/executor/config"
	srvconfig "github.com/sei-protocol/sei-chain/sei-cosmos/server/config"
	"github.com/sei-protocol/sei-chain/sei-wasmd/x/wasm"
	"github.com/sei-protocol/sei-chain/testutil/configtest"
	"github.com/sei-protocol/sei-chain/x/evm/blocktest"
	"github.com/sei-protocol/sei-chain/x/evm/querier"
	"github.com/sei-protocol/sei-chain/x/evm/replay"

	"github.com/sei-protocol/sei-chain/sei-cosmos/server"
)

// Whether a sei.toml adopted from a node's own files resolves what those files resolve.
//
// This is the question adoption exists to answer, and the settings comparison next door cannot answer
// it. That one compares the flat map each manager produces, which differs by design here: v2 writes
// every declared key into the file, so a key an older release never wrote is present on one side and
// absent on the other. Those rows are the adoption working, not failing.
//
// What has to match is what the node ends up running, so this compares the configuration each reader
// produces rather than the map it reads from. A type the reader casts away is then not a finding, and a
// value it carries through is.

// bootReader is one configuration reader, named as the boot names it.
type bootReader struct {
	// name is what a failure calls this reader.
	name string
	// read runs it against a populated server context.
	read func(*server.Context) (any, error)
}

// bootReaders are the readers app.New calls, plus the server config the handler builds.
//
// Taken from the boot rather than from the registry's section list, because this asks what the node
// does with a resolved configuration. A section no reader here consumes cannot change how the node
// runs, whatever the registry says about it.
//
// Every one takes servertypes.AppOptions, which a viper satisfies, so each reader is handed the very
// channel the boot would hand it.
var bootReaders = []bootReader{
	{"server config", func(c *server.Context) (any, error) { return srvconfig.GetConfig(c.Viper) }},
	{"wasm", func(c *server.Context) (any, error) { return wasm.ReadWasmConfig(c.Viper) }},
	{"evm rpc", func(c *server.Context) (any, error) { return evmrpcconfig.ReadConfig(c.Viper) }},
	{"admin server", func(c *server.Context) (any, error) { return admin.ReadConfig(c.Viper) }},
	{"evm query", func(c *server.Context) (any, error) { return querier.ReadConfig(c.Viper) }},
	{"eth replay", func(c *server.Context) (any, error) { return replay.ReadConfig(c.Viper) }},
	{"eth block test", func(c *server.Context) (any, error) { return blocktest.ReadConfig(c.Viper) }},
	{"giga executor", func(c *server.Context) (any, error) { return gigaconfig.ReadConfig(c.Viper) }},
	{"light invariance", func(c *server.Context) (any, error) { return app.ReadLightInvarianceConfig(c.Viper) }},
	{"genesis import", func(c *server.Context) (any, error) { return app.ReadGenesisImportConfig(c.Viper) }},
}

// requireSameReaderOutput asserts both contexts resolve the same configuration through every reader.
//
// Errors are compared as well as values. A reader that refuses one context and accepts the other is a
// divergence whichever way round it falls, and comparing only the values would read a refusal as an
// empty configuration that happened to match.
func requireSameReaderOutput(t *testing.T, legacy, v2 *server.Context, msgAndArgs ...any) {
	t.Helper()
	where := callerContext(msgAndArgs)
	require.NotNil(t, legacy.Viper, "legacy Apply left serverCtx.Viper nil"+where)
	require.NotNil(t, v2.Viper, "v2 Apply left serverCtx.Viper nil"+where)

	for _, reader := range bootReaders {
		legacyCfg, legacyErr := reader.read(legacy)
		v2Cfg, v2Err := reader.read(v2)

		if legacyErr != nil || v2Err != nil {
			require.Equal(t, legacyErr, v2Err,
				"the %s reader disagrees about whether the configuration is readable%s",
				reader.name, where)
			continue
		}
		if reflect.DeepEqual(legacyCfg, v2Cfg) {
			continue
		}
		// They differ, so the difference is reported through the dump, whose one line per leaf is
		// readable where two nested structs printed whole are not. An empty slice standing in for a
		// nil one is the single difference this tolerates, for the reason writeAbsentSliceAsEmpty
		// gives, and any other leaf still fails here.
		require.Equal(t,
			writeAbsentSliceAsEmpty(configtest.Dump(legacyCfg)),
			writeAbsentSliceAsEmpty(configtest.Dump(v2Cfg)),
			"the %s reader resolves a different configuration from sei.toml than from the node's own "+
				"files%s", reader.name, where)
	}
}

// requireTheDeliveryReachedTheChannel is what stops the comparison above passing on nothing.
//
// The v2 manager re-enters the legacy reader on the node's own files and installs the resolved values
// over the top, so a delivery that installed nothing leaves the two contexts identical and every reader
// trivially agrees. The parity assertion cannot tell that apart from a faithful delivery, and this can:
// these fixtures come from an older release, so the registry declares keys their files do not carry, and
// a delivery that ran put those keys in the v2 channel and nowhere else.
func requireTheDeliveryReachedTheChannel(t *testing.T, legacy, v2 *server.Context, where string) {
	t.Helper()
	legacySettings, v2Settings := configtest.Settings(legacy.Viper), configtest.Settings(v2.Viper)

	var onlyInV2 int
	for key := range v2Settings {
		if _, inLegacy := legacySettings[key]; !inLegacy {
			onlyInV2++
		}
	}
	require.NotZero(t, onlyInV2,
		"the v2 channel carries no key the legacy channel lacks, so sei.toml delivered nothing and "+
			"the reader comparison holds two readings of the same files%s", where)
}

// writeAbsentSliceAsEmpty renders a nil slice the way one that went through sei.toml reads back.
//
// TOML has no null, so a key whose absent value is a nil slice is written as an empty list and read
// back as an empty slice. The v2 path cannot produce a nil slice at all, which makes nil against empty
// the one difference a parity comparison has to allow rather than a finding it could act on. Every
// other leaf is compared as it stands.
func writeAbsentSliceAsEmpty(dump string) string {
	return strings.ReplaceAll(dump, "<nil-slice>", "<empty-slice>")
}

// requireTheReaderListIsUsable refuses a list the comparison could walk without reading anything.
//
// It does not hold the list against the registry's sections, and cannot: one entry here covers ten of
// them, because the server config reader resolves the whole cosmos base in one call. So a reader added
// to app.New and not added here leaves a section uncompared and nothing fails. That gap is real and this
// only closes the part it can, which is a list that is empty or names an entry twice.
func requireTheReaderListIsUsable(t *testing.T) {
	t.Helper()
	if len(bootReaders) == 0 {
		t.Fatal("no readers, so every comparison below passes without reading anything")
	}
	seen := map[string]bool{}
	for _, reader := range bootReaders {
		require.False(t, seen[reader.name], "%q is listed twice", reader.name)
		seen[reader.name] = true
	}
}

// fleetHome copies a fleet fixture's files into a fresh home.
func fleetHome(t *testing.T, name string) *configtest.Home {
	t.Helper()
	home := configtest.NewHome(t)
	dir := filepath.Join("testdata", "fleet", name, "config")

	appTOML, err := os.ReadFile(filepath.Join(dir, "app.toml"))
	require.NoError(t, err, "read the %s fixture's app.toml", name)
	home.WriteAppTOML(t, appTOML)

	configTOML, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	require.NoError(t, err, "read the %s fixture's config.toml", name)
	home.WriteConfigTOML(t, configTOML)

	return home
}

// adoptInPlace writes a sei.toml built from the home's own files.
func adoptInPlace(t *testing.T, home *configtest.Home, mode registry.Mode) {
	t.Helper()
	files, err := configcli.LegacySource(home.Root)
	require.NoError(t, err, "read the home's existing configuration")

	adoption, err := configcli.Adopt(configcli.Existing{
		Files:       files,
		FlagDefault: configcli.StartFlagDefaults(home.Root),
		LookupEnv:   os.LookupEnv,
	}, mode)
	require.NoError(t, err, "adopt for %q mode", mode)
	require.NoError(t, adoption.File.Save(configcli.Path(home.Root)))
}

// TestTheBootReaderListIsUsable guards the list the comparison walks.
func TestTheBootReaderListIsUsable(t *testing.T) {
	requireTheReaderListIsUsable(t)
}

// TestAdoptedSeiTomlResolvesWhatTheNodesFilesResolve is the parity proof for adoption.
func TestAdoptedSeiTomlResolvesWhatTheNodesFilesResolve(t *testing.T) {
	for _, node := range fleetNodes {
		t.Run(node.name, func(t *testing.T) {
			configtest.Isolate(t)

			// One home, read twice. The legacy run goes first and leaves the files untouched, so the
			// adoption below reads what that run read rather than something this test wrote.
			home := fleetHome(t, node.name)
			legacyCtx := runConfigManager(t, configmanager.LegacyConfigManager{}, home)

			adoptInPlace(t, home, node.mode)
			v2Ctx := runConfigManager(t, configmanager.SeiConfigManager{}, home)

			where := callerContext([]any{"the " + node.name + " fixture"})
			requireTheDeliveryReachedTheChannel(t, legacyCtx, v2Ctx, where)
			requireSameReaderOutput(t, legacyCtx, v2Ctx, "the %s fixture", node.name)
		})
	}
}

// TestTheLegacyRunLeavesAFleetNodesFilesAlone is the premise the parity test rests on.
//
// If the handler rewrote app.toml or config.toml, the adoption in that test would read a file this
// binary had just produced and the comparison would be between two of its own renderings. The test
// would still pass, and it would have stopped measuring a node already in the field.
func TestTheLegacyRunLeavesAFleetNodesFilesAlone(t *testing.T) {
	configtest.Isolate(t)
	home := fleetHome(t, "validator")

	before := snapshotHome(t, home)
	_ = runConfigManager(t, configmanager.LegacyConfigManager{}, home)

	require.Equal(t, before, snapshotHome(t, home),
		"the legacy handler changed the fixture, so the parity comparison no longer reads the node's "+
			"own files")
}
