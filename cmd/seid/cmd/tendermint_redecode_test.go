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
// Decoding a second time is the cheapest way out, and these tests are what say how far it goes. They are
// here rather than in a design note because the answer is a property of viper, mapstructure and the
// upstream handler together, and none of those three is ours.
//
// The answer is that decoding the whole source a second time is not safe, and the test below that fails
// is the reason. A second decode does not see what the first saw: the handler merges app.toml into the
// same source afterwards, and its bindFlags pass copies configuration values into flags as text and marks
// them changed, which viper then ranks above the file. So a value that reached the first decode as a
// number reaches the second as a string, and a duration written as a bare number is rejected. Every
// property below holds; none of them says the whole source may be decoded again.
//
// What that leaves is decoding the resolved keys alone, out of a source built for them, which is what
// deliverDecodedSections does and what tendermint_delivery_test.go covers.
//
// Two tests that lived here are gone, both for the same reason: declaring the config.toml sections put
// their resolved baselines into that source at override rank, and a whole-source decode now sees them.
// One measured an installed key reaching its own setting and nothing else, which stopped being true
// because every declared baseline is now in the source. The other exhibited a duration written as a bare
// number decoding on the first pass and failing on the second, which the declared baseline now masks by
// outranking the stringified flag that caused it. Neither survives the declarations, and neither is a
// property of the delivery that replaced them.

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

// TestASecondDecodeOfTheWholeSourceMovesFieldsNobodyWrote is why the delivery reads a narrow source.
//
// The boot's handler writes P2P.RecvRate, P2P.SendRate and the commit timeout override into the struct
// by hand, after its own decode. Once config.toml sections are declared, the installed baselines for
// those keys sit in the source, so a decode of the whole source reverts every one of them, on every
// node, with nothing written by anybody.
//
// Recorded as the movement it is rather than asserted away, and what is required is that those fields
// are among the ones that move. If they stop moving, this has stopped measuring the hazard rather than
// the hazard having gone.
func TestASecondDecodeOfTheWholeSourceMovesFieldsNobodyWrote(t *testing.T) {
	for _, c := range []struct {
		name string
		boot func(*testing.T) *server.Context
	}{
		{"with a config.toml", func(t *testing.T) *server.Context {
			return bootWithConfigToml(t, "schema_version = 2\nnode_mode = \"validator\"\n")
		}},
		{"without one", func(t *testing.T) *server.Context {
			return bootWithSeiToml(t, "schema_version = 2\nnode_mode = \"validator\"\n")
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

			moved := map[string]bool{}
			for _, path := range movedFields(before, tendermintLeaves(ctx.Config)) {
				moved[path] = true
			}
			if !moved[remainField] {
				t.Errorf("the ,remain map did not move. A second decode puts the whole declared key " +
					"space there, so this is no longer measuring a second decode at all")
			}
			// The handler sets these only on the branch where it writes a fresh file, so only that
			// branch can show them being reverted.
			if c.name != "without one" {
				return
			}
			for _, path := range []string{"P2P.RecvRate", "P2P.SendRate"} {
				if !moved[path] {
					t.Errorf("%s did not move on a whole-source decode, which is the hazard the narrow "+
						"delivery exists for. This test has stopped measuring it", path)
				}
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
	ctx := bootWithConfigToml(t, "schema_version = 2\nnode_mode = \"validator\"\n")

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
