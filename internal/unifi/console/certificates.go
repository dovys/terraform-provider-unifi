package console

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"
)

// CertificateService is the userCertificates group on the console OS surface
// (/api/userCertificates...). It carries the BYO-upload CRUD plus the singleton
// ACME challenge chain (start -> complete) the device uses to issue and then
// natively renew a Let's Encrypt certificate.
type CertificateService struct {
	core *core
}

const (
	certsRoot = "userCertificates"

	// The console UI retries challenge/complete while the TXT propagates; mirror
	// it (bounded, context-aware).
	defaultCompleteAttempts = 10
	defaultCompleteDelay    = 10 * time.Second
)

func (s CertificateService) idPath(id, action string) string {
	p := certsRoot + "/" + url.PathEscape(id) // defense-in-depth: ids may come from import input
	if action != "" {
		p += "/" + action
	}
	return s.core.apiPath(p)
}

func (s CertificateService) challengePath(action string) string {
	return s.core.apiPath(certsRoot + "/challenge/" + action)
}

// List returns every installed user certificate (empty while the console is on
// its self-signed default).
func (s CertificateService) List(ctx context.Context) ([]Certificate, error) {
	var out []Certificate
	if err := do(ctx, s.core, http.MethodGet, s.core.apiPath(certsRoot), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Get returns the certificate with the given id, or ErrNotFound.
func (s CertificateService) Get(ctx context.Context, id string) (*Certificate, error) {
	var out Certificate
	if err := do(ctx, s.core, http.MethodGet, s.idPath(id, ""), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Upload installs a BYO certificate (JSON {name, cert, key}) and returns the
// stored certificate (including the PEM material on this response only).
func (s CertificateService) Upload(ctx context.Context, req UploadRequest) (*Certificate, error) {
	var out Certificate
	if err := do(ctx, s.core, http.MethodPost, s.core.apiPath(certsRoot), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Renew triggers a native renewal of the certificate. The device owns renewal
// and rejects an early call with ACME_CERTIFICATE_RENEW_TOO_EARLY.
func (s CertificateService) Renew(ctx context.Context, id string) (*Certificate, error) {
	var out Certificate
	if err := do(ctx, s.core, http.MethodPost, s.idPath(id, "renew"), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Revoke asks the CA to revoke an ACME certificate (POST :id/revoke). Distinct
// from Delete: revocation is a permanent CA action and is rate-limited. An
// already-absent certificate is not an error.
func (s CertificateService) Revoke(ctx context.Context, id string) error {
	err := do[any](ctx, s.core, http.MethodPost, s.idPath(id, "revoke"), nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// Delete removes the certificate (DELETE :id). When the active certificate is
// removed the console reverts to its self-signed default. An already-absent
// certificate is not an error. This is the local removal the resource uses on
// destroy — it does not revoke the certificate at the CA (see Revoke).
func (s CertificateService) Delete(ctx context.Context, id string) error {
	err := do[any](ctx, s.core, http.MethodDelete, s.idPath(id, ""), nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// SetActive sets whether this certificate is the active (serving) console
// certificate — PUT :id/status. A freshly issued or uploaded certificate is
// inactive until activated; the console serves the active certificate, and
// activating one supersedes the previously active. The renewal the device
// performs under the same id preserves the active flag.
func (s CertificateService) SetActive(ctx context.Context, id string, active bool) error {
	return do[any](ctx, s.core, http.MethodPut, s.idPath(id, "status"), statusRequest{Active: active}, nil)
}

// StartChallenge begins the ACME DNS-01 chain (singleton). With an automated DNS
// provider the device places the _acme-challenge TXT itself; in manual mode the
// returned ChallengeStartResult names the TXT the operator must create. Only one
// challenge runs at a time — a second start fails ACME_CHALLENGE_IN_PROGRESS.
func (s CertificateService) StartChallenge(ctx context.Context, req ChallengeStartRequest) (*ChallengeStartResult, error) {
	var out ChallengeStartResult
	if err := do(ctx, s.core, http.MethodPost, s.challengePath("start"), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CurrentChallenge returns the in-flight challenge, or (nil, nil) when none is
// running (the API returns an empty object).
func (s CertificateService) CurrentChallenge(ctx context.Context) (*ChallengeCurrent, error) {
	var out ChallengeCurrent
	if err := do(ctx, s.core, http.MethodGet, s.challengePath("current"), nil, &out); err != nil {
		return nil, err
	}
	if out.Name == "" && out.Domain == "" && out.Error == nil {
		return nil, nil // no challenge in progress
	}
	return &out, nil
}

// CancelChallenge aborts the in-flight challenge. An absent challenge is not an
// error.
func (s CertificateService) CancelChallenge(ctx context.Context) error {
	err := do[any](ctx, s.core, http.MethodPost, s.challengePath("cancel"), nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// CompleteChallenge validates the pending challenge and issues the certificate.
// While the TXT propagates the device reports DNS_RECORD_NOT_PROPAGATED; mirror
// the console UI and retry (bounded, context-aware) until it clears. The success
// body is the issued certificate (with PEM material on this response only).
func (s CertificateService) CompleteChallenge(ctx context.Context) (*Certificate, error) {
	return s.completeChallenge(ctx, defaultCompleteAttempts, defaultCompleteDelay)
}

func (s CertificateService) completeChallenge(ctx context.Context, attempts int, delay time.Duration) (*Certificate, error) {
	var last error
	for attempt := 0; attempt < attempts; attempt++ {
		var out Certificate
		err := do(ctx, s.core, http.MethodPost, s.challengePath("complete"), nil, &out)
		if err == nil {
			return &out, nil
		}
		var ce *ConsoleError
		if errors.As(err, &ce) && ce.Code == ErrCodeDNSRecordNotPropagated {
			last = err
			if attempt == attempts-1 {
				break
			}
			if serr := sleep(ctx, delay); serr != nil {
				return nil, serr
			}
			continue
		}
		return nil, err
	}
	return nil, last
}
