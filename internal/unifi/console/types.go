package console

// Hand-written wire types for the unifi-core certificate surface (see the
// package doc in core.go for why these are not generated). Field names and JSON
// tags are taken verbatim from the console UI bundle's zod schemas: response
// objects are snake_case, the DNS-provider request union is camelCase.

// Certificate is a user-installed TLS certificate on the UniFi OS console,
// returned by GET /api/userCertificates[/:id]. List and Get omit the PEM
// material; only the upload/complete response carries Cert and Key. A self-signed
// default console cert is not a user certificate and never appears here (the list
// is empty until a cert is installed).
type Certificate struct {
	ID           string                `json:"id"`
	Name         string                `json:"name"`
	Active       bool                  `json:"active"`
	Version      int                   `json:"version"`
	SerialNumber string                `json:"serial_number"`
	Fingerprint  string                `json:"fingerprint"`
	Subject      CertificateAttributes `json:"subject"`
	Issuer       CertificateAttributes `json:"issuer"`
	ValidFrom    string                `json:"valid_from"`
	ValidTo      string                `json:"valid_to"`
	CreatedAt    string                `json:"created_at"`
	UpdatedAt    string                `json:"updated_at"`
	// Source is "lets_encrypt" or "uploaded" (absent for legacy/unknown).
	Source string `json:"source,omitempty"`
	// ACMERenewError, when present, reports why the device's last native renewal
	// attempt failed (Code is one of the renew Err* constants).
	ACMERenewError *ACMEError `json:"acme_renew_error,omitempty"`

	// Cert and Key are the PEM material, returned only on the upload/complete
	// response — never on List/Get. Never persisted to Terraform state.
	Cert string `json:"cert,omitempty"`
	Key  string `json:"key,omitempty"`
}

// CertificateAttributes is an X.509 distinguished-name subset (subject/issuer).
type CertificateAttributes struct {
	C  string `json:"C,omitempty"`
	CN string `json:"CN,omitempty"`
	L  string `json:"L,omitempty"`
	O  string `json:"O,omitempty"`
	OU string `json:"OU,omitempty"`
}

// ACMEError is the device-reported native-renewal error on a Certificate.
type ACMEError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// UploadRequest is the body of POST /api/userCertificates — install a BYO cert.
// JSON (not multipart): {name, cert, key} with PEM-encoded material.
type UploadRequest struct {
	Name string `json:"name"`
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

// ChallengeStartRequest is the body of POST /api/userCertificates/challenge/start.
// DNSProvider is omitted for manual DNS-01 (the operator places the TXT from the
// returned ChallengeStartResult themselves).
type ChallengeStartRequest struct {
	Name        string       `json:"name"`
	Domain      string       `json:"domain"`
	DNSProvider *DNSProvider `json:"dnsProvider,omitempty"`
}

// DNSProvider is the discriminated union the console accepts for automated
// DNS-01. Provider selects the API the device calls to place the _acme-challenge
// TXT; the remaining fields are that provider's credentials. Only the fields for
// the selected provider are sent (omitempty). Credential values are write-only
// at the Terraform layer and never read back.
//
//	cloudflare, digitalocean -> APIToken
//	duckdns                  -> Token
//	dynu                     -> APIKey
//	godaddy                  -> APIKey, APISecret
//	porkbun                  -> APIKey, SecretKey
//	route53                  -> AccessKeyID, SecretAccessKey, Region
//	ovh                      -> ApplicationKey, ApplicationSecret, ConsumerKey, Endpoint
//	manual                   -> (omit DNSProvider entirely)
type DNSProvider struct {
	Provider          string `json:"provider"`
	APIToken          string `json:"apiToken,omitempty"`
	Token             string `json:"token,omitempty"`
	APIKey            string `json:"apiKey,omitempty"`
	APISecret         string `json:"apiSecret,omitempty"`
	SecretKey         string `json:"secretKey,omitempty"`
	AccessKeyID       string `json:"accessKeyId,omitempty"`
	SecretAccessKey   string `json:"secretAccessKey,omitempty"`
	Region            string `json:"region,omitempty"`
	ApplicationKey    string `json:"applicationKey,omitempty"`
	ApplicationSecret string `json:"applicationSecret,omitempty"`
	ConsumerKey       string `json:"consumerKey,omitempty"`
	Endpoint          string `json:"endpoint,omitempty"`
}

// ChallengeStartResult is the success body of challenge/start: the
// _acme-challenge TXT record the device wants placed (Name + Content). With an
// automated DNS provider the device places it itself; in manual mode the
// operator must create it before challenge/complete succeeds.
type ChallengeStartResult struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// ChallengeCurrent is the body of GET /api/userCertificates/challenge/current —
// the in-flight challenge (singleton), or the zero value when none is running.
type ChallengeCurrent struct {
	Name        string          `json:"name"`
	Domain      string          `json:"domain"`
	DNSProvider *DNSProvider    `json:"dnsProvider,omitempty"`
	Error       *ChallengeError `json:"error,omitempty"`
}

// ChallengeError is the error block on a current-challenge poll.
type ChallengeError struct {
	HTTPCode int    `json:"httpCode"`
	Code     string `json:"code"`
}

// Provider names for the DNS-01 union (the `provider` discriminator).
const (
	ProviderCloudflare   = "cloudflare"
	ProviderRoute53      = "route53"
	ProviderGoDaddy      = "godaddy"
	ProviderDigitalOcean = "digitalocean"
	ProviderPorkbun      = "porkbun"
	ProviderDuckDNS      = "duckdns"
	ProviderDynu         = "dynu"
	ProviderOVH          = "ovh"
	ProviderManual       = "manual"
)

// Error codes the unifi-core certificate API returns (machine-readable `code`).
const (
	// challenge/start
	ErrCodeUserCertificateDuplicate = "USER_CERTIFICATE_DUPLICATE"
	ErrCodeACMEChallengeInProgress  = "ACME_CHALLENGE_IN_PROGRESS"
	ErrCodeCallDNSProviderAPIFailed = "CALL_DNS_PROVIDER_API_FAILED"
	ErrCodeACMEChallengeIssueFailed = "ACME_CHALLENGE_ISSUE_FAILED"
	// challenge/complete
	ErrCodeActionForbidden               = "ACTION_FORBIDDEN"
	ErrCodeDNSRecordNotPropagated        = "DNS_RECORD_NOT_PROPAGATED"
	ErrCodeACMEChallengeValidationFailed = "ACME_CHALLENGE_VALIDATION_FAILED"
	// renew (also surfaced on Certificate.ACMERenewError.Code)
	ErrCodeACMECertificateRenewTooEarly = "ACME_CERTIFICATE_RENEW_TOO_EARLY"
	ErrCodeACMERenewCertificateFailed   = "ACME_RENEW_CERTIFICATE_FAILED"
	// upload (POST /api/userCertificates)
	ErrCodeUserCertificateKeyMismatch     = "USER_CERTIFICATE_KEY_MISMATCH"
	ErrCodeUserCertificateUnsupportedType = "USER_CERTIFICATE_UNSUPPORTED_TYPE"
	ErrCodeUserCertificateInvalidCN       = "USER_CERTIFICATE_INVALID_CN"
	ErrCodeUserCertificateExpired         = "USER_CERTIFICATE_EXPIRED"
	// shared
	ErrCodeNotFound = "NOT_FOUND"
)

// statusRequest is the body of PUT /api/userCertificates/:id/status — set whether
// the certificate is the active (serving) console certificate.
type statusRequest struct {
	Active bool `json:"active"`
}
