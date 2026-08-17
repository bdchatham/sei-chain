package configtest

import (
	"strings"
	"testing"
)

// What CheckSection is held to, by this package rather than by its callers.
//
// A check removed from sectionChecks stops running for every section at once and fails nothing,
// because a check that does not run asserts nothing and its record simply goes unread. That is the one
// way this shape loses coverage in silence, and it is why the list below is written by hand: derived
// from sectionChecks it would move whenever that moved and never disagree with it.

// everyCheckASectionGets is what covering a section means.
//
// A change here is a change to that meaning, so it belongs in a diff beside the change to
// sectionChecks that caused it.
var everyCheckASectionGets = []string{
	"absent",
	"zero-when-absent",
	"defaults",
	"key-names",
	"experimental-shadow",
	"manifest",
}

// TestCheckSectionRunsEveryCheckItClaims is the guard that replaced the coverage record.
func TestCheckSectionRunsEveryCheckItClaims(t *testing.T) {
	var got []string
	for _, check := range sectionChecks {
		got = append(got, check.name)
	}
	if len(got) != len(everyCheckASectionGets) {
		t.Fatalf("a section is put through %v, and covering one is defined as %v.\n\nA name missing "+
			"from the first list stopped running for every section at once, and nothing else would "+
			"fail. If the removal is deliberate, take it out of everyCheckASectionGets in the same "+
			"change so the loss lands in a diff", got, everyCheckASectionGets)
	}
	for i, want := range everyCheckASectionGets {
		if got[i] != want {
			t.Errorf("check %d is %q, want %q. The order is the order a failure reports in, so it is "+
				"recorded rather than left to the table's arrangement", i, got[i], want)
		}
	}
}

// TestEveryCheckStatesWhatItNeeds pins that each entry is usable.
//
// An entry whose stated is nil would panic, and one whose run is nil would pass while asserting
// nothing, which is the failure this whole file exists to prevent expressed one row at a time.
func TestEveryCheckStatesWhatItNeeds(t *testing.T) {
	for i, check := range sectionChecks {
		if check.name == "" {
			t.Errorf("check %d has no name, so the contract test above cannot refer to it", i)
		}
		if check.assertion == "" {
			t.Errorf("%s has no assertion, so its subtest would be named for nothing", check.name)
		}
		if check.stated == nil {
			t.Errorf("%s cannot say whether a section supplies what it needs", check.name)
		}
		if check.run == nil {
			t.Errorf("%s runs nothing, so every section passes it", check.name)
		}
	}
}

// TestASectionMayNotOptOutOfACheckBySayingNothing pins the refusals.
//
// They are what make the list above mean anything. Without them a section reaches full coverage on
// paper and half of it in fact, by leaving the input a check needs empty.
func TestASectionMayNotOptOutOfACheckBySayingNothing(t *testing.T) {
	stated := func() Section {
		return Section{
			Read:     func(AppOpts) (any, error) { return struct{}{}, nil },
			Defaults: struct{}{},
			Keys:     []KeySpec{{Key: "probe.value"}},
		}
	}

	for _, tc := range []struct {
		name string
		of   func(Section) Section
		want string
	}{
		{"no reader and no reason",
			func(s Section) Section { s.Read = nil; return s }, "WithoutReader"},
		{"no defaults and no reason",
			func(s Section) Section { s.Defaults = nil; return s }, "WithoutDefaults"},
		{"no keys at all",
			func(s Section) Section { s.Keys = nil; return s }, "declares no keys"},
		{"a reader and a reason not to have one",
			func(s Section) Section { s.WithoutReader = "because"; return s }, "supplies a reader"},
		{"defaults and a reason not to have them",
			func(s Section) Section { s.WithoutDefaults = "because"; return s }, "supplies defaults"},
		{"the same key twice",
			func(s Section) Section {
				s.Keys = append(s.Keys, s.Keys[0])
				return s
			}, "twice in its manifest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := capture(t, func(tb testing.TB) {
				requireSectionIsStated(tb, "probe", tc.of(stated()))
			})
			if len(c.failures) == 0 {
				t.Fatalf("a section with %s was accepted. Its checks would run short and nothing "+
					"would say so", tc.name)
			}
			if !strings.Contains(c.only(t), tc.want) {
				t.Errorf("the refusal for %s reads %q, which does not mention %q",
					tc.name, c.only(t), tc.want)
			}
		})
	}
}

// TestAFullyStatedSectionIsNotRefused is the other half, so the refusals above cannot pass by
// refusing everything.
func TestAFullyStatedSectionIsNotRefused(t *testing.T) {
	c := capture(t, func(tb testing.TB) {
		requireSectionIsStated(tb, "probe", Section{
			Read:     func(AppOpts) (any, error) { return struct{}{}, nil },
			Defaults: struct{}{},
			Keys:     []KeySpec{{Key: "probe.value"}},
		})
	})
	if len(c.failures) > 0 {
		t.Fatalf("a section stating a reader, defaults and one key was refused: %v", c.failures)
	}
}
