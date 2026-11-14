package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/roguefairystudios/intranet/studio-server/internal/config"
)

// ErrUnauthenticated means the request carries no valid credentials.
var ErrUnauthenticated = errors.New("auth: unauthenticated")

const (
	HeaderStudioKey      = "X-Studio-Key"
	HeaderStudioClient   = "X-Studio-Client"
	HeaderInternalKey    = "X-Studio-Internal-Key"
	HeaderInternalActor  = "X-Studio-Actor"
	ServiceKeyPrefix     = "stk_"
	bearerAuthorization  = "bearer "
)

var validClientApps = []string{
	"aetherion",
	"geppetto",
	"intranet",
	"lore",
	"goethe",
	"animafaber",
	"fontpit",
	"carnifex",
	"unity",
	"studio",
	"cli",
	"generic",
}

// StoredServiceKey represents a persisted service key record looked up by SHA-256 hash.
type StoredServiceKey struct {
	ID     string
	Name   string
	Role   string // "reader", "editor", "admin"
	Groups []string
}

// ServiceKeyLookup resolves hashed service keys from storage.
type ServiceKeyLookup interface {
	GetServiceKeyByHash(ctx context.Context, hash string) (*StoredServiceKey, error)
}

// WikiGroupRef identifies a Wiki.js group by numeric ID and/or Authentik name.
type WikiGroupRef struct {
	ID   int    `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// DelegatedActor carries the verified Wiki.js user context when Wiki.js delegates over STUDIO_INTERNAL_KEY.
type DelegatedActor struct {
	ID          int            `json:"id"`
	APIID       int            `json:"apiId,omitempty"`
	Email       string         `json:"email,omitempty"`
	Username    string         `json:"username,omitempty"`
	Name        string         `json:"name,omitempty"`
	Groups      []WikiGroupRef `json:"groups,omitempty"`
	Permissions []string       `json:"permissions,omitempty"`
}

// Principal represents any authenticated caller (Authentik OIDC user, Wiki.js delegated user, or Service Key).
type Principal struct {
	Subject      string             `json:"sub"`
	WikiUserID   int                `json:"wikiUserId,omitempty"`
	WikiAPIID    int                `json:"wikiApiId,omitempty"`
	Username     string             `json:"username,omitempty"`
	Name         string             `json:"name,omitempty"`
	Email        string             `json:"email,omitempty"`
	Groups       []string           `json:"groups,omitempty"`
	GroupIDs     []int              `json:"groupIds,omitempty"`
	Permissions  []string           `json:"permissions,omitempty"`
	Entitlements []string           `json:"entitlements,omitempty"`
	LoreGrants   []LoreProjectClaim `json:"loreGrants,omitempty"`

	TokenApp    string `json:"tokenApp,omitempty"`
	ClientApp   string `json:"clientApp"`
	ServiceKey  bool   `json:"serviceKey,omitempty"`
	ServiceRole string `json:"serviceRole,omitempty"` // "reader", "editor", "admin"
	Delegated   bool   `json:"delegated,omitempty"`
	IsAdmin     bool   `json:"isAdmin"`
}

// ActorID returns the canonical audit trail actor string matching Wiki.js history conventions.
func (p Principal) ActorID() any {
	if p.WikiAPIID > 0 {
		return fmt.Sprintf("api:%d", p.WikiAPIID)
	}
	if p.ServiceKey {
		return "key:" + p.Name
	}
	if p.WikiUserID > 0 {
		return p.WikiUserID
	}
	if p.Email != "" {
		return p.Email
	}
	return p.Subject
}

// NormalizeClientApp sanitizes an explicit client hint header or query param.
func NormalizeClientApp(raw, fallback string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		if fallback != "" {
			return fallback
		}
		return "generic"
	}
	if slices.Contains(validClientApps, v) {
		return v
	}
	inferred := InferClientApp(v, "", "", nil)
	if slices.Contains(validClientApps, inferred) {
		return inferred
	}
	if fallback != "" {
		return fallback
	}
	return "generic"
}

// Authenticator validates incoming HTTP requests across all supported Studio auth mechanisms.
type Authenticator struct {
	Verifier    TokenVerifier
	InternalKey string
	StaticKeys  []config.StaticServiceKey
	Keys        ServiceKeyLookup
	AdminGroups []string
	Log         *slog.Logger
}

// Authenticate resolves credentials on r into a verified Principal.
func (a *Authenticator) Authenticate(r *http.Request) (Principal, error) {
	explicitClient := strings.TrimSpace(r.URL.Query().Get("client"))
	if explicitClient == "" {
		explicitClient = strings.TrimSpace(r.Header.Get(HeaderStudioClient))
	}

	// 1. Internal Wiki.js delegation (X-Studio-Internal-Key + X-Studio-Actor)
	if intKey := strings.TrimSpace(r.Header.Get(HeaderInternalKey)); intKey != "" {
		return a.authenticateInternal(intKey, r.Header.Get(HeaderInternalActor), explicitClient)
	}

	// 2. Service key in X-Studio-Key
	if sKey := strings.TrimSpace(r.Header.Get(HeaderStudioKey)); sKey != "" {
		return a.authenticateServiceKey(r.Context(), sKey, explicitClient)
	}

	// 3. Authorization: Bearer <stk_... | jwt>
	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(authz) > len(bearerAuthorization) && strings.EqualFold(authz[:len(bearerAuthorization)], bearerAuthorization) {
		token := strings.TrimSpace(authz[len(bearerAuthorization):])
		if strings.HasPrefix(token, ServiceKeyPrefix) {
			return a.authenticateServiceKey(r.Context(), token, explicitClient)
		}
		p, err := a.authenticateBearer(r.Context(), token, explicitClient)
		if err != nil {
			a.logRejection(r, err)
		}
		return p, err
	}

	return Principal{}, ErrUnauthenticated
}

func (a *Authenticator) authenticateInternal(providedKey, rawActor, explicitClient string) (Principal, error) {
	if a.InternalKey == "" || subtle.ConstantTimeCompare([]byte(providedKey), []byte(a.InternalKey)) != 1 {
		return Principal{}, ErrUnauthenticated
	}
	rawActor = strings.TrimSpace(rawActor)
	if rawActor == "" {
		return Principal{
			Subject:     "internal:wiki",
			Name:        "wiki-internal",
			TokenApp:    "intranet",
			ClientApp:   NormalizeClientApp(explicitClient, "intranet"),
			ServiceKey:  true,
			ServiceRole: "admin",
			Delegated:   true,
			IsAdmin:     true,
		}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(rawActor)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(rawActor)
		if err != nil {
			return Principal{}, ErrUnauthenticated
		}
	}
	var actor DelegatedActor
	if err := json.Unmarshal(decoded, &actor); err != nil {
		return Principal{}, ErrUnauthenticated
	}
	if actor.APIID == 0 && (actor.ID <= 0 || actor.ID == 2) {
		return Principal{}, ErrUnauthenticated
	}
	var groupNames []string
	var groupIDs []int
	for _, g := range actor.Groups {
		if g.ID > 0 && !slices.Contains(groupIDs, g.ID) {
			groupIDs = append(groupIDs, g.ID)
		}
		if name := strings.TrimSpace(g.Name); name != "" && !slices.Contains(groupNames, name) {
			groupNames = append(groupNames, name)
		}
	}
	isAdmin := slices.Contains(actor.Permissions, "manage:system") || (actor.APIID == 0 && a.hasAnyAdminGroup(groupNames))
	sub := fmt.Sprintf("wiki:%d", actor.ID)
	if actor.APIID > 0 {
		sub = fmt.Sprintf("api:%d", actor.APIID)
	}
	return Principal{
		Subject:     sub,
		WikiUserID:  actor.ID,
		WikiAPIID:   actor.APIID,
		Username:    strings.TrimSpace(actor.Username),
		Name:        strings.TrimSpace(actor.Name),
		Email:       strings.TrimSpace(actor.Email),
		Groups:      groupNames,
		GroupIDs:    groupIDs,
		Permissions: actor.Permissions,
		TokenApp:    "intranet",
		ClientApp:   NormalizeClientApp(explicitClient, "intranet"),
		Delegated:   true,
		IsAdmin:     isAdmin,
	}, nil
}

func (a *Authenticator) authenticateBearer(ctx context.Context, token, explicitClient string) (Principal, error) {
	if a.Verifier == nil || token == "" {
		return Principal{}, ErrUnauthenticated
	}
	c, err := a.Verifier.Verify(ctx, token)
	if err != nil {
		return Principal{}, fmt.Errorf("%w: %w", ErrUnauthenticated, err)
	}
	return Principal{
		Subject:      c.Subject,
		Username:     c.Username,
		Name:         c.Name,
		Email:        c.Email,
		Groups:       c.Groups,
		Entitlements: c.Entitlements,
		LoreGrants:   c.LoreGrants,
		TokenApp:     c.ClientApp,
		ClientApp:    NormalizeClientApp(explicitClient, c.ClientApp),
		IsAdmin:      a.hasAnyAdminGroup(c.Groups),
	}, nil
}

func (a *Authenticator) authenticateServiceKey(ctx context.Context, plain, explicitClient string) (Principal, error) {
	if plain == "" {
		return Principal{}, ErrUnauthenticated
	}
	for _, sk := range a.StaticKeys {
		if subtle.ConstantTimeCompare([]byte(plain), []byte(sk.Secret)) == 1 {
			return Principal{
				Subject:     "key:" + sk.Name,
				Name:        sk.Name,
				Groups:      sk.Groups,
				TokenApp:    "cli",
				ClientApp:   NormalizeClientApp(explicitClient, "cli"),
				ServiceKey:  true,
				ServiceRole: sk.Role,
				IsAdmin:     sk.Role == "admin",
			}, nil
		}
	}
	if a.Keys == nil {
		return Principal{}, ErrUnauthenticated
	}
	stored, err := a.Keys.GetServiceKeyByHash(ctx, HashServiceKey(plain))
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}
	if stored == nil {
		return Principal{}, ErrUnauthenticated
	}
	return Principal{
		Subject:     "key:" + stored.ID,
		Name:        stored.Name,
		Groups:      stored.Groups,
		TokenApp:    "cli",
		ClientApp:   NormalizeClientApp(explicitClient, "cli"),
		ServiceKey:  true,
		ServiceRole: stored.Role,
		IsAdmin:     stored.Role == "admin",
	}, nil
}

func (a *Authenticator) hasAnyAdminGroup(userGroups []string) bool {
	for _, ug := range userGroups {
		for _, ag := range a.AdminGroups {
			if strings.EqualFold(strings.TrimSpace(ug), strings.TrimSpace(ag)) {
				return true
			}
		}
	}
	return false
}

func (a *Authenticator) logRejection(r *http.Request, err error) {
	log := a.Log
	if log == nil {
		log = slog.Default()
	}
	reason := "unauthenticated"
	switch {
	case errors.Is(err, ErrIdentityProviderUnavailable):
		reason = "idp_unavailable"
	case errors.Is(err, ErrInvalidToken):
		reason = "token_invalid"
	}
	log.Info("bearer token rejected", "method", r.Method, "path", r.URL.Path, "reason", reason)
}

// GenerateServiceKey creates a random "stk_<64 hex>" service key and its SHA-256 hex digest.
func GenerateServiceKey() (plain, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plain = ServiceKeyPrefix + hex.EncodeToString(b)
	return plain, HashServiceKey(plain), nil
}

// HashServiceKey returns the SHA-256 hex digest of plain.
func HashServiceKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

type principalCtxKey struct{}

// WithPrincipal attaches p to ctx.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey{}, p)
}

// PrincipalFrom retrieves Principal from ctx.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalCtxKey{}).(Principal)
	return p, ok
}
