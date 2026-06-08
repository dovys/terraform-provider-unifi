//go:build hil

package hil

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestMain wires resource.TestMain so `-sweep` runs the registered tfacc- sweepers
// (run.sh/write.sh use it as the pre/post safety net). HIL runs against the
// operator's live console, not a managed stack, so there is no lifecycle to own.
func TestMain(m *testing.M) {
	resource.TestMain(m)
}
