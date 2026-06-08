package network

import (
	"context"
	"errors"
	"testing"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

func TestFeaturesList(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`["IPS","BGP_ROUTING","STATIC_DNS"]`)}
	feats, err := newTestClient(f).Features(context.Background())
	if err != nil {
		t.Fatalf("Features: %v", err)
	}
	if want := "/proxy/network/v2/api/site/default/features"; f.path != want || f.method != "GET" {
		t.Errorf("got %s %s, want GET %s", f.method, f.path, want)
	}
	if !feats.Has("IPS") || !feats.Has("STATIC_DNS") {
		t.Errorf("Has(IPS)/Has(STATIC_DNS) = false, want present in %v", feats)
	}
	if feats.Has("NOT_A_FEATURE") {
		t.Errorf("Has(NOT_A_FEATURE) = true, want absent")
	}
}

func TestFeaturesNotFound(t *testing.T) {
	f := &fakeDoer{err: &transport.APIError{StatusCode: 404}}
	if _, err := newTestClient(f).Features(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Features err = %v, want ErrNotFound", err)
	}
}
