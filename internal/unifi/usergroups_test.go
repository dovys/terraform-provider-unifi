package unifi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func iptr(i int) *int { return &i }

func TestUserGroupCRUD_AndGetMergePUT(t *testing.T) {
	var lastPut map[string]any
	mux := http.NewServeMux()

	mux.HandleFunc("/proxy/network/api/s/default/rest/usergroup", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"ug1","name":"Default","attr_no_delete":true}]}`))
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"ug2","name":"Capped","qos_rate_max_up":5000,"qos_rate_max_down":20000}]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/proxy/network/api/s/default/rest/usergroup/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// Unknown id → the controller echoes an empty data array (→ ErrNotFound).
			if !strings.HasSuffix(r.URL.Path, "/ug2") {
				_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
				return
			}
			// attr_no_delete is controller-managed; it must survive the merge-PUT.
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"ug2","name":"Capped","qos_rate_max_up":5000,"qos_rate_max_down":20000,"attr_no_delete":false}]}`))
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &lastPut)
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[` + string(body) + `]}`))
		case http.MethodDelete:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	c, srv := newTestClient(t, mux)
	defer srv.Close()
	ctx := context.Background()

	groups, err := c.ListUserGroups(ctx)
	if err != nil || len(groups) != 1 || groups[0].Name != "Default" {
		t.Fatalf("ListUserGroups = %+v, err %v", groups, err)
	}
	if groups[0].AttrNoDelete == nil || !*groups[0].AttrNoDelete {
		t.Errorf("default group attr_no_delete = %v, want true", groups[0].AttrNoDelete)
	}

	created, err := c.CreateUserGroup(ctx, &UserGroup{
		Name: "Capped", QOSRateMaxUp: iptr(5000), QOSRateMaxDown: iptr(20000),
	})
	if err != nil || created.ID != "ug2" {
		t.Fatalf("CreateUserGroup = %+v, err %v", created, err)
	}
	if created.QOSRateMaxUp == nil || *created.QOSRateMaxUp != 5000 {
		t.Errorf("created qos_rate_max_up = %v, want 5000", created.QOSRateMaxUp)
	}

	got, err := c.GetUserGroup(ctx, "ug2")
	if err != nil || got.Name != "Capped" {
		t.Fatalf("GetUserGroup(ug2) = %+v, err %v", got, err)
	}
	if _, err := c.GetUserGroup(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetUserGroup(nope) err = %v, want ErrNotFound", err)
	}

	if _, err := c.UpdateUserGroup(ctx, &UserGroup{ID: "ug2", Name: "Renamed", QOSRateMaxUp: iptr(10000)}); err != nil {
		t.Fatalf("UpdateUserGroup: %v", err)
	}
	// GET-merge-PUT overwrites owned fields and preserves controller-managed ones.
	if lastPut["name"] != "Renamed" {
		t.Errorf("PUT name = %v, want Renamed", lastPut["name"])
	}
	if lastPut["qos_rate_max_up"] != float64(10000) {
		t.Errorf("PUT qos_rate_max_up = %v, want 10000", lastPut["qos_rate_max_up"])
	}
	if _, ok := lastPut["attr_no_delete"]; !ok {
		t.Errorf("merge-PUT dropped controller-managed attr_no_delete; body = %v", lastPut)
	}

	// UpdateUserGroup without an id is a programming error, not a network call.
	if _, err := c.UpdateUserGroup(ctx, &UserGroup{Name: "x"}); err == nil {
		t.Errorf("UpdateUserGroup with empty id should error")
	}

	if err := c.DeleteUserGroup(ctx, "ug2"); err != nil {
		t.Fatalf("DeleteUserGroup: %v", err)
	}
}
