package models

// Site is an Integration v1 site object (GET /integration/v1/sites).
//
// Three namespaces name the same site: the Integration API keys by UUID (ID),
// legacy/v2 key by the slug in InternalReference (e.g. "default"), and Name is
// the human label ("Default"). The provider matches on InternalReference
// (architecture.md §5).
type Site struct {
	ID                string `json:"id"`
	InternalReference string `json:"internalReference"`
	Name              string `json:"name"`
}

// LegacySite is the object the legacy `/cmd/sitemgr` returns for add-site:
// Name is the slug (the v2/legacy site key, == Site.InternalReference) and Desc
// is the human display name. ID is the Mongo `_id` used by delete/update-site.
type LegacySite struct {
	ID   string `json:"_id"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}
