package configmanager

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/sei-protocol/sei-chain/config/registry"
	"github.com/sei-protocol/sei-chain/config/seitoml"
)

// RefuseConflictingFlags stops a boot where a flag the operator typed disagrees with a file they wrote,
// for a key that says the two cannot both be meant.
//
// A flag beating a file is ordinarily the point. For a few settings the disagreement is evidence of a
// mistake rather than an intent, because the two answer a question that has one answer, and the chain a
// node joins is the case: a flag naming one chain and a file naming another is a node about to join a
// network nobody chose.
//
// This runs before either manager and outside both, which is the whole reason it works. The check the
// upstream start command makes compares the flag against the value it resolved, and once the files and
// the flags share one source that value is the flag, so it can never see a difference. Here the two are
// still separate: what the operator typed, and what their files say.
//
// Outside both managers rather than inside one, because the reading that defeats the upstream check is
// the shared one in the root command and not anything a manager does. A check on one path would leave
// the other running without it.
//
// Every file that states the key is compared, rather than whichever file this node's manager would read.
// A node in the middle of migrating has both, and a value in either is a value somebody wrote.
func RefuseConflictingFlags(cmd *cobra.Command) error {
	keys := registry.ConflictingFlagKeys()
	if len(keys) == 0 {
		return nil
	}
	typed := TypedFlags(cmd)
	if len(typed) == 0 {
		return nil
	}
	home, err := resolveHomeDir(cmd)
	if err != nil {
		return nil // reported by the manager, which resolves the same directory a moment later
	}

	written := writtenValues(home)
	for _, key := range keys {
		onCommandLine, wasTyped := typed[key]
		if !wasTyped {
			continue
		}
		for file, values := range written {
			inFile, states := values[key]
			if !states || fmt.Sprintf("%v", inFile) == onCommandLine {
				continue
			}
			why, _ := registry.ConflictingFlagRefused(key)
			return fmt.Errorf("%s is %q on the command line and %v in %s: %s. Change one of them, or "+
				"drop the flag to use the file", key, onCommandLine, inFile, file, why)
		}
	}
	return nil
}

// writtenValues reads what each of this node's configuration files states, by file name.
//
// Read directly rather than through the source the boot builds, because that source merges the files
// with the flags and ranks the flags above them, which is the ranking that makes the comparison
// impossible. A file that is absent or unreadable states nothing, which is not an error here: every
// other diagnostic reports on those, and this one is looking for a value that disagrees.
func writtenValues(home string) map[string]map[string]any {
	out := map[string]map[string]any{}
	dir := filepath.Join(home, "config")

	if file, err := seitoml.Load(filepath.Join(dir, seiTomlName)); err == nil {
		if values, err := file.Values(); err == nil {
			out[seiTomlName] = values
		}
	}

	// The legacy files, for a node that has not migrated. Read one at a time into a source of its own so
	// the name in the message is the file the operator has to edit.
	for _, name := range []string{"client.toml", "app.toml", "config.toml"} {
		v := viper.New()
		v.SetConfigFile(filepath.Join(dir, name))
		if err := v.ReadInConfig(); err != nil {
			continue
		}
		values := map[string]any{}
		for _, key := range v.AllKeys() {
			values[key] = v.Get(key)
		}
		out[name] = values
	}
	return out
}
