package seitoml

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

// What a float written to a file reads back as.
//
// A key the registry declares as a float is resolved by casting whatever the file holds, so a value
// that reads back as an integer is a type the rest of the boot did not ask for. TOML separates the two
// by the fractional part or the exponent, and the shortest form of a whole float has neither, so this
// is the case that fails without anyone writing a wrong value.

// TestEveryFloatReadsBackAsAFloat pins the round trip through the file.
func TestEveryFloatReadsBackAsAFloat(t *testing.T) {
	// Whole numbers first, because they are the ones a shortest-form renderer turns into integers, and
	// the fractional and exponent values beside them are what makes the whole set the claim.
	for _, want := range []float64{
		0, 1, -1, 2, 100, 1e6,
		0.1, 0.5, 1.5, 2.25, -0.75,
		1e21, 1e-7, math.MaxFloat64, math.SmallestNonzeroFloat64,
	} {
		got := roundTrip(t, want)

		asFloat, isFloat := got.(float64)
		if !isFloat {
			t.Errorf("%v was written and read back as %T (%v). A key declared as a float then resolves "+
				"as one type from a node's own files and another from its sei.toml", want, got, got)
			continue
		}
		if asFloat != want {
			t.Errorf("%v was written and read back as %v", want, asFloat)
		}
	}
}

// TestAFloatWithNoTOMLFormIsRefusedWithAReason holds the refusal to stating why.
//
// Asserting only that the write fails would prove nothing this package does. The TOML parser has no
// form for an infinity either, so it refuses one on its own and a test satisfied by any error passes
// whether this package looks at the value or not. What is this package's own is the reason: no key the
// registry declares means anything at infinity, so the refusal is a decision here rather than a limit
// inherited from whichever parser is underneath.
func TestAFloatWithNoTOMLFormIsRefusedWithAReason(t *testing.T) {
	for _, value := range []float64{math.Inf(1), math.Inf(-1), math.NaN()} {
		file := newFile(t)
		err := file.Set("mempool.drop-utilisation-threshold", value)
		if err == nil {
			raw, bytesErr := file.Bytes()
			t.Errorf("%v was accepted, and rendering gave %q (err %v). No reader can load that",
				value, raw, bytesErr)
			continue
		}
		if !strings.Contains(err.Error(), "finite") {
			t.Errorf("%v was refused as %q, which is the parser's own complaint rather than a statement "+
				"that a configuration file holds finite numbers", value, err)
		}
	}
}

// roundTrip writes a value, renders the file, reads it back and returns what the key holds.
func roundTrip(t *testing.T, value float64) any {
	t.Helper()
	const key = "mempool.drop-utilisation-threshold"

	file := newFile(t)
	if err := file.Set(key, value); err != nil {
		t.Fatalf("set %s to %v: %v", key, value, err)
	}
	raw, err := file.Bytes()
	if err != nil {
		t.Fatalf("render the file holding %v: %v", value, err)
	}

	// Read through Parse rather than by inspecting the document in memory. The claim is about what a
	// node starting up gets, and that node reads bytes.
	reloaded, err := Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("reload the file holding %v: %v\n%s", value, err, raw)
	}
	got, present, err := reloaded.Get(key)
	if err != nil {
		t.Fatalf("read %s back: %v", key, err)
	}
	if !present {
		t.Fatalf("%s was written and the reloaded file does not carry it:\n%s", key, raw)
	}
	return got
}

// newFile returns an empty file to write into.
func newFile(t *testing.T) *File {
	t.Helper()
	file, err := New("full", "test")
	if err != nil {
		t.Fatalf("new file: %v", err)
	}
	return file
}
