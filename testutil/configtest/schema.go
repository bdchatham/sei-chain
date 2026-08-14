package configtest

import (
	"reflect"
	"sort"
	"testing"

	"github.com/sei-protocol/sei-chain/config/registry"
)

// SchemaCheck describes a section whose keys are declared by a struct written for that purpose.
//
// Some sections cannot declare their keys from the type their reader fills. That type may carry no
// mapstructure tags at all, or tags that name something other than the keys the reader resolves, and
// it may live in a tree this repository does not change. The section then declares a struct written
// for the purpose: it holds the spelling and nothing decodes into it.
//
// That split needs holding together, and the part that needs it most is the correspondence between a
// key and the setting it reaches. A test that states that correspondence twice proves nothing, since
// both statements come from the same reading. This one asks the reader instead.
type SchemaCheck struct {
	// Read runs the section's live reader over a set of written values.
	Read func(AppOpts) (any, error)
	// Probe is one value per declared key, each different from what the reader leaves that key's
	// setting at when nothing is written. A value that is not different leaves the reader's output
	// unchanged, which the check reports: it cannot tell that from a key the reader never looks up.
	//
	// Different from the empty read rather than from the baseline, because the two are not always the
	// same. A reader that assigns straight from a lookup resolves an absent key to zero, so a key whose
	// baseline is true needs a probe of true to be observable at all.
	Probe map[string]any
	// Skip names a key this check does not cover, and what covers it instead. Two cases need it. A
	// section's keys are not always read by one function, so a value another reader resolves into
	// another type would report as a key that reaches nothing. And a reader may compute one setting
	// from two keys, so neither of those keys reaches exactly one setting and probing one without the
	// other reaches none. The reason is required, so an exclusion has to say where the key is covered
	// rather than only that it is not covered here.
	Skip map[string]string
}

// CheckSchemaMatchesTheReader holds a purpose-written schema against the reader it describes.
//
// One property, per declared key: writing a value under the key changes exactly one setting the reader
// returns. That is what says the key reaches something, and which thing.
//
// The property is not stated by hand. The setting a key reaches is found by writing to the key and
// observing the reader, so a schema field paired with the wrong setting fails here rather than
// resolving one operator's value into another's setting.
//
// What such a key resolves to when nothing supplies it is a separate question, and
// CheckZeroWhenAbsentMatchesTheReader answers it against the same reader.
func CheckSchemaMatchesTheReader(t testing.TB, section string, c SchemaCheck) {
	t.Helper()

	registered, ok := registry.Lookup(section)
	if !ok {
		t.Fatalf("%s is not registered, so there is no schema to check", section)
	}
	if len(registered.Keys) == 0 {
		t.Fatalf("%s declares no keys, so every check below holds by covering nothing", section)
	}
	if _, err := c.Read(AppOpts{}); err != nil {
		t.Fatalf("%s: the reader refused an empty configuration, which is what a node that has written "+
			"nothing has: %v", section, err)
	}

	for _, key := range registered.Keys {
		probe, covered := c.probeFor(t, section, key)
		if !covered {
			continue
		}
		if !c.probeIsTheDeclaredShape(t, section, key, probe) {
			continue
		}
		c.checkKeyReachesOneSetting(t, section, key, probe)
	}
}

// probeFor returns the probe value for a key, and whether this check covers the key at all.
func (c SchemaCheck) probeFor(t testing.TB, section, key string) (any, bool) {
	t.Helper()

	if reason, skipped := c.Skip[key]; skipped {
		if reason == "" {
			t.Errorf("%s: %q is skipped with no reason. An exclusion has to name what covers the key "+
				"instead, or it is indistinguishable from one nothing covers", section, key)
		}
		return nil, false
	}
	probe, ok := c.Probe[key]
	if !ok {
		t.Errorf("%s: no probe value for %q, so nothing checks which setting it reaches", section, key)
		return nil, false
	}
	return probe, true
}

// probeIsTheDeclaredShape holds the probe to the shape the resolved configuration delivers.
//
// A probe of some other shape tests a value no operator could cause to arrive, and a reader that
// accepts only one shape would look as though it accepted the declared one. The simulation gas limit is
// the case: its reader takes a non-empty string and ignores a number, so declaring it as a number gives
// an operator a setting that never applies.
//
// Checked against every mode's baseline, because a section resolves one per mode and a key whose
// declared shape varies between them has no single shape to deliver.
func (c SchemaCheck) probeIsTheDeclaredShape(t testing.TB, section, key string, probe any) bool {
	t.Helper()

	for _, mode := range registry.Modes() {
		resolved, err := registry.Resolve(mode)
		if err != nil {
			t.Fatalf("%s: cannot resolve the baseline for %q: %v", section, mode, err)
		}
		baseline, ok := resolved.Keys[key]
		if !ok {
			t.Errorf("%s: the resolver produced no baseline for %q in %q mode", section, key, mode)
			return false
		}
		if want, got := reflect.TypeOf(baseline.Value), reflect.TypeOf(probe); want != got {
			t.Errorf("%s: the schema declares %q as %v in %q mode and the probe is %v. The resolved "+
				"configuration delivers the declared shape, so a probe of another shape checks a value "+
				"that cannot arrive; make the probe match, or the schema match what the reader takes",
				section, key, want, mode, got)
			return false
		}
	}
	return true
}

// checkKeyReachesOneSetting writes the probe and requires exactly one of the reader's settings to move.
func (c SchemaCheck) checkKeyReachesOneSetting(t testing.TB, section, key string, probe any) {
	t.Helper()

	base, err := c.Read(AppOpts{})
	if err != nil {
		t.Errorf("%s: the reader refused an empty configuration while checking %q: %v", section, key, err)
		return
	}
	written, err := c.Read(AppOpts{key: probe})
	if err != nil {
		t.Errorf("%s: the reader refused %v under %q: %v", section, probe, key, err)
		return
	}

	switch changed := settingsThatDiffer(base, written); len(changed) {
	case 1:
	case 0:
		t.Errorf("%s: writing %v under %q changed nothing the reader returns, so either the reader "+
			"does not resolve that key or the probe is not distinctive", section, probe, key)
	default:
		t.Errorf("%s: writing %v under %q changed %v. A key that reaches several settings is one this "+
			"check cannot describe, so it needs skipping with the reason that covers it instead",
			section, probe, key, changed)
	}
}

// settingsThatDiffer returns the exported field paths whose values differ between two reader outputs.
func settingsThatDiffer(before, after any) []string {
	var paths []string
	walkFields(reflect.ValueOf(before), reflect.ValueOf(after), "", &paths)
	sort.Strings(paths)
	return paths
}

// walkFields appends the paths of differing exported fields, descending into nested structs.
func walkFields(before, after reflect.Value, prefix string, paths *[]string) {
	for before.Kind() == reflect.Ptr {
		if before.IsNil() != after.IsNil() {
			*paths = append(*paths, prefix)
			return
		}
		if before.IsNil() {
			return
		}
		before, after = before.Elem(), after.Elem()
	}
	if before.Kind() != reflect.Struct {
		if !reflect.DeepEqual(before.Interface(), after.Interface()) {
			*paths = append(*paths, prefix)
		}
		return
	}
	for i := 0; i < before.NumField(); i++ {
		if !before.Type().Field(i).IsExported() {
			continue
		}
		name := before.Type().Field(i).Name
		if prefix != "" {
			name = prefix + "." + name
		}
		walkFields(before.Field(i), after.Field(i), name, paths)
	}
}
