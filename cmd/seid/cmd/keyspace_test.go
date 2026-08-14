package cmd_test

import (
	"strings"
	"testing"

	"github.com/sei-protocol/sei-chain/config/keyspace"
)

// TestThisBinarySeesEverySectionItDeclares is what makes the registration import load-bearing.
//
// A section reaches the registry through its owning package's initialisation, so the set follows the
// import graph. Two sections were arriving only because this package imports their owner for an
// unrelated reason; dropping that use would have removed their keys from every diagnostic and from
// what a booting node installs, and nothing would have failed.
func TestThisBinarySeesEverySectionItDeclares(t *testing.T) {
	if drift := keyspace.Drift(); len(drift) > 0 {
		t.Fatalf("the key space this binary registers is not the one config/keyspace names:\n  %s\n\n"+
			"A section named there and not registered has its keys resolving through the machinery that "+
			"answered them before the registry existed, which reports nothing. One registered and not "+
			"named there is one nothing would notice leaving again", strings.Join(drift, "\n  "))
	}
}
