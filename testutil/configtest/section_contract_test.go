package configtest

import (
	"strings"
	"testing"
)

// What CheckSection is held to: that each entry in its table is usable, and that a section cannot
// reach full coverage on paper by leaving a check's input empty. Whether the table still has every
// entry is not checked here. A record of that shape is a test of this suite's arrangement rather than
// of any reader, and review is what holds it.

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
