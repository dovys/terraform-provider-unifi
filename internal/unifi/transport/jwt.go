package transport

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// jwtClaims is the subset of the TOKEN cookie's JWT payload the classic session
// reads: the CSRF token (some firmwares surface it only here) and the expiry
// (so re-auth is driven by the token's own exp, not a hard-coded ~2h).
type jwtClaims struct {
	CSRFToken string `json:"csrfToken"`
	Exp       int64  `json:"exp"`
}

// decodeJWTClaims reads — without verifying — the payload segment of a JWT. The
// classic session never validates the signature: it's the controller's own
// token, and we only read the claims it issued to us.
func decodeJWTClaims(token string) (jwtClaims, error) {
	var claims jwtClaims
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return claims, errors.New("not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, err
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, err
	}
	return claims, nil
}

func (c jwtClaims) expiry() time.Time {
	if c.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(c.Exp, 0)
}
