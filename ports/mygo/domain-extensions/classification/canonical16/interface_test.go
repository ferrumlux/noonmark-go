// SPDX-License-Identifier: AGPL-3.0-only
package canonical16_test

import (
	classification "github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/canonical16"
)

// Compile-time compatibility only; this is not runtime installation or evidence
// of the parent engine's workflow, audit, persistence or Swift equivalence.
var _ classification.NameCanonicalizer = canonical16.Canonicalizer{}
