package wire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// FlexTime is the Go type for every Protect `date-time` field (wired via
// type-mapping in oapi-codegen.yaml). The controller is inconsistent: some
// fields come back as epoch-millisecond numbers (automation createdAt), others
// as RFC 3339 strings or null. A plain time.Time rejects the numeric form, and
// since encoding/json fails the whole object on one bad field, that single
// mismatch blocks every plan/apply that reads the resource. FlexTime accepts all
// three forms.
type FlexTime struct{ time.Time }

// UnmarshalJSON accepts a JSON null (-> zero), an RFC 3339 string, or a bare
// number read as Unix milliseconds (Protect's timestamp unit).
func (t *FlexTime) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		t.Time = time.Time{}
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" {
			t.Time = time.Time{}
			return nil
		}
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return err
		}
		t.Time = parsed
		return nil
	}
	ms, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return fmt.Errorf("wire: FlexTime: cannot decode %s as time: %w", b, err)
	}
	t.Time = time.UnixMilli(ms).UTC()
	return nil
}

// MarshalJSON emits RFC 3339 (or null for the zero value). Protect date-time
// fields are decode-only in this provider, so this exists only to keep the type
// a well-behaved json.Marshaler.
func (t FlexTime) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.UTC().Format(time.RFC3339))
}
