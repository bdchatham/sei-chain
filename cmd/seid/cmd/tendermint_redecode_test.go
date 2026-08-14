package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace"

	"github.com/sei-protocol/sei-chain/cmd/seid/cmd/configmanager"
	"github.com/sei-protocol/sei-chain/sei-cosmos/server"
	tmcfg "github.com/sei-protocol/sei-chain/sei-tendermint/config"
	"github.com/sei-protocol/sei-chain/testutil/configtest"
)

// Whether a declared config.toml key could reach the settings Tendermint runs.
//
// It cannot today, and the reason is an ordering that is deliberate on both sides. The boot's own handler
// decodes config.toml into a struct, and the values a sei.toml resolves are installed into the source
// afterwards, because the source the handler builds does not exist before it runs. So a declared
// Tendermint key would land in the source after the struct it would populate was already built, and
// nothing decodes a second time.
//
// Decoding a second time is the cheapest way out, and these tests are what say whether it is safe. They
// are here rather than in a design note because the answer is a property of viper, mapstructure and the
// upstream handler together, and none of those three is ours.

// bootWithConfigToml boots the way bootWithSeiToml does, with a config.toml already on disk.
//
// The two branches of the upstream handler differ in what the source holds: with a config.toml it reads
// the file in, and without one it writes a fresh file and reads nothing. A re-decode reads that source,
// so both branches have to be measured.
func bootWithConfigToml(t *testing.T, seiToml string) *server.Context {
	t.Helper()
	home := configtest.NewHome(t)
	dir := filepath.Join(home.Root, "config")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tmcfg.WriteConfigFile(home.Root, tmcfg.DefaultConfig())
	if _, err := os.Stat(filepath.Join(dir, "config.toml")); err != nil {
		t.Fatalf("the fixture wrote no config.toml, so this takes the same branch as the other test: %v",
			err)
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
	ctx, err := runManager(t, configmanager.SeiConfigManager{}, cmd)
	if err != nil {
		t.Fatalf("Apply refused the boot: %v", err)
	}
	return ctx
}

// tendermintLeaves renders every reachable leaf of the Tendermint configuration as path to value.
//
// Rendered rather than compared with reflect.DeepEqual, because what a failure needs to say is which
// field moved. A whole-struct comparison reports only that one did.
func tendermintLeaves(v any) map[string]string {
	out := map[string]string{}
	collectLeaves(reflect.ValueOf(v), "", out)
	return out
}

func collectLeaves(v reflect.Value, prefix string, out map[string]string) {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			out[prefix] = "<nil>"
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		if v.CanInterface() {
			out[prefix] = fmt.Sprintf("%#v", v.Interface())
		}
		return
	}
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
		if !f.IsExported() {
			continue
		}
		name := f.Name
		if prefix != "" {
			name = prefix + "." + name
		}
		collectLeaves(v.Field(i), name, out)
	}
}

// movedFields names every leaf whose value differs between two readings, sorted.
func movedFields(before, after map[string]string) []string {
	var moved []string
	for path, was := range before {
		now, present := after[path]
		if !present {
			moved = append(moved, path+" (gone)")
			continue
		}
		if now != was {
			moved = append(moved, path)
		}
	}
	for path := range after {
		if _, present := before[path]; !present {
			moved = append(moved, path+" (new)")
		}
	}
	sort.Strings(moved)
	return moved
}

// remainField is the only leaf a second decode is allowed to move.
//
// Tendermint's BaseConfig ends in a map tagged ",remain", which mapstructure fills with every key nothing
// else claimed. A second decode sees the whole declared key space in the source and puts all of it there.
// Nothing in this tree reads that map, which is what makes the movement inert rather than tolerated.
const remainField = "BaseConfig.Other"

// TestASecondDecodeOfTheTendermintConfigMovesNothing is the property the ordering fix rests on.
//
// If a second decode moved a field on its own, installing a key and decoding again could not be used at
// all: every node would take the movement whether or not it had written the key.
func TestASecondDecodeOfTheTendermintConfigMovesNothing(t *testing.T) {
	for _, c := range []struct {
		name string
		boot func(*testing.T) *server.Context
	}{
		{"with a config.toml", func(t *testing.T) *server.Context {
			return bootWithConfigToml(t, "schema_version = 1\nnode_mode = \"validator\"\n")
		}},
		{"without one", func(t *testing.T) *server.Context {
			return bootWithSeiToml(t, "schema_version = 1\nnode_mode = \"validator\"\n")
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			configtest.Isolate(t)
			ctx := c.boot(t)
			if ctx.Config == nil {
				t.Fatal("the boot produced no Tendermint configuration")
			}
			before := tendermintLeaves(ctx.Config)
			if len(before) < 100 {
				t.Fatalf("read %d leaf fields, want the whole configuration. A reading this small is "+
					"measuring something other than what a node runs", len(before))
			}

			if err := ctx.Viper.Unmarshal(ctx.Config); err != nil {
				t.Fatalf("a second decode failed: %v", err)
			}

			for _, path := range movedFields(before, tendermintLeaves(ctx.Config)) {
				if path == remainField {
					continue
				}
				t.Errorf("a second decode moved %s with nothing installed. Every node would take that "+
					"movement, so a key cannot be delivered this way", path)
			}
		})
	}
}

// TestTheRootDirectorySurvivesASecondDecode is the field most likely to break, named on its own.
//
// The handler calls SetRoot after its own decode, so the root is the one setting written by code rather
// than read from the source. A second decode reading an empty or different home would leave every path
// derived from it pointing somewhere else, and a node that cannot find its own data directory does not
// start.
func TestTheRootDirectorySurvivesASecondDecode(t *testing.T) {
	configtest.Isolate(t)
	ctx := bootWithConfigToml(t, "schema_version = 1\nnode_mode = \"validator\"\n")

	root := ctx.Config.RootDir
	if root == "" {
		t.Fatal("the boot left no root directory, so this test cannot tell whether one survives")
	}
	if err := ctx.Viper.Unmarshal(ctx.Config); err != nil {
		t.Fatalf("a second decode failed: %v", err)
	}
	if ctx.Config.RootDir != root {
		t.Errorf("the root directory moved from %q to %q on a second decode. Every path a node resolves "+
			"against it would follow", root, ctx.Config.RootDir)
	}
}

// TestAnInstalledTendermintKeyReachesItsSettingOnASecondDecode is the delivery this is all for.
//
// One key, installed into the source the way a declared key would be, and one field moving. That is what
// says config.toml keys can be declared without changing how the boot is ordered.
func TestAnInstalledTendermintKeyReachesItsSettingOnASecondDecode(t *testing.T) {
	configtest.Isolate(t)
	ctx := bootWithConfigToml(t, "schema_version = 1\nnode_mode = \"validator\"\n")

	const key = "p2p.max-packet-msg-payload-size"
	const want = 4242
	if ctx.Config.P2P.MaxPacketMsgPayloadSize == want {
		t.Fatalf("the probe is already %d, so this test would pass without installing anything", want)
	}
	before := tendermintLeaves(ctx.Config)

	ctx.Viper.Set(key, want)
	if err := ctx.Viper.Unmarshal(ctx.Config); err != nil {
		t.Fatalf("a second decode failed: %v", err)
	}

	if got := ctx.Config.P2P.MaxPacketMsgPayloadSize; got != want {
		t.Errorf("%s was installed as %d and the setting reads %d. The key did not reach the struct, so "+
			"a second decode is not a delivery mechanism", key, want, got)
	}
	for _, path := range movedFields(before, tendermintLeaves(ctx.Config)) {
		if path == remainField || path == "P2P.MaxPacketMsgPayloadSize" {
			continue
		}
		t.Errorf("installing one key also moved %s. A declared key has to reach its own setting and "+
			"nothing else", path)
	}
}
