package configcli

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sei-protocol/sei-chain/config/registry"
	"github.com/sei-protocol/sei-chain/config/seitoml"
	"github.com/sei-protocol/sei-chain/sei-cosmos/version"
)

// Source is an existing node's resolved configuration, read by key.
//
// Declared here rather than imported, so this package does not depend on whichever type happens to
// carry a node's configuration today. Anything that enumerates its keys and answers for one
// satisfies it.
type Source interface {
	AllKeys() []string
	Get(string) any
}

// Existing is everything a running node resolves its configuration from.
//
// Three layers, in the order the node itself consults them. Named as a type rather than passed as
// three arguments because the order is the whole point, and because a layer left out of a call is a
// value the adopted file silently gets wrong.
type Existing struct {
	// Files reads app.toml and config.toml.
	Files Source
	// FlagDefault reads the default of a flag bound to a key, which answers when the files do not.
	FlagDefault func(key string) (string, bool)
	// LookupEnv reads an environment variable. Reported, never written.
	LookupEnv func(name string) (string, bool)
}

// Adoption is the result of carrying an existing node's configuration into a sei.toml.
type Adoption struct {
	// File is the document produced. Not written anywhere until the caller saves it.
	File *seitoml.File
	// Carried are the keys whose value came from app.toml or config.toml, sorted.
	Carried []string
	// FromFlagDefault are the keys the files did not carry and a bound flag's default supplied,
	// sorted.
	FromFlagDefault []string
	// Unsupplied are the keys nothing supplied, sorted. Each is written at the value the node resolves
	// when the key is absent everywhere, which is not always this binary's default for it.
	Unsupplied []string
	// Environment are keys an environment variable supplies, sorted. Reported, never written.
	Environment []string
	// Unconvertible are keys whose existing value could not be read as the declared type, sorted.
	// Each is written at its absent value instead, so nothing silently becomes a value nobody chose.
	Unconvertible []Rejection
}

// Rejection is one existing value that could not be carried over.
type Rejection struct {
	// Key is the dotted key.
	Key string
	// Value is what the existing configuration held.
	Value any
	// Reason says why it could not be read as the declared type.
	Reason string
}

// Adopt builds a sei.toml from a node's existing configuration.
//
// Every key is written at the value the node resolves for it today, so a node already running keeps
// what it runs instead of moving onto defaults unannounced. That is the whole difference between
// adopting and generating, and it is why a key nothing supplies is written at its absent value rather
// than at this binary's default: for a key whose reader assigns straight from its lookup, those two
// differ, and the default is one the node has never run.
//
// This reports environment variables and never folds them into the file. They sit above the file in
// the resolution order, so writing one in changes nothing while it is set and changes the node's
// behaviour, unannounced, the day somebody unsets it.
//
// A value this cannot read as its declared type is written at its absent value, and the result names
// it. Writing it anyway produces a file the node refuses at its next boot, and dropping it quietly
// moves the node onto a value nobody chose.
func Adopt(node Existing, mode registry.Mode) (Adoption, error) {
	if node.Files == nil {
		return Adoption{}, fmt.Errorf("no existing configuration to adopt from")
	}
	if err := knownMode(mode); err != nil {
		return Adoption{}, err
	}
	resolved, err := registry.Resolve(mode)
	if err != nil {
		return Adoption{}, fmt.Errorf("resolve the baselines for mode %q: %w", mode, err)
	}
	if len(resolved.Keys) == 0 {
		return Adoption{}, fmt.Errorf("no section has registered a key, so there is nothing to adopt")
	}
	absent, err := registry.AbsentValues(mode)
	if err != nil {
		return Adoption{}, fmt.Errorf("resolve the absent values for mode %q: %w", mode, err)
	}
	if len(absent) != len(resolved.Keys) {
		return Adoption{}, fmt.Errorf("the registry resolves %d key(s) and names an absent value for "+
			"%d, so some key has no value to write when nothing supplies it",
			len(resolved.Keys), len(absent))
	}

	file, err := seitoml.New(string(mode), version.Version)
	if err != nil {
		return Adoption{}, err
	}
	out := Adoption{File: file}

	for _, key := range sortedKeys(resolved) {
		d := declared{key: key, typ: reflect.TypeOf(resolved.Keys[key].Value), absent: absent[key]}
		if err := file.Set(key, adoptedValue(node, d, &out)); err != nil {
			return Adoption{}, fmt.Errorf("write %s: %w", key, err)
		}
	}

	file.SetPreamble(adoptionPreamble(mode, out))
	sort.Strings(out.Carried)
	sort.Strings(out.FromFlagDefault)
	sort.Strings(out.Unsupplied)
	sort.Strings(out.Environment)
	sort.Slice(out.Unconvertible, func(i, j int) bool {
		return out.Unconvertible[i].Key < out.Unconvertible[j].Key
	})
	return out, nil
}

// declared is what the registry says about one key.
type declared struct {
	// key is the dotted key.
	key string
	// typ is the declared type, taken from the baseline, and is what an existing value is read as.
	typ reflect.Type
	// absent is the value the node resolves when nothing supplies the key.
	absent any
}

// adoptedValue decides one key's value and records where it came from.
//
// The layers are consulted in the order the node consults them. A configuration file beats a bound
// flag's default, because the resolution reaches that default only when nothing else answered.
func adoptedValue(node Existing, d declared, out *Adoption) any {
	// The environment stays separate from the three: a variable is either set or not regardless of what
	// the other layers hold, and it is the one that wins today.
	if _, set := node.lookupEnv(registry.EnvName(d.key)); set {
		out.Environment = append(out.Environment, d.key)
	}

	if raw := node.Files.Get(d.key); raw != nil {
		value, err := coerce(raw, d.typ)
		if err != nil {
			return out.refuse(d, raw, err)
		}
		out.Carried = append(out.Carried, d.key)
		return value
	}
	if text, bound := node.flagDefault(d.key); bound {
		value, err := coerceText(text, d.typ)
		if err != nil {
			return out.refuse(d, text, err)
		}
		out.FromFlagDefault = append(out.FromFlagDefault, d.key)
		return value
	}
	out.Unsupplied = append(out.Unsupplied, d.key)
	return d.absent
}

// refuse records a value that could not be read as its declared type and answers with the key's
// absent value instead.
func (a *Adoption) refuse(d declared, raw any, err error) any {
	a.Unconvertible = append(a.Unconvertible, Rejection{Key: d.key, Value: raw, Reason: err.Error()})
	return d.absent
}

// flagDefault reads a bound flag's default, answering for nothing when no flag set was supplied.
func (e Existing) flagDefault(key string) (string, bool) {
	if e.FlagDefault == nil {
		return "", false
	}
	return e.FlagDefault(key)
}

// lookupEnv reads an environment variable, answering for nothing when no environment was supplied.
func (e Existing) lookupEnv(name string) (string, bool) {
	if e.LookupEnv == nil {
		return "", false
	}
	return e.LookupEnv(name)
}

// adoptionPreamble is what the adopted file says about itself.
func adoptionPreamble(mode registry.Mode, out Adoption) []string {
	lines := []string{
		fmt.Sprintf(" Adopted from this node's existing configuration, for %q mode.", mode),
		"",
		" Every value below is the one this node resolves today:",
		fmt.Sprintf("   %d came from the app.toml or config.toml it was already running,",
			len(out.Carried)),
		fmt.Sprintf("   %d from the default of a start flag bound to that setting,",
			len(out.FromFlagDefault)),
		fmt.Sprintf("   %d from this binary, because nothing supplied them.", len(out.Unsupplied)),
		"",
		" Every key below is a written value, which this binary treats as your decision and never",
		" rewrites, so this node keeps them across an upgrade.",
	}
	if len(out.Environment) > 0 {
		lines = append(lines,
			"",
			fmt.Sprintf(" %d setting(s) are supplied by environment variables and were deliberately",
				len(out.Environment)),
			" not written here. An environment variable overrides this file, so writing it in would",
			" change nothing now and would change how this node runs the day it is unset.")
	}
	return lines
}

// coerceText reads a bound flag's default as the declared type.
//
// Separate from coerce because the two read different things. A value out of a configuration file
// arrives already typed, so text where a number is declared is an operator's mistake and worth
// reporting. A flag's default is text whatever the flag's type, so reading it is all there is to do.
//
// A list is refused rather than parsed. pflag renders one as text of its own making, nothing this
// binary declares needs it, and a reading written for no caller would have its rules decided by
// guessing.
func coerceText(text string, want reflect.Type) (any, error) {
	if want == nil {
		return nil, fmt.Errorf("the key has no declared type")
	}
	if want == reflect.TypeOf(time.Duration(0)) {
		return coerceDuration(text)
	}
	switch want.Kind() {
	case reflect.Bool:
		b, err := strconv.ParseBool(text)
		if err != nil {
			return nil, fmt.Errorf("the flag's default %q is not true or false", text)
		}
		return b, nil
	case reflect.String:
		return text, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return coerceWholeNumber(text)
	case reflect.Float32, reflect.Float64:
		return coerceFloat(text)
	default:
		return nil, fmt.Errorf("a %s cannot be read from a flag's default", want)
	}
}

// coerce reads an existing configuration value as the declared type.
//
// Existing values arrive from a file or a flag binding, so a number may be a string and a duration
// may be text. Converting through the declared type is what keeps an adopted file readable by the
// node, and refusing is what keeps a value nobody can read out of it.
func coerce(raw any, want reflect.Type) (any, error) {
	if want == nil {
		return nil, fmt.Errorf("the key has no declared type")
	}
	// A value already of the declared type passes straight through.
	if reflect.TypeOf(raw) == want {
		return raw, nil
	}
	if want == reflect.TypeOf(time.Duration(0)) {
		return coerceDuration(raw)
	}
	switch want.Kind() {
	case reflect.Bool:
		if b, ok := raw.(bool); ok {
			return b, nil
		}
		return nil, fmt.Errorf("%#v is not true or false", raw)
	case reflect.String:
		if s, ok := raw.(string); ok {
			return s, nil
		}
		return nil, fmt.Errorf("%#v is not text", raw)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return coerceWholeNumber(raw)
	case reflect.Float32, reflect.Float64:
		return coerceFloat(raw)
	case reflect.Slice:
		if want.Elem().Kind() != reflect.String {
			return nil, fmt.Errorf("a list of %s cannot be carried over", want.Elem())
		}
		return coerceStringList(raw)
	default:
		return nil, fmt.Errorf("a %s cannot be carried over", want)
	}
}

// coerceDuration reads a duration, refusing a bare number.
//
// A unit-less number is the coercion the legacy path performs silently, reading it as nanoseconds,
// which turns an intended 30 seconds into 30 billionths of one.
func coerceDuration(raw any) (any, error) {
	s, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("%#v is not a duration; a duration needs a unit, as in 30s", raw)
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return nil, fmt.Errorf("%q is not a duration; a duration needs a unit, as in 30s", s)
	}
	return d, nil
}

// coerceWholeNumber reads a whole number, refusing anything with a fractional part.
func coerceWholeNumber(raw any) (any, error) {
	switch n := raw.(type) {
	case int:
		return int64(n), nil
	case int32:
		return int64(n), nil
	case int64:
		return n, nil
	case uint64:
		return n, nil
	case string:
		v, err := strconv.ParseInt(n, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a whole number", n)
		}
		return v, nil
	case float64:
		if n != float64(int64(n)) {
			return nil, fmt.Errorf("%v has a fractional part and the key is a whole number", n)
		}
		return int64(n), nil
	default:
		return nil, fmt.Errorf("%#v is not a whole number", raw)
	}
}

// coerceFloat reads a number.
func coerceFloat(raw any) (any, error) {
	switch n := raw.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case string:
		v, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a number", n)
		}
		return v, nil
	default:
		return nil, fmt.Errorf("%#v is not a number", raw)
	}
}

// coerceStringList reads a list of strings.
func coerceStringList(raw any) (any, error) {
	switch v := raw.(type) {
	case []string:
		return v, nil
	case string:
		return splitList(v), nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("the list holds %#v, which is not text", item)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%#v is not a list of text", raw)
	}
}

// Report renders an adoption for an operator, most actionable first.
func (a Adoption) Report() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Wrote what this node resolves today: %d setting(s) from its "+
		"configuration files, %d from a bound flag's default, %d that nothing supplied.\n",
		len(a.Carried), len(a.FromFlagDefault), len(a.Unsupplied)))

	if len(a.Unconvertible) > 0 {
		b.WriteString(fmt.Sprintf("\n%d existing value(s) could not be read as the setting's type. "+
			"Each was written at what an absent key resolves to, so check them before starting the "+
			"node:\n", len(a.Unconvertible)))
		for _, r := range a.Unconvertible {
			b.WriteString(fmt.Sprintf("  %s held %#v: %s\n", r.Key, r.Value, r.Reason))
		}
	}
	if len(a.Environment) > 0 {
		b.WriteString(fmt.Sprintf("\n%d setting(s) come from environment variables and were not "+
			"written to the file. They override it, so the file cannot record them:\n",
			len(a.Environment)))
		for _, k := range a.Environment {
			b.WriteString(fmt.Sprintf("  %s (%s)\n", k, registry.EnvName(k)))
		}
	}
	return b.String()
}
