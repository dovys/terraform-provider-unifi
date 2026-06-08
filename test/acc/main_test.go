//go:build acc

package acc

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestMain owns the Tier-2 stack in managed mode (UNIFI_T2_MANAGE=1): boot, wait,
// seed, export env — test-extended.sh only triggers it. The stack is left up
// after the run for fast reruns; `./test-extended.sh down` tears it down.
// Otherwise it just runs the registered sweepers.
func TestMain(m *testing.M) {
	if tier2Managed() {
		if err := tier2Up(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(m.Run())
	}
	resource.TestMain(m)
}
