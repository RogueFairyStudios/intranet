// Package auth verifies Tellurian Authentik multi-application OIDC tokens,
// service keys, internal Wiki.js delegation headers, and Authentik Directory queries.
package auth

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

var (
	// ErrInvalidToken indicates a missing, malformed, expired, untrusted, or unverifiable JWT.
	ErrInvalidToken = errors.New("invalid or expired token")
	// ErrIdentityProviderUnavailable indicates OIDC discovery or JWKS retrieval failed.
	ErrIdentityProviderUnavailable = errors.New("identity provider unavailable")
)

const (
	clockSkewLeeway  = 60 * time.Second
	jwksCacheTTL     = 15 * time.Minute
	minRefreshPeriod = 30 * time.Second
)

// LoreProjectClaim mirrors per-project grants emitted by Tellurian Authentik's Rogue Fairy Lore Scope.
type LoreProjectClaim struct {
	Project       string   `json:"lore_project"`
	Visibility    string   `json:"lore_visibility"`
	Actions       []string `json:"lore_actions"`
	CanCreateRepo bool     `json:"lore_can_create_repo"`
}

// Claims holds verified OIDC claims plus the inferred Studio client tool.
type Claims struct {
	Issuer       string
	Subject      string
	Username     string
	Name         string
	Email        string
	Groups       []string
	Entitlements []string
	ClientApp    string
	LoreGrants   []LoreProjectClaim
}

// TokenVerifier verifies a compact JWS bearer token.
type TokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (Claims, error)
}

// MultiAppVerifierOptions configures multi-application OIDC verification for Tellurian Authentik.
type MultiAppVerifierOptions struct {
	BaseURL          string   // e.g. https://auth.telluriancorp.info
	AllowedSlugs     []string // e.g. aetherion, geppetto, rfs-intranet, rfs-lore-cli
	ExplicitIssuers  []string // Additional explicit issuer URLs (e.g. local test servers)
	AllowedAudiences []string // Optional audience allowlist; empty accepts app-specific audiences
	HTTPClient       *http.Client
}

// MultiAppVerifier validates JWTs across multiple Tellurian Authentik OIDC providers,
// caching signing keys per issuer without holding locks across network I/O.
type MultiAppVerifier struct {
	baseURL          string
	allowedSlugs     map[string]struct{}
	explicitIssuers  map[string]struct{}
	allowedAudiences []string
	httpClient       *http.Client

	mu      sync.RWMutex
	issuers map[string]*issuerKeyCache
}

type issuerKeyCache struct {
	issuer     string
	httpClient *http.Client

	mu        sync.RWMutex
	keys      map[string]crypto.PublicKey
	fetchedAt time.Time

	refreshMu   sync.Mutex
	jwksURI     string
	lastAttempt time.Time
}

var _ TokenVerifier = (*MultiAppVerifier)(nil)

// NewMultiAppVerifier creates a multi-application OIDC verifier.
func NewMultiAppVerifier(opts MultiAppVerifierOptions) *MultiAppVerifier {
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	slugs := make(map[string]struct{}, len(opts.AllowedSlugs))
	for _, s := range opts.AllowedSlugs {
		clean := strings.ToLower(strings.Trim(strings.TrimSpace(s), "/"))
		if clean != "" {
			slugs[clean] = struct{}{}
		}
	}
	explicit := make(map[string]struct{}, len(opts.ExplicitIssuers))
	for _, iss := range opts.ExplicitIssuers {
		clean := normalizeIssuer(iss)
		if clean != "" {
			explicit[clean] = struct{}{}
		}
	}
	return &MultiAppVerifier{
		baseURL:          normalizeIssuer(opts.BaseURL),
		allowedSlugs:     slugs,
		explicitIssuers:  explicit,
		allowedAudiences: opts.AllowedAudiences,
		httpClient:       hc,
		issuers:          make(map[string]*issuerKeyCache),
	}
}

func normalizeIssuer(iss string) string {
	return strings.TrimRight(strings.TrimSpace(iss), "/")
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

type rawRFSLoreClaim struct {
	Projects []LoreProjectClaim `json:"projects"`
}

type jwtClaims struct {
	Issuer            string          `json:"iss"`
	Subject           string          `json:"sub"`
	Audience          json.RawMessage `json:"aud"`
	AuthorizedParty   string          `json:"azp"`
	ClientID          string          `json:"client_id"`
	ExpiresAt         *int64          `json:"exp"`
	NotBefore         *int64          `json:"nbf"`
	IssuedAt          *int64          `json:"iat"`
	PreferredUsername string          `json:"preferred_username"`
	Name              string          `json:"name"`
	Email             string          `json:"email"`
	Groups            []string        `json:"groups"`
	Entitlements      []string        `json:"entitlements"`
	RFSLore           json.RawMessage `json:"rfs_lore"`
}

// Verify validates a JWT against the trusted Tellurian Authentik issuer allowlist and JWKS.
func (m *MultiAppVerifier) Verify(ctx context.Context, rawToken string) (Claims, error) {
	parts := strings.Split(strings.TrimSpace(rawToken), ".")
	if len(parts) != 3 {
		return Claims{}, fmt.Errorf("%w: malformed jwt", ErrInvalidToken)
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, fmt.Errorf("%w: decode header", ErrInvalidToken)
	}
	var hdr jwtHeader
	if err := json.Unmarshal(headerBytes, &hdr); err != nil {
		return Claims{}, fmt.Errorf("%w: parse header", ErrInvalidToken)
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, fmt.Errorf("%w: decode payload", ErrInvalidToken)
	}
	var claims jwtClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return Claims{}, fmt.Errorf("%w: parse claims", ErrInvalidToken)
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, fmt.Errorf("%w: decode signature", ErrInvalidToken)
	}

	normIss := normalizeIssuer(claims.Issuer)
	slug, trusted := m.matchTrustedIssuer(normIss)
	if !trusted {
		return Claims{}, fmt.Errorf("%w: untrusted issuer", ErrInvalidToken)
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return Claims{}, fmt.Errorf("%w: missing sub claim", ErrInvalidToken)
	}

	now := time.Now()
	if claims.ExpiresAt == nil {
		return Claims{}, fmt.Errorf("%w: missing exp claim", ErrInvalidToken)
	}
	if now.After(time.Unix(*claims.ExpiresAt, 0).Add(clockSkewLeeway)) {
		return Claims{}, fmt.Errorf("%w: token expired", ErrInvalidToken)
	}
	if claims.NotBefore != nil && now.Before(time.Unix(*claims.NotBefore, 0).Add(-clockSkewLeeway)) {
		return Claims{}, fmt.Errorf("%w: token not valid yet", ErrInvalidToken)
	}

	if len(m.allowedAudiences) > 0 && !matchesAnyAudience(claims.Audience, claims.AuthorizedParty, m.allowedAudiences) {
		return Claims{}, fmt.Errorf("%w: audience mismatch", ErrInvalidToken)
	}

	cache := m.cacheForIssuer(normIss)
	pubKey, err := cache.getKey(ctx, hdr.Kid)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}

	signingInput := parts[0] + "." + parts[1]
	if err := verifySignature(hdr.Alg, pubKey, []byte(signingInput), sigBytes); err != nil {
		return Claims{}, fmt.Errorf("%w: signature verification failed", ErrInvalidToken)
	}

	clientApp := InferClientApp(slug, claims.AuthorizedParty, claims.ClientID, parseAudiences(claims.Audience))

	return Claims{
		Issuer:       normIss,
		Subject:      strings.TrimSpace(claims.Subject),
		Username:     strings.TrimSpace(claims.PreferredUsername),
		Name:         strings.TrimSpace(claims.Name),
		Email:        strings.TrimSpace(claims.Email),
		Groups:       claims.Groups,
		Entitlements: claims.Entitlements,
		ClientApp:    clientApp,
		LoreGrants:   parseLoreGrants(claims.RFSLore),
	}, nil
}

func (m *MultiAppVerifier) matchTrustedIssuer(normIss string) (slug string, ok bool) {
	if normIss == "" {
		return "", false
	}
	if _, found := m.explicitIssuers[normIss]; found {
		return extractAuthentikSlug(normIss), true
	}
	if m.baseURL != "" && strings.HasPrefix(normIss, m.baseURL+"/application/o/") {
		rest := strings.TrimPrefix(normIss, m.baseURL+"/application/o/")
		rest = strings.Trim(rest, "/")
		if rest != "" && !strings.Contains(rest, "/") {
			lower := strings.ToLower(rest)
			if len(m.allowedSlugs) == 0 {
				return lower, true
			}
			if _, allowed := m.allowedSlugs[lower]; allowed {
				return lower, true
			}
		}
	}
	return "", false
}

func extractAuthentikSlug(normIss string) string {
	idx := strings.Index(normIss, "/application/o/")
	if idx < 0 {
		return ""
	}
	rest := strings.Trim(normIss[idx+len("/application/o/"):], "/")
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		rest = rest[:slash]
	}
	return strings.ToLower(rest)
}

// InferClientApp normalizes an Authentik OIDC slug, client_id, azp, or audience into a canonical Studio tool identifier.
func InferClientApp(slug, azp, clientID string, audiences []string) string {
	candidates := []string{slug, azp, clientID}
	candidates = append(candidates, audiences...)
	for _, raw := range candidates {
		v := strings.ToLower(strings.TrimSpace(raw))
		if v == "" {
			continue
		}
		switch {
		case strings.Contains(v, "aetherion") || strings.Contains(v, "godot"):
			return "aetherion"
		case strings.Contains(v, "geppetto") || strings.Contains(v, "krita"):
			return "geppetto"
		case strings.Contains(v, "intranet") || strings.Contains(v, "wiki"):
			return "intranet"
		case strings.Contains(v, "lore"):
			return "lore"
		case strings.Contains(v, "goethe"):
			return "goethe"
		case strings.Contains(v, "animafaber"):
			return "animafaber"
		case strings.Contains(v, "fontpit"):
			return "fontpit"
		case strings.Contains(v, "carnifex"):
			return "carnifex"
		case strings.Contains(v, "studio"):
			return "studio"
		}
	}
	if slug != "" {
		return slug
	}
	return "generic"
}

func parseAudiences(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil && single != "" {
		return []string{single}
	}
	var multi []string
	if err := json.Unmarshal(raw, &multi); err == nil {
		return multi
	}
	return nil
}

func matchesAnyAudience(rawAud json.RawMessage, azp string, allowed []string) bool {
	auds := parseAudiences(rawAud)
	for _, exp := range allowed {
		if azp != "" && azp == exp {
			return true
		}
		if slices.Contains(auds, exp) {
			return true
		}
	}
	return false
}

func parseLoreGrants(raw json.RawMessage) []LoreProjectClaim {
	if len(raw) == 0 {
		return nil
	}
	var direct []LoreProjectClaim
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct
	}
	var wrapped rawRFSLoreClaim
	if err := json.Unmarshal(raw, &wrapped); err == nil {
		return wrapped.Projects
	}
	return nil
}

func (m *MultiAppVerifier) cacheForIssuer(normIss string) *issuerKeyCache {
	m.mu.RLock()
	c, ok := m.issuers[normIss]
	m.mu.RUnlock()
	if ok {
		return c
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok = m.issuers[normIss]; ok {
		return c
	}
	c = &issuerKeyCache{
		issuer:     normIss,
		httpClient: m.httpClient,
		keys:       make(map[string]crypto.PublicKey),
	}
	m.issuers[normIss] = c
	return c
}

func (c *issuerKeyCache) cached(kid string) (key crypto.PublicKey, ok, fresh bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	fresh = !c.fetchedAt.IsZero() && time.Since(c.fetchedAt) <= jwksCacheTTL
	if k, found := c.keys[kid]; found {
		return k, true, fresh
	}
	if kid == "" && len(c.keys) == 1 {
		for _, k := range c.keys {
			return k, true, fresh
		}
	}
	return nil, false, fresh
}

func (c *issuerKeyCache) getKey(ctx context.Context, kid string) (crypto.PublicKey, error) {
	key, ok, fresh := c.cached(kid)
	if ok && fresh {
		return key, nil
	}
	if err := c.refreshJWKS(ctx); err != nil {
		if ok {
			return key, nil
		}
		return nil, fmt.Errorf("%w: %w", ErrIdentityProviderUnavailable, err)
	}
	if key, ok, _ := c.cached(kid); ok {
		return key, nil
	}
	return nil, fmt.Errorf("unknown signing key kid=%q", kid)
}

type oidcDiscoveryDoc struct {
	JWKSURI string `json:"jwks_uri"`
}

type jwksDoc struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func (c *issuerKeyCache) refreshJWKS(ctx context.Context) error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	c.mu.RLock()
	hasKeys := !c.fetchedAt.IsZero()
	c.mu.RUnlock()

	if hasKeys && time.Since(c.lastAttempt) < minRefreshPeriod {
		return nil
	}
	c.lastAttempt = time.Now()

	if c.jwksURI == "" {
		// Authentik discovery resides at <issuer>/.well-known/openid-configuration
		discURL := c.issuer + "/.well-known/openid-configuration"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, discURL, nil)
		if err != nil {
			return err
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("discovery status %d", resp.StatusCode)
		}
		if readErr != nil {
			return readErr
		}
		var doc oidcDiscoveryDoc
		if err := json.Unmarshal(body, &doc); err != nil {
			return err
		}
		if strings.TrimSpace(doc.JWKSURI) == "" {
			return errors.New("discovery missing jwks_uri")
		}
		c.jwksURI = strings.TrimSpace(doc.JWKSURI)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.jwksURI, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks status %d", resp.StatusCode)
	}
	if readErr != nil {
		return readErr
	}
	var jwks jwksDoc
	if err := json.Unmarshal(body, &jwks); err != nil {
		return err
	}
	parsed := make(map[string]crypto.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		pub, err := parseJWK(k)
		if err != nil {
			continue
		}
		parsed[k.Kid] = pub
	}
	if len(parsed) == 0 {
		return errors.New("jwks contained no usable signing keys")
	}

	c.mu.Lock()
	c.keys = parsed
	c.fetchedAt = time.Now()
	c.mu.Unlock()
	return nil
}

func parseJWK(k jwkKey) (crypto.PublicKey, error) {
	switch k.Kty {
	case "RSA":
		nb, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, err
		}
		eb, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, err
		}
		n := new(big.Int).SetBytes(nb)
		e := int(new(big.Int).SetBytes(eb).Int64())
		if e <= 1 {
			return nil, errors.New("invalid rsa exponent")
		}
		return &rsa.PublicKey{N: n, E: e}, nil
	case "EC":
		var curve elliptic.Curve
		switch k.Crv {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		case "P-521":
			curve = elliptic.P521()
		default:
			return nil, fmt.Errorf("unsupported ec curve %q", k.Crv)
		}
		xb, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil {
			return nil, err
		}
		yb, err := base64.RawURLEncoding.DecodeString(k.Y)
		if err != nil {
			return nil, err
		}
		return &ecdsa.PublicKey{
			Curve: curve,
			X:     new(big.Int).SetBytes(xb),
			Y:     new(big.Int).SetBytes(yb),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported key type %q", k.Kty)
	}
}

func verifySignature(alg string, pub crypto.PublicKey, signingInput, sig []byte) error {
	var (
		hashType crypto.Hash
		hashFn   func() hash.Hash
	)
	switch alg {
	case "RS256", "ES256":
		hashType, hashFn = crypto.SHA256, sha256.New
	case "RS384", "ES384":
		hashType, hashFn = crypto.SHA384, sha512.New384
	case "RS512", "ES512":
		hashType, hashFn = crypto.SHA512, sha512.New
	default:
		return fmt.Errorf("unsupported algorithm %q", alg)
	}
	h := hashFn()
	_, _ = h.Write(signingInput)
	digest := h.Sum(nil)

	switch {
	case strings.HasPrefix(alg, "RS"):
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			return errors.New("key is not RSA")
		}
		return rsa.VerifyPKCS1v15(rsaPub, hashType, digest, sig)
	case strings.HasPrefix(alg, "ES"):
		ecPub, ok := pub.(*ecdsa.PublicKey)
		if !ok {
			return errors.New("key is not ECDSA")
		}
		keySize := (ecPub.Curve.Params().BitSize + 7) / 8
		if len(sig) == 2*keySize {
			r := new(big.Int).SetBytes(sig[:keySize])
			s := new(big.Int).SetBytes(sig[keySize:])
			if !ecdsa.Verify(ecPub, digest, r, s) {
				return errors.New("ecdsa signature invalid")
			}
			return nil
		}
		if !ecdsa.VerifyASN1(ecPub, digest, sig) {
			return errors.New("ecdsa asn1 signature invalid")
		}
		return nil
	default:
		return fmt.Errorf("unsupported algorithm %q", alg)
	}
}
