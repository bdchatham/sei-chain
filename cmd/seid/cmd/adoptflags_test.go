package cmd_test

import (
	"sort"
	"testing"

	"github.com/sei-protocol/sei-chain/cmd/seid/cmd/configcli"
	"github.com/sei-protocol/sei-chain/config/registry"
)

// The flag layer of an adoption, measured against the sections this binary actually registers.
//
// The tests in configcli drive the layer with a flag set built for them. These drive the one the start
// command binds, which is the only one that runs on a node.

// noFiles is a node with no app.toml and no config.toml, so every key falls to a lower layer.
type noFiles struct{}

func (noFiles) Get(string) any    { return nil }
func (noFiles) AllKeys() []string { return nil }

// adoptWithFlagsOnly adopts a node whose only configuration is the start command's flag defaults.
func adoptWithFlagsOnly(t *testing.T, mode registry.Mode) configcli.Adoption {
	t.Helper()
	got, err := configcli.Adopt(configcli.Existing{
		Files:       noFiles{},
		FlagDefault: configcli.StartFlagDefaults(t.TempDir()),
		LookupEnv:   func(string) (string, bool) { return "", false },
	}, mode)
	if err != nil {
		t.Fatalf("Adopt(%s): %v", mode, err)
	}
	return got
}

// TestEveryBoundFlagsDefaultReadsAsItsKeysType keeps our own defaults out of the refusal path.
//
// A flag's default that cannot be read as its key's declared type is a defect in this binary, not
// anything an operator did, and it costs that key its real value in every adopted file. Adoption
// survives one by falling through and reporting it, so nothing fails at boot; this is what stops one
// from shipping unnoticed in the first place.
func TestEveryBoundFlagsDefaultReadsAsItsKeysType(t *testing.T) {
	for _, mode := range registry.Modes() {
		t.Run(string(mode), func(t *testing.T) {
			got := adoptWithFlagsOnly(t, mode)
			for _, r := range got.Unconvertible {
				t.Errorf("%s is bound to a start flag whose default %#v does not read as the key's "+
					"declared type: %s. Every node whose files omit this key has been running that "+
					"default, and an adopted file cannot record it", r.Key, r.Value, r.Reason)
			}
		})
	}
}

// TestTheFlagLayerAnswersForTheKeysItIsThereFor keeps the layer from being wired to nothing.
//
// Every test above passes against a flag layer that finds no flags at all: nothing would be refused
// and every key would fall through. So this names the keys a start flag actually carries. A key
// leaving this list is a key whose value now comes from somewhere else, which changes what an adopted
// file holds for it.
//
// Both halves of a node's configuration are represented. The start command binds its own flags and then
// calls AddNodeFlags, which binds Tendermint's, so declaring a config.toml section can put a key here
// without anything in this repository being edited.
func TestTheFlagLayerAnswersForTheKeysItIsThereFor(t *testing.T) {
	want := []string{
		"chain-id",
		"compaction-interval",
		"concurrency-workers",
		"consensus.create-empty-blocks",
		"consensus.create-empty-blocks-interval",
		"consensus.double-sign-check-height",
		"consensus.gossip-tx-key-only",
		"db-backend",
		"db-dir",
		"grpc-web.address",
		"grpc-web.enable",
		"grpc.address",
		"grpc.enable",
		"halt-height",
		"halt-time",
		"inter-block-cache",
		"min-retain-blocks",
		"minimum-gas-prices",
		"mode",
		"moniker",
		"p2p.laddr",
		"p2p.persistent-peers",
		"p2p.pex",
		"p2p.private-peer-ids",
		"p2p.upnp",
		"proxy-app",
		"pruning",
		"pruning-interval",
		"pruning-keep-every",
		"pruning-keep-recent",
		"rpc.laddr",
		"rpc.pprof-laddr",
		"rpc.unsafe",
		"state-sync.snapshot-interval",
		"state-sync.snapshot-keep-recent",
	}

	got := adoptWithFlagsOnly(t, registry.ModeFull).FromFlagDefault
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("a start flag answers for %d declared key(s), and this test names %d:\ngot  %v\nwant %v",
			len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("a start flag answers for %q where this test names %q", got[i], want[i])
		}
	}
}

// TestABoundFlagAnswersAheadOfAKeysZero is the case that makes the layer order load-bearing.
//
// Some keys are both bound to a start flag and declared as resolving to their zero when nothing
// supplies them. Both are true of such a key: the reader does assign straight from its lookup, and the
// resolution does reach the flag's default before that lookup comes back empty. The flag answers
// first, so the zero is not what such a node runs.
//
// Getting that order backwards is not a cosmetic difference. grpc.enable would adopt as false on a node
// whose app.toml omits it, taking the gRPC server off a node that has been serving it, and pruning
// would adopt as the empty string rather than "default".
func TestABoundFlagAnswersAheadOfAKeysZero(t *testing.T) {
	bound := configcli.StartFlagDefaults(t.TempDir())
	adopted, err := adoptWithFlagsOnly(t, registry.ModeFull).File.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}

	both := 0
	for _, key := range registry.ZeroWhenAbsentKeys() {
		text, isBound := bound(key)
		if !isBound {
			continue
		}
		both++
		if written, ok := adopted[key].(string); ok && written != text {
			t.Errorf("%s adopted as %q where its bound flag defaults to %q", key, written, text)
		}
		if isZero(adopted[key]) && !isZero(coercedText(text)) {
			t.Errorf("%s adopted as its zero %#v while a start flag supplies %q. A node whose files "+
				"omit this key has been running the flag's default, and this file takes it away",
				key, adopted[key], text)
		}
	}
	if both == 0 {
		t.Error("no key is both flag-bound and zero-when-absent, so this test asserts nothing. Either " +
			"the declarations moved or the flag layer stopped answering")
	}
}

// isZero reports whether a value read back off a file is its type's zero.
func isZero(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case bool:
		return !v
	case int64:
		return v == 0
	case float64:
		return v == 0
	default:
		return false
	}
}

// coercedText reads a flag's default the way a written value reads back, so the two are comparable.
func coercedText(text string) any {
	switch text {
	case "":
		return ""
	case "false":
		return false
	case "true":
		return true
	case "0":
		return int64(0)
	default:
		return text
	}
}
