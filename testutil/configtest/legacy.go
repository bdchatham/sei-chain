package configtest

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/sei-protocol/sei-chain/config/registry"
)

// CheckLegacyKeysAreDeclared records the keys a node's existing configuration files carry that no
// section declares.
//
// This is the remaining work, counted. A migration builds sei.toml from the keys the registry declares,
// so a key an operator's app.toml or config.toml can hold and no section owns is a value the migration
// reads past and drops. The operator's file said something and the new one does not.
//
// Empty is the finished state, and it is what makes shipping the migration safe: every value a node
// could be carrying has somewhere to land. A record rather than an assertion because it is not empty
// yet, and a count that shrinks in a diff is what says which release closed how much of it.
//
// A key nothing reads is not remaining work, and inert names the ones that are in that position. The
// rendered files carry a few keys whose spelling no reader looks up, so declaring one would put a
// setting in the registry that reaches nothing. Each needs the reason it is inert, and the reason has
// to name what establishes that, because a key called inert without evidence is indistinguishable from
// one nobody has looked at.
//
// The subtraction happens here rather than at the call site. A caller that computed it would be stating
// the declared set a second time, and the two would disagree the first time a section was added.
func CheckLegacyKeysAreDeclared(t testing.TB, name string, legacyKeys []string, inert map[string]string) {
	t.Helper()

	if len(legacyKeys) == 0 {
		t.Fatalf("no legacy keys were read, so %s would record an empty set and the remaining work "+
			"would read as none", name)
	}
	declared := map[string]bool{}
	for _, key := range registry.Keys() {
		declared[key] = true
	}

	var undeclared []string
	for _, key := range legacyKeys {
		if declared[key] {
			if _, named := inert[key]; named {
				t.Errorf("%q is named as inert and a section declares it. One of the two is wrong, and "+
					"while they disagree this record undercounts the work by one", key)
			}
			continue
		}
		if reason, named := inert[key]; named {
			if reason == "" {
				t.Errorf("%q is named as inert with no reason. Without one it cannot be told from a key "+
					"nobody has looked at yet, which is what this record exists to count", key)
			}
			continue
		}
		undeclared = append(undeclared, key)
	}
	sort.Strings(undeclared)

	// A name here for a key the files do not carry is a reason nobody can check, and it makes the
	// remaining work read as smaller than it is.
	carried := map[string]bool{}
	for _, key := range legacyKeys {
		carried[key] = true
	}
	for key := range inert {
		if !carried[key] {
			t.Errorf("%q is named as inert and no rendered file carries it. Either the template stopped "+
				"writing it, in which case remove this, or the name is wrong", key)
		}
	}

	var b strings.Builder
	b.WriteString("# Keys a freshly initialised node's configuration files carry that no section\n")
	b.WriteString("# declares. Regenerate with -update.\n")
	b.WriteString("#\n")
	b.WriteString("# A migration builds sei.toml from the declared keys, so every line here is a value\n")
	b.WriteString("# an operator can write today that the migration would read past and drop. Empty is\n")
	b.WriteString("# the finished state, and it is the condition for shipping the migration.\n")
	b.WriteString("#\n")
	b.WriteString("# Read through the same source the migration reads, so this measures what that\n")
	b.WriteString("# migration would actually see rather than what a struct's tags suggest.\n\n")
	if len(undeclared) == 0 {
		b.WriteString("(none: every key these files carry has a section that owns it)\n")
	}
	for _, key := range undeclared {
		b.WriteString(key + "\n")
	}

	got := strings.TrimRight(b.String(), "\n")
	path := goldenFilePath(t, name, ".legacy.golden")

	if goldenUpdateRequested() {
		writeGolden(t, name, path, got)
		return
	}
	want, err := os.ReadFile(path) // #nosec G304 -- goldenFilePath confines this to testdata
	if err != nil {
		t.Fatalf("%s: cannot read %s: %v\n\nThis record counts the keys a migration would drop. Create "+
			"it with `go test ./<pkg>/ -update` and read the diff", name, path, err)
	}
	if recorded := strings.TrimRight(string(want), "\n"); recorded != got {
		t.Errorf("%s: the keys no section declares no longer match %s.\n\ngot:\n%s\n\nrecorded:\n%s\n\n"+
			"A line removed is a section being declared, which is the work. A line added is a key an "+
			"operator can now write that the migration would drop. Regenerate with "+
			"`go test ./<pkg>/ -update` and keep the diff in the change that caused it",
			name, path, got, recorded)
	}
}
