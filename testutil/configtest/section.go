package configtest

import (
	"sort"
	"testing"
)

// Section is everything the checks need to know about one configuration section.
//
// Gathering them makes a section's coverage one call rather than eight. Eight calls are eight things a
// later edit can remove one of while every remaining check still passes, and no package could tell.
//
// One call does not make that impossible, it moves it: a check can still stop running by leaving
// sectionChecks, where it stops running for every section at once. That is why the list is a table
// this package's own contract test holds CheckSection to, rather than a straight-line body. A check
// that does not run asserts nothing and its record simply goes unread, so nothing else would notice.
//
// A section that genuinely has no reader or no defaults says so in the matching Without field. An
// omission with no reason is refused, on the same grounds a registry exclusion is: a field left empty
// cannot be told from one nobody has looked at.
type Section struct {
	// Read runs the section's reader against a set of options. Nil only with WithoutReader.
	Read func(AppOpts) (any, error)
	// Defaults is the struct the reader produces when nothing supplies a key.
	Defaults any
	// Keys are the specs naming every key the reader looks up.
	Keys []KeySpec
	// CoveredElsewhere names fields no spec claims, each because another test owns that field's
	// behaviour. It is CheckManifestCoversEveryField's tail, kept here so the reasons sit beside the
	// manifest they qualify.
	CoveredElsewhere []string
	// DerivedDefaults are the defaults computed from the machine rather than written down.
	DerivedDefaults []DerivedDefault
	// AlsoRecorded are key names the record carries that no spec produces.
	AlsoRecorded []KeyName

	// WithoutReader says why this section has no reader to drive. A section config.toml owns is
	// decoded once and never looked up, so there is nothing for an AppOpts to reach.
	WithoutReader string
	// WithoutDefaults says why this section records no default values.
	WithoutDefaults string
}

// sectionCheck is one check a section is put through.
type sectionCheck struct {
	// name is what the contract test and a report call this check.
	name string
	// stated reports whether the section supplies what this check needs.
	stated func(Section) bool
	// assertion is the subtest name, which says what broke rather than which function found it.
	assertion string
	// run performs the check.
	run func(t *testing.T, section string, s Section)
}

// sectionChecks is every check a section is put through, in order.
//
// A table rather than a sequence of calls, because this is the claim CheckSection makes and
// TestCheckSectionRunsEveryCheckItClaims holds it to. Removing an entry is how this shape loses
// coverage, and it is the one edit that fails nothing on its own.
var sectionChecks = []sectionCheck{
	{
		name:      "absent",
		stated:    func(s Section) bool { return s.Read != nil },
		assertion: "absent keys resolve to the declared defaults",
		run: func(t *testing.T, section string, s Section) {
			CheckAbsent(t, section, s.Read, s.Defaults)
		},
	},
	{
		name:      "zero-when-absent",
		stated:    func(s Section) bool { return s.Read != nil },
		assertion: "a key absent everywhere resolves as the reader's own zero",
		run: func(t *testing.T, section string, s Section) {
			CheckZeroWhenAbsentMatchesTheReader(t, section, s.Read)
		},
	},
	{
		name:      "defaults",
		stated:    func(s Section) bool { return s.Defaults != nil },
		assertion: "defaults match the recorded values",
		run: func(t *testing.T, section string, s Section) {
			CheckDefaults(t, section, s.Defaults, s.DerivedDefaults...)
		},
	},
	{
		name:      "key-names",
		stated:    func(s Section) bool { return len(s.Keys) > 0 },
		assertion: "key names match the recorded names",
		run: func(t *testing.T, section string, s Section) {
			CheckKeyNames(t, section, s.Keys, s.AlsoRecorded...)
		},
	},
	{
		name:      "experimental-shadow",
		stated:    func(s Section) bool { return len(s.Keys) > 0 },
		assertion: "no experimental key shadows this section",
		run: func(t *testing.T, section string, s Section) {
			CheckNoExperimentalKeyShadowsThisSection(t, section, s.Keys)
		},
	},
	{
		name:      "manifest",
		stated:    func(s Section) bool { return len(s.Keys) > 0 && s.Defaults != nil },
		assertion: "the manifest names every field",
		run: func(t *testing.T, section string, s Section) {
			CheckManifestCoversEveryField(t, section, s.Defaults, s.Keys, s.CoveredElsewhere...)
		},
	},
}

// CheckSection runs every check that applies to one section.
//
// Each runs as a subtest, so a failure still names which claim broke while the section keeps a single
// call site.
func CheckSection(t *testing.T, name string, s Section) {
	t.Helper()
	requireSectionIsStated(t, name, s)

	for _, check := range sectionChecks {
		if !check.stated(s) {
			continue
		}
		t.Run(check.assertion, func(t *testing.T) { check.run(t, name, s) })
	}
}

// FuzzSection runs the checks a fuzz target drives, which cannot run under a plain test.
//
// Separate from CheckSection because Go requires a seed corpus and f.Fuzz to live in a FuzzXxx
// function, so these two cannot join the others however much they belong to the same section.
func FuzzSection(f *testing.F, name string, s Section, seeds *Seeds) {
	f.Helper()
	if s.Read == nil {
		f.Fatalf("%s: FuzzSection needs a reader to drive", name)
	}
	if len(s.Keys) == 0 {
		f.Fatalf("%s: FuzzSection needs the section's keys", name)
	}
	CheckEveryRowHasADiscriminatingSeed(f, name, s.Read, s.Keys, seeds)
}

// requireSectionIsStated refuses a section whose omissions carry no reason.
//
// The check that makes the rest trustworthy. Without it a Section left half filled runs half the
// checks and passes, which is indistinguishable from a section that is fully covered.
//
// Takes a testing.TB rather than the *testing.T CheckSection needs for its subtests, so this package's
// own test can drive it through the capture harness and read the refusal it produced.
func requireSectionIsStated(t testing.TB, name string, s Section) {
	t.Helper()
	if name == "" {
		t.Fatal("a section needs a name; it is what every record and every failure is keyed by")
	}
	if s.Read == nil && s.WithoutReader == "" {
		t.Fatalf("%s has no reader and no WithoutReader reason. A section whose reader is simply "+
			"missing cannot be told from one that has none, and half its checks would not run", name)
	}
	if s.Read != nil && s.WithoutReader != "" {
		t.Fatalf("%s states WithoutReader %q and supplies a reader. One of the two is wrong",
			name, s.WithoutReader)
	}
	if s.Defaults == nil && s.WithoutDefaults == "" {
		t.Fatalf("%s has no defaults and no WithoutDefaults reason", name)
	}
	if s.Defaults != nil && s.WithoutDefaults != "" {
		t.Fatalf("%s states WithoutDefaults %q and supplies defaults. One of the two is wrong",
			name, s.WithoutDefaults)
	}
	if len(s.Keys) == 0 && s.WithoutReader == "" {
		t.Fatalf("%s declares no keys. A section with keys nobody listed is the state this whole "+
			"suite exists to prevent", name)
	}
	if dupes := duplicateKeys(s.Keys); len(dupes) > 0 {
		t.Fatalf("%s names %v twice in its manifest, so one of the two rows is never the one a "+
			"failure is about", name, dupes)
	}
}

// duplicateKeys names any key a manifest lists more than once, sorted.
func duplicateKeys(specs []KeySpec) []string {
	seen := map[string]int{}
	for _, spec := range specs {
		seen[spec.Key]++
	}
	var out []string
	for key, n := range seen {
		if n > 1 {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}
