//go:build acc

package acc

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Tier-2 stack lifecycle, owned by the Go test harness (TestMain), not by the
// shell — test-extended.sh only triggers `go test` with UNIFI_T2_MANAGE=1. When
// managed, the harness boots the Docker controller + Mongo + UniFi-OS bridge
// (test/tier2/compose.yaml), waits for readiness, seeds a local admin, and
// exports the UNIFI_* connection env the provider reads. The whole stack is a
// throwaway, network-isolated controller, so the credentials are fixed.
const (
	tier2Project = "tfunifi"
	tier2Host    = "http://localhost:8443"
	tier2User    = "tfadmin"
	tier2Pass    = "tfadmin-pw-12345"
	// SHA-512 crypt of tier2Pass (openssl passwd -6 -salt tfacctier2seed …),
	// embedded so seeding needs no openssl at runtime. Matches tier2Pass.
	tier2Shadow = `$6$tfacctier2seed$fPgTvtRri8hHFU8PJNE9xVjiPp1hAXWGR2KIb.SijminpCAgjRO9B90z3Ap4nj1jHf4GR5IzU6afnx69mTSwd.`
)

func tier2Managed() bool { return os.Getenv("UNIFI_T2_MANAGE") == "1" }

func tier2ComposePath() string {
	_, self, _, _ := runtime.Caller(0) // test/acc/harness_test.go
	return filepath.Join(filepath.Dir(self), "..", "tier2", "compose.yaml")
}

func tier2Compose(args ...string) *exec.Cmd {
	return exec.Command("docker", append([]string{"compose", "-p", tier2Project, "-f", tier2ComposePath()}, args...)...)
}

func tier2Logf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "\033[36m[tier2]\033[0m "+format+"\n", a...)
}

// tier2Up boots the stack (unless it is already serving), waits for the
// controller, seeds the admin, and exports the connection env. Lifecycle stays
// in Go; the bridge/compose files are just the artifacts it drives.
func tier2Up() error {
	if !tier2StatusUp() {
		tier2Logf("booting stack (controller + mongo + bridge)…")
		if out, err := tier2Compose("up", "-d").CombinedOutput(); err != nil {
			return fmt.Errorf("compose up: %w\n%s", err, out)
		}
		if err := tier2WaitReady(); err != nil {
			return err
		}
	} else {
		tier2Logf("stack already serving; re-seeding")
	}
	if err := tier2SeedAdmin(); err != nil {
		return err
	}
	for k, v := range map[string]string{
		"UNIFI_HOST": tier2Host, "UNIFI_USERNAME": tier2User,
		"UNIFI_PASSWORD": tier2Pass, "UNIFI_SITE": "default",
		"UNIFI_ALLOW_INSECURE": "true",
	} {
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
	return nil
}

func tier2StatusUp() bool {
	c := &http.Client{Timeout: 4 * time.Second}
	resp, err := c.Get(tier2Host + "/status")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return strings.Contains(string(b), `"up":true`)
}

// tier2WaitReady blocks until the controller answers and first-run init has
// created the sites the seed references (cold boot can exceed 5 minutes).
func tier2WaitReady() error {
	tier2Logf("waiting for controller (cold boot is slow)…")
	deadline := time.Now().Add(12 * time.Minute)
	for time.Now().Before(deadline) {
		if tier2StatusUp() && strings.TrimSpace(tier2Mongo(`db.site.countDocuments({name:"default"})`)) == "1" {
			tier2Logf("controller ready")
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	out, _ := tier2Compose("logs", "--tail=30", "controller").CombinedOutput()
	return fmt.Errorf("controller not ready in time\n%s", out)
}

func tier2Mongo(js string) string {
	out, _ := exec.Command("docker", "exec", tier2Project+"-mongo-1",
		"mongosh", "--quiet", "unifi", "--eval", js).Output()
	return string(out)
}

// tier2SeedAdmin inserts (replacing any prior) the local admin + its super/site
// privileges the way the controller stores them — the standalone controller has
// no headless add-admin command — then confirms login through the bridge.
func tier2SeedAdmin() error {
	tier2Logf("seeding local admin %q…", tier2User)
	js := fmt.Sprintf(`
		db.admin.find({name:%[1]q}).forEach(a => db.privilege.deleteMany({admin_id:a._id.toString()}));
		db.admin.deleteMany({name:%[1]q});
		var sup = db.site.findOne({name:"super"})._id.toString();
		var def = db.site.findOne({name:"default"})._id.toString();
		var a = db.admin.insertOne({
			name:%[1]q, email:%[1]q+"@example.com", x_shadow:%[2]q,
			time_created: Math.floor(Date.now()/1000), last_site_name:"default",
			requires_new_password:false, is_verified:true, ui_settings:{}
		});
		var aid = a.insertedId.toString();
		db.privilege.insertOne({admin_id:aid, site_id:sup, role:"admin", permissions:[], is_super:true});
		db.privilege.insertOne({admin_id:aid, site_id:def, role:"admin", permissions:[]});
	`, tier2User, tier2Shadow)
	if out := tier2Mongo(js); strings.Contains(out, "Error") {
		return fmt.Errorf("seed admin: %s", out)
	}
	if !tier2LoginOK() {
		return fmt.Errorf("admin login through the bridge failed after seeding")
	}
	tier2Logf("admin ready, login OK")
	return nil
}

func tier2LoginOK() bool {
	body := strings.NewReader(fmt.Sprintf(`{"username":%q,"password":%q,"rememberMe":true}`, tier2User, tier2Pass))
	resp, err := http.Post(tier2Host+"/api/auth/login", "application/json", body)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return strings.Contains(string(b), `"rc":"ok"`)
}
