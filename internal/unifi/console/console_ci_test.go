package console

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// recordedCall captures one request the service made.
type recordedCall struct {
	method string
	path   string
	body   any
}

// fakeDoer is an injectable session seam. fn returns the canned (resp, err) for
// each call; calls records the requests for assertions.
type fakeDoer struct {
	fn    func(call recordedCall) (*transport.Response, error)
	calls []recordedCall
}

func (f *fakeDoer) Do(_ context.Context, method, path string, body any) (*transport.Response, error) {
	c := recordedCall{method: method, path: path, body: body}
	f.calls = append(f.calls, c)
	return f.fn(c)
}

func newService(fn func(call recordedCall) (*transport.Response, error)) (CertificateService, *fakeDoer) {
	d := &fakeDoer{fn: fn}
	return CertificateService{core: &core{session: d}}, d
}

func okJSON(v any) *transport.Response {
	b, _ := json.Marshal(v)
	return &transport.Response{StatusCode: 200, Body: b}
}

func apiErr(status int, body string) error {
	return &transport.APIError{Surface: "classic", StatusCode: status, Body: []byte(body)}
}

func TestStartChallenge_CloudflareBody(t *testing.T) {
	svc, d := newService(func(recordedCall) (*transport.Response, error) {
		return okJSON(ChallengeStartResult{Name: "_acme-challenge.console.example.com", Content: "abc"}), nil
	})
	res, err := svc.StartChallenge(context.Background(), ChallengeStartRequest{
		Name:        "console",
		Domain:      "console.example.com",
		DNSProvider: &DNSProvider{Provider: ProviderCloudflare, APIToken: "cf-secret"},
	})
	if err != nil {
		t.Fatalf("StartChallenge: %v", err)
	}
	if res.Name == "" || res.Content != "abc" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if got, want := d.calls[0].path, "/api/userCertificates/challenge/start"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	// The serialized body must carry only cloudflare's apiToken — no other
	// provider's credential fields leak in (omitempty discipline).
	b, _ := json.Marshal(d.calls[0].body)
	got := string(b)
	want := `{"name":"console","domain":"console.example.com","dnsProvider":{"provider":"cloudflare","apiToken":"cf-secret"}}`
	if got != want {
		t.Fatalf("start body = %s\nwant %s", got, want)
	}
}

func TestStartChallenge_Route53Body(t *testing.T) {
	svc, d := newService(func(recordedCall) (*transport.Response, error) {
		return okJSON(ChallengeStartResult{}), nil
	})
	_, err := svc.StartChallenge(context.Background(), ChallengeStartRequest{
		Name:   "console",
		Domain: "console.example.com",
		DNSProvider: &DNSProvider{
			Provider:        ProviderRoute53,
			AccessKeyID:     "AKIA",
			SecretAccessKey: "secret",
			Region:          "us-east-1",
		},
	})
	if err != nil {
		t.Fatalf("StartChallenge: %v", err)
	}
	b, _ := json.Marshal(d.calls[0].body)
	want := `{"name":"console","domain":"console.example.com","dnsProvider":{"provider":"route53","accessKeyId":"AKIA","secretAccessKey":"secret","region":"us-east-1"}}`
	if string(b) != want {
		t.Fatalf("start body = %s\nwant %s", b, want)
	}
}

func TestStartChallenge_ManualOmitsProvider(t *testing.T) {
	svc, d := newService(func(recordedCall) (*transport.Response, error) {
		return okJSON(ChallengeStartResult{Name: "_acme-challenge.x", Content: "txt"}), nil
	})
	_, err := svc.StartChallenge(context.Background(), ChallengeStartRequest{Name: "console", Domain: "console.example.com"})
	if err != nil {
		t.Fatalf("StartChallenge: %v", err)
	}
	b, _ := json.Marshal(d.calls[0].body)
	want := `{"name":"console","domain":"console.example.com"}`
	if string(b) != want {
		t.Fatalf("manual start body = %s\nwant %s", b, want)
	}
}

func TestStartChallenge_InProgressTypedError(t *testing.T) {
	svc, _ := newService(func(recordedCall) (*transport.Response, error) {
		// The OS surface may report the error envelope on a non-2xx response.
		return nil, apiErr(http.StatusConflict, `{"code":"ACME_CHALLENGE_IN_PROGRESS","level":"error","message":"already running"}`)
	})
	_, err := svc.StartChallenge(context.Background(), ChallengeStartRequest{Name: "c", Domain: "d"})
	var ce *ConsoleError
	if !errors.As(err, &ce) {
		t.Fatalf("want *ConsoleError, got %T: %v", err, err)
	}
	if ce.Code != ErrCodeACMEChallengeInProgress {
		t.Fatalf("code = %q", ce.Code)
	}
}

func TestErrorEnvelopeOn2xxBody(t *testing.T) {
	// Some challenge endpoints return the {code,...} envelope with HTTP 200.
	svc, _ := newService(func(recordedCall) (*transport.Response, error) {
		return &transport.Response{StatusCode: 200, Body: []byte(`{"code":"CALL_DNS_PROVIDER_API_FAILED","message":"bad token"}`)}, nil
	})
	_, err := svc.StartChallenge(context.Background(), ChallengeStartRequest{Name: "c", Domain: "d"})
	var ce *ConsoleError
	if !errors.As(err, &ce) || ce.Code != ErrCodeCallDNSProviderAPIFailed {
		t.Fatalf("want CALL_DNS_PROVIDER_API_FAILED ConsoleError, got %v", err)
	}
}

func TestGet_NotFoundMapping(t *testing.T) {
	// NOT_FOUND code -> ErrNotFound (so Read removes from state).
	svc, _ := newService(func(recordedCall) (*transport.Response, error) {
		return nil, apiErr(http.StatusBadRequest, `{"code":"NOT_FOUND","message":"no such cert"}`)
	})
	if _, err := svc.Get(context.Background(), "abc"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	// A bare HTTP 404 (no envelope) also maps to ErrNotFound.
	svc2, _ := newService(func(recordedCall) (*transport.Response, error) {
		return nil, apiErr(http.StatusNotFound, "")
	})
	if _, err := svc2.Get(context.Background(), "abc"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("404 want ErrNotFound, got %v", err)
	}
}

func TestGet_Path(t *testing.T) {
	svc, d := newService(func(recordedCall) (*transport.Response, error) {
		return okJSON(Certificate{ID: "id1", Name: "console", Active: true, ValidTo: "2026-09-01T00:00:00Z"}), nil
	})
	c, err := svc.Get(context.Background(), "id1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if c.ID != "id1" || !c.Active {
		t.Fatalf("unexpected cert: %+v", c)
	}
	if d.calls[0].method != http.MethodGet || d.calls[0].path != "/api/userCertificates/id1" {
		t.Fatalf("call = %+v", d.calls[0])
	}
}

func TestUpload_Body(t *testing.T) {
	svc, d := newService(func(recordedCall) (*transport.Response, error) {
		return okJSON(Certificate{ID: "id2", Name: "byo", Source: "uploaded"}), nil
	})
	c, err := svc.Upload(context.Background(), UploadRequest{Name: "byo", Cert: "CERTPEM", Key: "KEYPEM"})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if c.ID != "id2" || c.Source != "uploaded" {
		t.Fatalf("unexpected cert: %+v", c)
	}
	if d.calls[0].method != http.MethodPost || d.calls[0].path != "/api/userCertificates" {
		t.Fatalf("call = %+v", d.calls[0])
	}
	b, _ := json.Marshal(d.calls[0].body)
	if string(b) != `{"name":"byo","cert":"CERTPEM","key":"KEYPEM"}` {
		t.Fatalf("upload body = %s", b)
	}
}

func TestRevoke_Path_And_NotFoundSwallowed(t *testing.T) {
	svc, d := newService(func(recordedCall) (*transport.Response, error) {
		return &transport.Response{StatusCode: 200}, nil
	})
	if err := svc.Revoke(context.Background(), "id3"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if d.calls[0].path != "/api/userCertificates/id3/revoke" {
		t.Fatalf("path = %q", d.calls[0].path)
	}
	// An already-absent cert is not an error.
	svc2, _ := newService(func(recordedCall) (*transport.Response, error) {
		return nil, apiErr(http.StatusNotFound, "")
	})
	if err := svc2.Revoke(context.Background(), "gone"); err != nil {
		t.Fatalf("Revoke(absent) = %v, want nil", err)
	}
}

func TestSetActive_Body(t *testing.T) {
	svc, d := newService(func(recordedCall) (*transport.Response, error) {
		return &transport.Response{StatusCode: 200}, nil
	})
	if err := svc.SetActive(context.Background(), "id9", true); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if d.calls[0].method != http.MethodPut || d.calls[0].path != "/api/userCertificates/id9/status" {
		t.Fatalf("call = %+v", d.calls[0])
	}
	b, _ := json.Marshal(d.calls[0].body)
	if string(b) != `{"active":true}` {
		t.Fatalf("status body = %s, want {\"active\":true}", b)
	}
}

func TestDelete_Path_And_NotFoundSwallowed(t *testing.T) {
	svc, d := newService(func(recordedCall) (*transport.Response, error) {
		return &transport.Response{StatusCode: 200}, nil
	})
	if err := svc.Delete(context.Background(), "id4"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if d.calls[0].method != http.MethodDelete || d.calls[0].path != "/api/userCertificates/id4" {
		t.Fatalf("call = %+v", d.calls[0])
	}
	svc2, _ := newService(func(recordedCall) (*transport.Response, error) {
		return nil, apiErr(http.StatusNotFound, "")
	})
	if err := svc2.Delete(context.Background(), "gone"); err != nil {
		t.Fatalf("Delete(absent) = %v, want nil", err)
	}
}

func TestCompleteChallenge_RetriesThenSucceeds(t *testing.T) {
	var n int
	svc, _ := newService(func(recordedCall) (*transport.Response, error) {
		n++
		if n < 3 {
			return nil, apiErr(http.StatusBadRequest, `{"code":"DNS_RECORD_NOT_PROPAGATED","message":"not yet"}`)
		}
		return okJSON(Certificate{ID: "issued", Name: "console", Active: true}), nil
	})
	cert, err := svc.completeChallenge(context.Background(), 5, time.Millisecond)
	if err != nil {
		t.Fatalf("completeChallenge: %v", err)
	}
	if cert.ID != "issued" {
		t.Fatalf("cert = %+v", cert)
	}
	if n != 3 {
		t.Fatalf("attempts = %d, want 3", n)
	}
}

func TestCompleteChallenge_GivesUpAfterAttempts(t *testing.T) {
	var n int
	svc, _ := newService(func(recordedCall) (*transport.Response, error) {
		n++
		return nil, apiErr(http.StatusBadRequest, `{"code":"DNS_RECORD_NOT_PROPAGATED"}`)
	})
	_, err := svc.completeChallenge(context.Background(), 3, time.Millisecond)
	var ce *ConsoleError
	if !errors.As(err, &ce) || ce.Code != ErrCodeDNSRecordNotPropagated {
		t.Fatalf("want DNS_RECORD_NOT_PROPAGATED, got %v", err)
	}
	if n != 3 {
		t.Fatalf("attempts = %d, want 3", n)
	}
}

func TestCompleteChallenge_OtherErrorReturnsImmediately(t *testing.T) {
	var n int
	svc, _ := newService(func(recordedCall) (*transport.Response, error) {
		n++
		return nil, apiErr(http.StatusBadRequest, `{"code":"ACME_CHALLENGE_VALIDATION_FAILED","message":"bad"}`)
	})
	_, err := svc.completeChallenge(context.Background(), 5, time.Millisecond)
	var ce *ConsoleError
	if !errors.As(err, &ce) || ce.Code != ErrCodeACMEChallengeValidationFailed {
		t.Fatalf("want validation-failed, got %v", err)
	}
	if n != 1 {
		t.Fatalf("attempts = %d, want 1 (no retry on a non-propagation error)", n)
	}
}

func TestCompleteChallenge_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	svc, _ := newService(func(recordedCall) (*transport.Response, error) {
		cancel() // cancel after the first failed attempt; the retry sleep must abort
		return nil, apiErr(http.StatusBadRequest, `{"code":"DNS_RECORD_NOT_PROPAGATED"}`)
	})
	_, err := svc.completeChallenge(ctx, 5, time.Hour)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestCurrentChallenge_NoneInProgress(t *testing.T) {
	svc, _ := newService(func(recordedCall) (*transport.Response, error) {
		return &transport.Response{StatusCode: 200, Body: []byte(`{}`)}, nil
	})
	cur, err := svc.CurrentChallenge(context.Background())
	if err != nil {
		t.Fatalf("CurrentChallenge: %v", err)
	}
	if cur != nil {
		t.Fatalf("want nil (no challenge), got %+v", cur)
	}
}

func TestCurrentChallenge_InProgress(t *testing.T) {
	svc, _ := newService(func(recordedCall) (*transport.Response, error) {
		return &transport.Response{StatusCode: 200, Body: []byte(`{"name":"console","domain":"console.example.com"}`)}, nil
	})
	cur, err := svc.CurrentChallenge(context.Background())
	if err != nil {
		t.Fatalf("CurrentChallenge: %v", err)
	}
	if cur == nil || cur.Domain != "console.example.com" {
		t.Fatalf("unexpected current: %+v", cur)
	}
}

func TestList_Path(t *testing.T) {
	svc, d := newService(func(recordedCall) (*transport.Response, error) {
		return &transport.Response{StatusCode: 200, Body: []byte(`[]`)}, nil
	})
	certs, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(certs) != 0 {
		t.Fatalf("want empty list, got %d", len(certs))
	}
	if d.calls[0].path != "/api/userCertificates" {
		t.Fatalf("path = %q", d.calls[0].path)
	}
}
