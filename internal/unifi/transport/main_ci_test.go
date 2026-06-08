package transport

import (
	"os"
	"testing"
)

// TestMain disables login spacing for the unit suite. The global throttle would
// otherwise add real seconds between the many mock-server logins these tests do;
// the spacing behaviour itself is covered by loginthrottle_ci_test.go.
func TestMain(m *testing.M) {
	globalLoginThrottle.setIntervalForTest(0)
	os.Exit(m.Run())
}
