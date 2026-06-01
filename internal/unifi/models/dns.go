package models

// StaticDNSRecord is a v2 `/static-dns` local DNS record. The v2 surface is
// bare JSON (no envelope). On 10.1+ firmware the router may move local DNS to
// the Integration dns/policies surface with no schema change (invariant #2);
// this struct models the v2 wire shape used today.
type StaticDNSRecord struct {
	ID         string `json:"_id,omitempty"`
	Key        string `json:"key"`               // the DNS name, e.g. "host.example.com"
	RecordType string `json:"record_type"`       // A | AAAA | CNAME | MX | TXT | SRV | NS
	Value      string `json:"value"`             // the target (IP / hostname / text)
	Enabled    *bool  `json:"enabled,omitempty"` // pointer: omit vs explicit false
	TTL        *int   `json:"ttl,omitempty"`     // 0 = controller default
	Port       *int   `json:"port,omitempty"`    // SRV
	Priority   *int   `json:"priority,omitempty"`
	Weight     *int   `json:"weight,omitempty"`
}
