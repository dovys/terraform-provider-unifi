package provider

import "testing"

func TestImportName(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		isName bool
	}{
		{"name=WAN1", "WAN1", true},
		{"name= WAN1 ", "WAN1", true}, // value is trimmed
		{"name=My Net", "My Net", true},
		{"name=", "", true},                     // name form, empty value (caller rejects)
		{"6151c3a2e4b0f1234567890a", "", false}, // raw _id → not a name form
		{"", "", false},
	}
	for _, c := range cases {
		got, isName := importName(c.in)
		if got != c.want || isName != c.isName {
			t.Errorf("importName(%q) = (%q, %v), want (%q, %v)", c.in, got, isName, c.want, c.isName)
		}
	}
}
