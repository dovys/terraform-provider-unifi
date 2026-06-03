package network

import (
	"context"
	"encoding/json"
	"net/http"
)

// Settings is the site-settings group. UniFi site settings are NOT REST CRUD:
// the controller stores one document per "key" section (mgmt, ntp, country, …)
// and exposes them through dedicated verbs rather than /rest — GET
// get/setting/<key> reads a section, POST set/setting/<key> writes one. A write
// is a shallow merge over the stored section (send only the fields to change;
// composite values whole), so callers pass an omitempty owned-field overlay
// rather than a whole document. Each section's field set is the controller's own
// validated writable set (see ../unifi-specs objects/setting.json).
func (c *Client) Settings() SettingCollection {
	return SettingCollection{core: c.core}
}

// SettingCollection reads and writes individual site-setting sections by key. It
// is deliberately type-agnostic: a section is carried as the raw stored document
// so the provider layer can map each key onto its own typed block without a wire
// type per section (the sections share one generated wire.Setting umbrella whose
// per-key sub-objects are anonymous and not separately addressable).
type SettingCollection struct {
	core *core
}

// GetKey returns the raw stored document for a setting section (the section's
// fields are flat in the document, alongside server-managed _id/site_id/key), or
// ErrNotFound if the controller has no such section.
func (s SettingCollection) GetKey(ctx context.Context, key string) (json.RawMessage, error) {
	docs, err := legacyData[json.RawMessage](ctx, s.core, http.MethodGet, s.core.legacyPath("get/setting/"+key), nil)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, ErrNotFound
	}
	return docs[0], nil
}

// SetKey writes a setting section: owned is an omitempty-tagged overlay carrying
// only the fields to change, which the controller shallow-merges over the stored
// section. It returns the controller's stored copy of the section after the
// write (re-reading if the write echoes nothing).
func (s SettingCollection) SetKey(ctx context.Context, key string, owned any) (json.RawMessage, error) {
	docs, err := legacyData[json.RawMessage](ctx, s.core, http.MethodPost, s.core.legacyPath("set/setting/"+key), owned)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return s.GetKey(ctx, key)
	}
	return docs[0], nil
}
