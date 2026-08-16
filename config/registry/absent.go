package registry

import (
	"fmt"
	"reflect"
	"sort"
)

// zeroWhenAbsent holds the keys whose reader resolves an absent key to the zero value.
var zeroWhenAbsent = map[string]bool{}

// valueWhenAbsent holds the keys whose absent value is neither their baseline nor their zero.
var valueWhenAbsent = map[string]any{}

// DeclareZeroWhenAbsent records that a key resolves to its type's zero when nothing supplies it.
//
// Most readers check that a key was present before assigning, so an absent key keeps the default the
// reader started from. Some assign straight from the lookup, and then an absent key resolves to what the
// cast makes of nothing, which is the zero, and the default beside it is lost.
//
// The difference decides what a migration writes for a key nothing else supplies. A file built for a node
// that has been running has to carry the value the node runs, and for these keys that is the zero rather
// than the default. Writing the default instead changes what the node does, which for the state store is a
// node that stops starting.
//
// A statement about the reader, not about every node. Where a start flag is bound to one of these keys, the
// resolution reaches that flag's default before the lookup comes back empty, so the flag's default is what
// the node runs and the zero never arrives. TestABoundFlagAnswersAheadOfAKeysZero names which keys those
// are and holds the order that decides it.
//
// Declared by the owning package, beside its registration, because whether a read is checked is a fact
// about that package's reader. A check holds the declaration against the reader, so this cannot drift
// from what the code does.
func DeclareZeroWhenAbsent(section string, keys ...string) {
	mu.Lock()
	defer mu.Unlock()
	if len(keys) == 0 {
		defects = append(defects, Defect{Section: section, Err: fmt.Errorf(
			"declared no keys as resolving to zero when absent; a declaration covering nothing reads as " +
				"though the section had been examined")})
		return
	}
	for _, key := range keys {
		zeroWhenAbsent[key] = true
	}
}

// DeclareValueWhenAbsent records the value a key resolves to when neither rule describes it.
//
// Two rules cover almost every key: a reader that checks its read keeps the default, and one that does not
// resolves to the zero. A key needs this only when its answer is neither, and the case that requires it is
// a section whose baseline varies by node mode while its reader's default does not. The EVM service
// toggles are that: seid init writes them per mode, so the baseline matches a node it provisioned, and a
// node whose file lacks them serves those interfaces whatever kind of node it is.
//
// Held to the same standard as the two rules. A check writes this value and requires the reader's output
// to be unchanged, so a declaration that does not preserve behaviour fails rather than shipping.
func DeclareValueWhenAbsent(section, key string, value any) {
	mu.Lock()
	defer mu.Unlock()
	if value == nil {
		defects = append(defects, Defect{Section: section, Err: fmt.Errorf(
			"declared no value for %q when absent; a migration cannot write nothing", key)})
		return
	}
	valueWhenAbsent[key] = value
}

// ZeroWhenAbsent reports whether a key resolves to its type's zero when nothing supplies it.
func ZeroWhenAbsent(key string) bool {
	mu.RLock()
	defer mu.RUnlock()
	return zeroWhenAbsent[key]
}

// ValueWhenAbsent returns the value a section named for a key neither rule describes.
func ValueWhenAbsent(key string) (any, bool) {
	mu.RLock()
	defer mu.RUnlock()
	value, ok := valueWhenAbsent[key]
	return value, ok
}

// ZeroWhenAbsentKeys returns those keys, sorted.
func ZeroWhenAbsentKeys() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(zeroWhenAbsent))
	for key := range zeroWhenAbsent {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// AbsentValues returns, for every declared key, the value a node resolves when nothing supplies it.
//
// This is what a migration writes for a key the existing configuration does not carry. For most keys it
// is the baseline, because the reader keeps its default when the key is absent. For a key declared
// through DeclareZeroWhenAbsent it is the zero of that key's type, because the reader does not. For the
// few where neither holds, it is whatever DeclareValueWhenAbsent named.
//
// Resolved for one mode, since a baseline may vary by mode. The zero does not.
func AbsentValues(mode Mode) (map[string]any, error) {
	resolved, err := Resolve(mode)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(resolved.Keys))
	mu.RLock()
	declared := make(map[string]any, len(valueWhenAbsent))
	for key, value := range valueWhenAbsent {
		declared[key] = value
	}
	mu.RUnlock()

	for key, resolution := range resolved.Keys {
		if value, named := declared[key]; named {
			out[key] = value
			continue
		}
		if !ZeroWhenAbsent(key) {
			out[key] = resolution.Value
			continue
		}
		if resolution.Value == nil {
			return nil, fmt.Errorf("%s resolves to nothing, so this cannot tell what its zero is", key)
		}
		out[key] = reflect.Zero(reflect.TypeOf(resolution.Value)).Interface()
	}
	return out, nil
}

// hostDerived holds the keys whose baseline the running machine decides, and why.
var hostDerived = map[string]string{}

// DeclareHostDerived records that a key's baseline is computed from the machine rather than stated.
//
// Four defaults in the declared space are read off the host: the moniker is its name, and three sizing
// values scale with its processor count. They are correct answers, and they are answers about one
// machine, so two nodes running the same release resolve different ones.
//
// Two things follow, and they are why this is declared rather than left implicit. A record of the key
// space cannot hold the evaluated value, because it would then say what the recording machine had and
// fail everywhere else. And a diagnostic comparing a node's file against this binary's defaults must not
// report one of these as drift, because on a different-sized machine the two legitimately differ.
//
// why names what the value is derived from, so a reader can check the claim. A key declared here without
// one cannot be told from a key somebody guessed about, and the owning package holds the claim with a
// test that computes the same derivation.
func DeclareHostDerived(section, key, why string) {
	mu.Lock()
	defer mu.Unlock()
	if why == "" {
		defects = append(defects, Defect{Section: section, Err: fmt.Errorf(
			"declared %q as host-derived with no reason; without one it cannot be told from a key "+
				"somebody guessed about", key)})
		return
	}
	hostDerived[key] = why
}

// HostDerived reports whether a key's baseline is computed from the running machine.
func HostDerived(key string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, ok := hostDerived[key]
	return ok
}

// HostDerivedKeys returns those keys, sorted.
func HostDerivedKeys() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(hostDerived))
	for key := range hostDerived {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// decodedNotLookedUp holds the sections whose values reach their reader by a decode, and why.
var decodedNotLookedUp = map[string]string{}

// DeclareDecodedNotLookedUp records that a section's values reach their reader by being decoded into a
// struct, rather than by a lookup in the application options.
//
// Almost every section is read the other way: a reader asks the options for a key by name, so installing
// the resolved value into the source is the whole delivery. The sections this names are read once, by
// decoding a configuration file into a struct before any of that happens, and a value installed into the
// source afterwards reaches nothing. They need delivering a second way.
//
// The declaration is what the boot filters on, and it is also why a read census cannot see these keys:
// the census records lookups, and these have none to record.
func DeclareDecodedNotLookedUp(section, why string) {
	mu.Lock()
	defer mu.Unlock()
	if why == "" {
		defects = append(defects, Defect{Section: section, Err: fmt.Errorf(
			"declared as decoded rather than looked up with no reason; the reason names the struct its " +
				"values are decoded into, which is what a reader has to check the claim against")})
		return
	}
	decodedNotLookedUp[section] = why
}

// DecodedNotLookedUp reports whether a section's values reach their reader by a decode.
func DecodedNotLookedUp(section string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, ok := decodedNotLookedUp[section]
	return ok
}

// KeysDecodedNotLookedUp returns the values of every such section that a layer above the baseline
// supplied, keyed by dotted key.
//
// The baselines are deliberately left out, and this is the difference between delivering a value and
// overwriting one. A section read by a lookup can be delivered whole, because the reader has nowhere
// else to get a value from. A section read by a decode already holds what its own file said: the boot's
// handler put it there before any of this ran. Delivering a baseline over that replaces the operator's
// config.toml with a default nobody chose, on every boot, for every key their sei.toml does not mention.
//
// So a key that took its baseline is skipped, and a key any other layer answered is delivered. That
// includes an operator writing the baseline value explicitly, since Resolved records which layer
// answered rather than whether the answer differs from the default: writing false where config.toml
// says true has to reach the node.
func KeysDecodedNotLookedUp(resolved Resolved) map[string]any {
	mu.RLock()
	owning := map[string]bool{}
	for name := range decodedNotLookedUp {
		owning[name] = true
	}
	mu.RUnlock()

	chosen := map[string]bool{}
	for _, key := range resolved.Overrides() {
		chosen[key] = true
	}

	out := map[string]any{}
	for _, section := range Sections() {
		if !owning[section.Name] {
			continue
		}
		for _, key := range section.Keys {
			if !chosen[key] {
				continue
			}
			if resolution, found := resolved.Keys[key]; found {
				out[key] = resolution.Value
			}
		}
	}
	return out
}

// legacyEnvNames holds the environment variables a key answered to before it was declared, and why.
var legacyEnvNames = map[string]envAlias{}

// envAlias is one variable a key still answers to, and the reason it does.
type envAlias struct {
	name string
	why  string
}

// DeclareLegacyEnvName records an environment variable a key answered to before this registry named it.
//
// Every declared key has exactly one canonical variable, derived from the key so a section cannot carry a
// spelling its variable does not match. A key that reached its reader through some other variable before
// it was declared has an operator somewhere with that variable set, and declaring the key would silence
// them without a word.
//
// The canonical name wins where both are set, because one of them is the name this binary documents. The
// legacy one is reported rather than applied silently, which is what makes it possible to know whether
// anyone still depends on it.
//
// This is the one declaration whose purpose is to stop being true. Every other one states something
// permanent about a reader; this one states a compatibility the key space has not finished absorbing, and
// left alone it becomes a second permanent spelling, which is the drift the registry exists to prevent.
// There is no removal date, so the reason says what it is for and doctor's report is what says whether it
// is still earning its place.
func DeclareLegacyEnvName(section, key, name, why string) {
	mu.Lock()
	defer mu.Unlock()
	switch {
	case name == "":
		defects = append(defects, Defect{Section: section, Err: fmt.Errorf(
			"declared an empty legacy environment variable for %q", key)})
	case why == "":
		defects = append(defects, Defect{Section: section, Err: fmt.Errorf(
			"declared the legacy variable %s for %q with no reason; without one nothing says what it is "+
				"for, and a compatibility nobody can explain is one nobody can remove", name, key)})
	case name == EnvName(key):
		defects = append(defects, Defect{Section: section, Err: fmt.Errorf(
			"declared %s as a legacy variable for %q and that is already its canonical name", name, key)})
	default:
		legacyEnvNames[key] = envAlias{name: name, why: why}
	}
}

// LegacyEnvName returns the variable a key still answers to, and whether it has one.
func LegacyEnvName(key string) (string, bool) {
	mu.RLock()
	defer mu.RUnlock()
	alias, ok := legacyEnvNames[key]
	return alias.name, ok
}

// LegacyEnvNameKeys returns the keys carrying such a variable, sorted.
func LegacyEnvNameKeys() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(legacyEnvNames))
	for key := range legacyEnvNames {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// conflictingFlagRefused holds the keys where a typed flag may not disagree with the file, and why.
var conflictingFlagRefused = map[string]string{}

// RefuseConflictingFlag records that a typed flag disagreeing with the file is a mistake for this key.
//
// A flag beating a file is ordinarily the point: an operator typed it, so it wins. For a few settings the
// disagreement is evidence of a mistake rather than an intent, because the two values answer a question
// with one right answer and the operator cannot have meant both. The chain a node joins is that: a flag
// naming one chain and a file naming another is a node about to run somewhere nobody chose.
//
// Refused rather than reported. This is the one thing the resolution stops a boot for, and the reason it
// may is that the path it replaces stops for the same input: the upstream start command panics when a
// typed chain-id disagrees with the client configuration. Declaring the key moved that comparison out of
// reach, because the flag now feeds the value the check reads, so the check can never see a difference.
// This puts it back where both sides are still visible.
func RefuseConflictingFlag(section, key, why string) {
	mu.Lock()
	defer mu.Unlock()
	if why == "" {
		defects = append(defects, Defect{Section: section, Err: fmt.Errorf(
			"refused a conflicting flag for %q with no reason; a boot this stops needs to say what "+
				"question the two values disagree about", key)})
		return
	}
	conflictingFlagRefused[key] = why
}

// ConflictingFlagRefused returns the reason a key refuses a disagreeing flag, and whether it does.
func ConflictingFlagRefused(key string) (string, bool) {
	mu.RLock()
	defer mu.RUnlock()
	why, ok := conflictingFlagRefused[key]
	return why, ok
}

// ConflictingFlagKeys returns those keys, sorted.
func ConflictingFlagKeys() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(conflictingFlagRefused))
	for key := range conflictingFlagRefused {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
