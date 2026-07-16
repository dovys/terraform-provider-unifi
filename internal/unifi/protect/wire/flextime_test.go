package wire

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFlexTimeUnmarshal(t *testing.T) {
	rfc := "2023-11-14T22:13:19Z"
	want, _ := time.Parse(time.RFC3339, rfc)

	cases := []struct {
		name   string
		in     string
		wantMs int64 // -1 => zero time
	}{
		{"epoch-millis number", "1700000000000", 1700000000000},
		{"rfc3339 string", `"` + rfc + `"`, want.UnixMilli()},
		{"null", "null", -1},
		{"empty string", `""`, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ft FlexTime
			if err := json.Unmarshal([]byte(tc.in), &ft); err != nil {
				t.Fatalf("Unmarshal(%s): %v", tc.in, err)
			}
			if tc.wantMs < 0 {
				if !ft.IsZero() {
					t.Fatalf("Unmarshal(%s): want zero time, got %v", tc.in, ft.Time)
				}
				return
			}
			if got := ft.UnixMilli(); got != tc.wantMs {
				t.Fatalf("Unmarshal(%s): want %d ms, got %d", tc.in, tc.wantMs, got)
			}
		})
	}
}

// Regression for #127: the controller returns automation createdAt as an
// epoch-millis number, which a plain time.Time cannot decode — and because
// encoding/json fails the whole object on that one field, it blocked every
// plan/apply that read an automation. Decoding must now succeed.
func TestAutomationCreatedAtNumericTimestamp(t *testing.T) {
	const ms = 1700000000000
	var a Automation
	if err := json.Unmarshal([]byte(`{"id":"abc","name":"n","createdAt":1700000000000}`), &a); err != nil {
		t.Fatalf("decode automation with numeric createdAt: %v", err)
	}
	if got := a.CreatedAt.UnixMilli(); got != ms {
		t.Fatalf("CreatedAt: want %d ms, got %d", ms, got)
	}
}

func TestFlexTimeMarshal(t *testing.T) {
	if b, _ := json.Marshal(FlexTime{}); string(b) != "null" {
		t.Fatalf("zero FlexTime: want null, got %s", b)
	}
	tm, _ := time.Parse(time.RFC3339, "2023-11-14T22:13:19Z")
	if b, _ := json.Marshal(FlexTime{Time: tm}); string(b) != `"2023-11-14T22:13:19Z"` {
		t.Fatalf("set FlexTime: want RFC3339 string, got %s", b)
	}
}
