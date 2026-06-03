package network

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSettingsGetKey(t *testing.T) {
	// get/setting/<key> returns the {meta,data:[doc]} envelope with a single,
	// flat section document.
	f := &fakeDoer{resp: okJSON(`{"meta":{"rc":"ok"},"data":[{"_id":"s1","site_id":"x","key":"locale","timezone":"America/New_York"}]}`)}
	raw, err := newTestClient(f).Settings().GetKey(context.Background(), "locale")
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	if f.method != "GET" {
		t.Errorf("method = %q, want GET", f.method)
	}
	if want := "/proxy/network/api/s/default/get/setting/locale"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode raw section: %v", err)
	}
	if doc["timezone"] != "America/New_York" {
		t.Errorf("timezone = %v, want America/New_York", doc["timezone"])
	}
}

func TestSettingsGetKeyNotFound(t *testing.T) {
	// An empty data array (no such section) maps to ErrNotFound.
	f := &fakeDoer{resp: okJSON(`{"meta":{"rc":"ok"},"data":[]}`)}
	if _, err := newTestClient(f).Settings().GetKey(context.Background(), "ntp"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSettingsSetKey(t *testing.T) {
	// SetKey POSTs the owned overlay to set/setting/<key>; omitempty drops the
	// fields the caller left nil (the shallow-merge contract: send only changes).
	f := &fakeDoer{resp: okJSON(`{"meta":{"rc":"ok"},"data":[{"_id":"s1","key":"ntp","setting_preference":"manual","ntp_server_1":"0.pool.ntp.org"}]}`)}
	type ntpOverlay struct {
		SettingPreference *string `json:"setting_preference,omitempty"`
		Server1           *string `json:"ntp_server_1,omitempty"`
		Server2           *string `json:"ntp_server_2,omitempty"`
	}
	pref, s1 := "manual", "0.pool.ntp.org"
	raw, err := newTestClient(f).Settings().SetKey(context.Background(), "ntp",
		&ntpOverlay{SettingPreference: &pref, Server1: &s1})
	if err != nil {
		t.Fatalf("SetKey: %v", err)
	}
	if f.method != "POST" {
		t.Errorf("method = %q, want POST", f.method)
	}
	if want := "/proxy/network/api/s/default/set/setting/ntp"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
	body, _ := json.Marshal(f.body)
	if !strings.Contains(string(body), `"setting_preference":"manual"`) || !strings.Contains(string(body), `"ntp_server_1":"0.pool.ntp.org"`) {
		t.Errorf("body missing configured fields: %s", body)
	}
	if strings.Contains(string(body), "ntp_server_2") {
		t.Errorf("nil overlay fields must be omitted, got: %s", body)
	}
	if !strings.Contains(string(raw), `"ntp_server_1":"0.pool.ntp.org"`) {
		t.Errorf("returned section = %s, want the stored doc", raw)
	}
}

func TestSettingsSetKeyReReadsOnEmptyEcho(t *testing.T) {
	// A write that echoes no data re-reads the section via get/setting/<key>.
	f := fakeQueue(
		`{"meta":{"rc":"ok"},"data":[]}`,                                             // POST set/setting/locale → empty echo
		`{"meta":{"rc":"ok"},"data":[{"_id":"s1","key":"locale","timezone":"UTC"}]}`, // GET re-read
	)
	raw, err := newTestClient(f).Settings().SetKey(context.Background(), "locale",
		&struct {
			Timezone *string `json:"timezone,omitempty"`
		}{Timezone: ptr("UTC")})
	if err != nil {
		t.Fatalf("SetKey: %v", err)
	}
	if f.calls != 2 {
		t.Errorf("calls = %d, want 2 (POST then re-read GET)", f.calls)
	}
	if !strings.Contains(string(raw), `"timezone":"UTC"`) {
		t.Errorf("re-read section = %s, want timezone UTC", raw)
	}
}
