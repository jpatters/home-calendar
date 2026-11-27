package chatgpt

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"
)

type discoveryDoc struct {
	Issuer             string `json:"issuer"`
	JWKSURI            string `json:"jwks_uri"`
	RevocationEndpoint string `json:"revocation_endpoint"`
}

func (c *Client) getJSON(ctx context.Context, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.o.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", u, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func (c *Client) discovery(ctx context.Context) (*discoveryDoc, error) {
	var d discoveryDoc
	if err := c.getJSON(ctx, c.o.Issuer+"/.well-known/openid-configuration", &d); err != nil {
		return nil, fmt.Errorf("openid configuration: %w", err)
	}
	return &d, nil
}

type idClaims struct {
	Issuer   string          `json:"iss"`
	Audience json.RawMessage `json:"aud"`
	Subject  string          `json:"sub"`
	Email    string          `json:"email"`
	Nonce    string          `json:"nonce"`
	Expiry   int64           `json:"exp"`
}

func (cl idClaims) hasAudience(aud string) bool {
	var one string
	if json.Unmarshal(cl.Audience, &one) == nil {
		return one == aud
	}
	var many []string
	if json.Unmarshal(cl.Audience, &many) == nil {
		for _, a := range many {
			if a == aud {
				return true
			}
		}
	}
	return false
}

var errInvalidIDToken = errors.New("the ID token from ChatGPT is not valid")

// validateIDToken checks an RS256 ID token's signature against the issuer's
// published keys, and its issuer, audience, expiry and (when nonce is not
// empty) nonce.
func (c *Client) validateIDToken(ctx context.Context, token, clientID, nonce string) (*idClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[2] == "" {
		return nil, errInvalidIDToken
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := decodeSegment(parts[0], &header); err != nil || header.Alg != "RS256" {
		return nil, errInvalidIDToken
	}
	disc, err := c.discovery(ctx)
	if err != nil {
		return nil, err
	}
	key, err := c.signingKey(ctx, disc.JWKSURI, header.Kid)
	if err != nil {
		return nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errInvalidIDToken
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig) != nil {
		return nil, errInvalidIDToken
	}
	var claims idClaims
	if err := decodeSegment(parts[1], &claims); err != nil {
		return nil, errInvalidIDToken
	}
	switch {
	case claims.Issuer != c.o.Issuer,
		!claims.hasAudience(clientID),
		claims.Subject == "",
		!c.o.Now().Before(time.Unix(claims.Expiry, 0)),
		nonce != "" && claims.Nonce != nonce:
		return nil, errInvalidIDToken
	}
	return &claims, nil
}

func (c *Client) signingKey(ctx context.Context, jwksURI, kid string) (*rsa.PublicKey, error) {
	if jwksURI == "" {
		return nil, errors.New("no signing keys published")
	}
	var set struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := c.getJSON(ctx, jwksURI, &set); err != nil {
		return nil, fmt.Errorf("signing keys: %w", err)
	}
	for _, k := range set.Keys {
		if k.Kid != kid || k.Kty != "RSA" {
			continue
		}
		n, errN := base64.RawURLEncoding.DecodeString(k.N)
		e, errE := base64.RawURLEncoding.DecodeString(k.E)
		if errN != nil || errE != nil || len(e) == 0 || len(e) > 4 {
			return nil, errInvalidIDToken
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}, nil
	}
	return nil, errInvalidIDToken
}

func decodeSegment(seg string, v any) error {
	raw, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}
