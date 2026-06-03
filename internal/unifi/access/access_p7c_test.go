package access

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/access/wire"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// --- access policies ---

func TestAccessPolicyListPath(t *testing.T) {
	f := &fakeDoer{resp: env(`[{"Policy":{"unique_id":"p1","name":"Staff"},"active":true,"user_count":4}]`)}
	pols, err := newTestClient(f).AccessPolicies().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if f.method != http.MethodGet || f.path != "/proxy/access/api/v2/access_policies" {
		t.Errorf("List = %s %s, want GET .../access_policies", f.method, f.path)
	}
	if len(pols) != 1 || pols[0].Policy == nil || deref(pols[0].Policy.UniqueId) != "p1" {
		t.Fatalf("pols = %+v, want one policy p1", pols)
	}
}

func TestAccessPolicyGetPath(t *testing.T) {
	f := &fakeDoer{resp: env(`{"Policy":{"unique_id":"p1","name":"Staff"},"active":true}`)}
	p, err := newTestClient(f).AccessPolicies().Get(context.Background(), "p1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if want := "/proxy/access/api/v2/access_policy/p1"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
	if p.Policy == nil || deref(p.Policy.Name) != "Staff" {
		t.Errorf("name = %v, want Staff", p.Policy)
	}
}

func TestAccessPolicyCreateSendsBody(t *testing.T) {
	f := &fakeDoer{resp: env(`{"unique_id":"p9","name":"New"}`)}
	created, err := newTestClient(f).AccessPolicies().Create(context.Background(), wire.AccessPolicyBoAccessPolicyBo{
		Name:       ptr("New"),
		ScheduleId: ptr("sch1"),
		Configs:    &wire.AccessPolicyPoConfigs{IsActive: ptr(true)},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if f.method != http.MethodPost || f.path != "/proxy/access/api/v2/access_policy" {
		t.Errorf("Create = %s %s, want POST .../access_policy", f.method, f.path)
	}
	body, _ := json.Marshal(f.body)
	if !strings.Contains(string(body), `"name":"New"`) || !strings.Contains(string(body), `"schedule_id":"sch1"`) {
		t.Errorf("body missing fields: %s", body)
	}
	if deref(created.UniqueId) != "p9" {
		t.Errorf("created id = %v, want p9", created.UniqueId)
	}
}

func TestAccessPolicyUpdateAndDelete(t *testing.T) {
	f := &fakeDoer{resp: env(`{"unique_id":"p1","name":"Renamed"}`)}
	if _, err := newTestClient(f).AccessPolicies().Update(context.Background(), "p1", wire.AccessPolicyBoAccessPolicyBo{Name: ptr("Renamed")}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if f.method != http.MethodPut || f.path != "/proxy/access/api/v2/access_policy/p1" {
		t.Errorf("Update = %s %s, want PUT .../access_policy/p1", f.method, f.path)
	}
	f2 := &fakeDoer{err: &transport.APIError{StatusCode: http.StatusNotFound}}
	if err := newTestClient(f2).AccessPolicies().Delete(context.Background(), "gone"); err != nil {
		t.Errorf("Delete of absent policy: %v, want nil", err)
	}
}

// --- holiday groups ---

func TestHolidayGroupListAndGetPaths(t *testing.T) {
	f := &fakeDoer{resp: env(`[{"unique_id":"hg1","name":"US Holidays","holiday_count":11}]`)}
	groups, err := newTestClient(f).HolidayGroups().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if f.path != "/proxy/access/api/v2/holiday_groups" {
		t.Errorf("List path = %q", f.path)
	}
	if len(groups) != 1 || deref(groups[0].UniqueId) != "hg1" || groups[0].HolidayCount == nil || *groups[0].HolidayCount != 11 {
		t.Fatalf("groups = %+v", groups)
	}

	f2 := &fakeDoer{resp: env(`{"unique_id":"hg1","name":"US Holidays","holidays":[{"unique_id":"h1","name":"NYD"}]}`)}
	g, err := newTestClient(f2).HolidayGroups().Get(context.Background(), "hg1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if f2.path != "/proxy/access/api/v2/holiday_group/hg1" {
		t.Errorf("Get path = %q", f2.path)
	}
	if g.Holidays == nil || len(*g.Holidays) != 1 {
		t.Errorf("holidays = %+v, want one", g.Holidays)
	}
}

func TestHolidayGroupCreateSendsBody(t *testing.T) {
	f := &fakeDoer{resp: env(`{"unique_id":"hg9","name":"New"}`)}
	ids := []string{"h1", "h2"}
	created, err := newTestClient(f).HolidayGroups().Create(context.Background(), wire.HolidayBoHolidayGroupParams{
		Name:       ptr("New"),
		HolidayIds: &ids,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if f.method != http.MethodPost || f.path != "/proxy/access/api/v2/holiday_group" {
		t.Errorf("Create = %s %s", f.method, f.path)
	}
	body, _ := json.Marshal(f.body)
	if !strings.Contains(string(body), `"holiday_ids":["h1","h2"]`) {
		t.Errorf("body missing holiday_ids: %s", body)
	}
	if deref(created.UniqueId) != "hg9" {
		t.Errorf("created id = %v", created.UniqueId)
	}
}

// --- doors (update-only) ---

func TestDoorListAndGetFilter(t *testing.T) {
	list := `[{"Location":{"unique_id":"d1","name":"Front"},"is_open":false},{"Location":{"unique_id":"d2","name":"Back"}}]`
	f := &fakeDoer{resp: env(list)}
	got, err := newTestClient(f).Doors().Get(context.Background(), "d2")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if f.path != "/proxy/access/api/v2/locations/doors" {
		t.Errorf("List path = %q (Get lists+filters)", f.path)
	}
	if got.Location == nil || deref(got.Location.Name) != "Back" {
		t.Errorf("got = %+v, want door d2 Back", got)
	}

	f2 := &fakeDoer{resp: env(list)}
	if _, err := newTestClient(f2).Doors().Get(context.Background(), "nope"); err != ErrNotFound {
		t.Errorf("Get of absent door = %v, want ErrNotFound", err)
	}
}

func TestDoorUpdateSendsBody(t *testing.T) {
	f := &fakeDoer{resp: env(`{"Location":{"unique_id":"d1","name":"Renamed"}}`)}
	_, err := newTestClient(f).Doors().Update(context.Background(), "d1", wire.LocationBoUpdateDoorParams{
		DoorId:    ptr("d1"),
		DoorName:  ptr("Renamed"),
		ExtraType: ptr("standard"),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if f.method != http.MethodPut || f.path != "/proxy/access/api/v2/locations/door/d1" {
		t.Errorf("Update = %s %s, want PUT .../locations/door/d1", f.method, f.path)
	}
	body, _ := json.Marshal(f.body)
	if !strings.Contains(string(body), `"door_name":"Renamed"`) {
		t.Errorf("body missing door_name: %s", body)
	}
}

// --- door guards (update-only, keyed by door id) ---

func TestDoorGuardGetAndUpdatePaths(t *testing.T) {
	f := &fakeDoer{resp: env(`{"door_id":"d1","user_ids":["u1","u2"]}`)}
	dg, err := newTestClient(f).DoorGuards().Get(context.Background(), "d1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if f.method != http.MethodGet || f.path != "/proxy/access/api/v2/locations/door/d1/door_guard" {
		t.Errorf("Get = %s %s, want GET .../door/d1/door_guard", f.method, f.path)
	}
	if dg.UserIds == nil || len(*dg.UserIds) != 2 {
		t.Errorf("user_ids = %+v, want two", dg.UserIds)
	}

	f2 := &fakeDoer{resp: env(`{"door_id":"d1","user_ids":["u3"]}`)}
	ids := []string{"u3"}
	if _, err := newTestClient(f2).DoorGuards().Update(context.Background(), "d1", wire.DoorGuardBoUpdateDoorGuard{DoorId: ptr("d1"), UserIds: &ids}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if f2.method != http.MethodPut || f2.path != "/proxy/access/api/v2/locations/door/d1/door_guard" {
		t.Errorf("Update = %s %s", f2.method, f2.path)
	}
}

// deref is a tiny test helper mirroring the provider's pointer deref.
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
