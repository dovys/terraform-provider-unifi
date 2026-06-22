package protect

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect/wire"
)

// AutomationService is the automations group (/proxy/protect/api/automations).
// It is NOT a generic Collection: the route has no per-id GET/PUT/PATCH/DELETE
// (every by-id call 404s; DELETE wipes ALL automations). So reads are
// list-then-filter and writes go through a batch PATCH carrying a one-element
// array. The provider never creates or deletes an automation — it manages a
// field on one that already exists.
type AutomationService struct {
	core *core
}

// List returns every automation.
func (s AutomationService) List(ctx context.Context) ([]wire.Automation, error) {
	var out []wire.Automation
	if err := decode(ctx, s.core, http.MethodGet, s.core.apiPath("automations"), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Get returns the automation with the given id, or ErrNotFound — there is no
// per-id route, so it lists and filters. The ErrNotFound lets Read remove the
// resource from state when the automation was deleted out of band.
func (s AutomationService) Get(ctx context.Context, id string) (*wire.Automation, error) {
	list, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Id == id {
			return &list[i], nil
		}
	}
	return nil, ErrNotFound
}

// batchPatchItem is one entry of the batch-PATCH request array: an id and the
// automation fields to overwrite.
type batchPatchItem struct {
	ID   string `json:"id"`
	Body any    `json:"body"`
}

// batchPatchResult is one 207 entry: a per-item status that can report failure
// under an overall 207, plus the updated object.
type batchPatchResult struct {
	Status   int             `json:"status"`
	Response wire.Automation `json:"response"`
}

// Update overwrites the given automation's fields via the batch-PATCH route.
// The controller replaces the whole actions[] array on write, so a caller
// editing one action must pass the full existing actions[] with its edit
// applied (the array-field merge, like device port_overrides).
func (s AutomationService) Update(ctx context.Context, id string, body any) (*wire.Automation, error) {
	if id == "" {
		return nil, fmt.Errorf("protect: update automation: missing id")
	}
	req := []batchPatchItem{{ID: id, Body: body}}
	resp, err := s.core.session.Do(ctx, http.MethodPatch, s.core.apiPath("automations"), req)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var results []batchPatchResult
	if err := json.Unmarshal(resp.Body, &results); err != nil {
		return nil, fmt.Errorf("decode protect batch-PATCH automations: %w", err)
	}
	if len(results) != 1 {
		return nil, fmt.Errorf("protect batch-PATCH automations: expected 1 result, got %d", len(results))
	}
	if results[0].Status != http.StatusOK {
		return nil, fmt.Errorf("protect batch-PATCH automation %s: inner status %d", id, results[0].Status)
	}
	return &results[0].Response, nil
}
