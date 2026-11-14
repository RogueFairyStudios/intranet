package auth_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/roguefairystudios/intranet/studio-server/internal/auth"
	"github.com/roguefairystudios/intranet/studio-server/internal/config"
)

type mockOIDCIdP struct {
	server *httptest.Server
	rsaKey *rsa.PrivateKey
	ecKey  *ecdsa.PrivateKey
}

func newMockOIDCIdP(t *testing.T) *mockOIDCIdP {
	t.Helper()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ec key: %v", err)
	}

	idp := &mockOIDCIdP{rsaKey: rsaKey, ecKey: ecKey}
	mux := http.NewServeMux()
	mux.HandleFunc("/application/o/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/.well-known/openid-configuration"):
			slugPath := strings.TrimSuffix(r.URL.Path, "/.well-known/openid-configuration")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":   idp.server.URL + slugPath + "/",
				"jwks_uri": idp.server.URL + slugPath + "/jwks/",
			})
		case strings.HasSuffix(r.URL.Path, "/jwks/"):
			eBytes := big.NewInt(int64(rsaKey.PublicKey.E)).Bytes()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"keys": []map[string]any{
					{
						"kty": "RSA",
						"use": "sig",
						"kid": "rsa-1",
						"n":   base64.RawURLEncoding.EncodeToString(rsaKey.PublicKey.N.Bytes()),
						"e":   base64.RawURLEncoding.EncodeToString(eBytes),
					},
					{
						"kty": "EC",
						"use": "sig",
						"kid": "ec-1",
						"crv": "P-256",
						"x":   base64.RawURLEncoding.EncodeToString(ecKey.PublicKey.X.Bytes()),
						"y":   base64.RawURLEncoding.EncodeToString(ecKey.PublicKey.Y.Bytes()),
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/api/v3/core/users/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ak-dir-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		search := strings.ToLower(r.URL.Query().Get("search"))
		if search == "bianca" || search == "bianca@roguefairy.net" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{
						"pk":        42,
						"uuid":      "sub-bianca-uuid",
						"username":  "bianca",
						"name":      "Bianca Artista",
						"email":     "bianca@roguefairy.net",
						"is_active": true,
						"groups_obj": []map[string]any{
							{"name": "Rogue Fairy Studios"},
							{"name": "Rogue Fairy Studios - Echoes - Art"},
						},
					},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
	})

	idp.server = httptest.NewServer(mux)
	t.Cleanup(idp.server.Close)
	return idp
}

func (m *mockOIDCIdP) signRS256(t *testing.T, slug string, claims map[string]any) string {
	t.Helper()
	if _, ok := claims["iss"]; !ok {
		claims["iss"] = m.server.URL + "/application/o/" + slug + "/"
	}
	if _, ok := claims["exp"]; !ok {
		claims["exp"] = time.Now().Add(10 * time.Minute).Unix()
	}
	hdrJSON, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "rsa-1", "typ": "JWT"})
	payJSON, _ := json.Marshal(claims)
	input := base64.RawURLEncoding.EncodeToString(hdrJSON) + "." + base64.RawURLEncoding.EncodeToString(payJSON)
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, m.rsaKey, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("sign rsa: %v", err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (m *mockOIDCIdP) signES256(t *testing.T, slug string, claims map[string]any) string {
	t.Helper()
	if _, ok := claims["iss"]; !ok {
		claims["iss"] = m.server.URL + "/application/o/" + slug + "/"
	}
	if _, ok := claims["exp"]; !ok {
		claims["exp"] = time.Now().Add(10 * time.Minute).Unix()
	}
	hdrJSON, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": "ec-1", "typ": "JWT"})
	payJSON, _ := json.Marshal(claims)
	input := base64.RawURLEncoding.EncodeToString(hdrJSON) + "." + base64.RawURLEncoding.EncodeToString(payJSON)
	sum := sha256.Sum256([]byte(input))
	rInt, sInt, err := ecdsa.Sign(rand.Reader, m.ecKey, sum[:])
	if err != nil {
		t.Fatalf("sign ecdsa: %v", err)
	}
	raw := make([]byte, 64)
	rInt.FillBytes(raw[:32])
	sInt.FillBytes(raw[32:])
	return input + "." + base64.RawURLEncoding.EncodeToString(raw)
}

func TestMultiAppAuthenticatorAndDirectory(t *testing.T) {
	idp := newMockOIDCIdP(t)
	verifier := auth.NewMultiAppVerifier(auth.MultiAppVerifierOptions{
		BaseURL:      idp.server.URL,
		AllowedSlugs: []string{"aetherion", "geppetto", "rfs-intranet", "rfs-lore-cli"},
	})
	authn := &auth.Authenticator{
		Verifier:    verifier,
		InternalKey: "internal-shared-secret",
		StaticKeys: []config.StaticServiceKey{
			{Name: "ci-runner", Role: "editor", Secret: "stk_static_ci_secret"},
		},
		AdminGroups: []string{"Rogue Fairy Studios - C-Level", "authentik Admins"},
	}

	t.Run("accepts Aetherion RS256 token and infers clientApp=aetherion", func(t *testing.T) {
		tok := idp.signRS256(t, "aetherion", map[string]any{
			"sub":                "sub-lucas",
			"preferred_username": "lucas",
			"name":               "Lucas Dev",
			"email":              "lucas@roguefairy.net",
			"groups":             []string{"Rogue Fairy Studios"},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/me/projects", nil)
		req.Header.Set("Authorization", "Bearer "+tok)

		p, err := authn.Authenticate(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.ClientApp != "aetherion" || p.TokenApp != "aetherion" {
			t.Fatalf("expected clientApp=aetherion, got client=%q token=%q", p.ClientApp, p.TokenApp)
		}
		if p.IsAdmin {
			t.Fatalf("regular employee must not be admin")
		}
	})

	t.Run("accepts Geppetto ES256 token with rfs_lore claims", func(t *testing.T) {
		tok := idp.signES256(t, "geppetto", map[string]any{
			"sub":                "sub-bianca",
			"preferred_username": "bianca",
			"email":              "bianca@roguefairy.net",
			"groups":             []string{"Rogue Fairy Studios"},
			"rfs_lore": map[string]any{
				"projects": []map[string]any{
					{
						"lore_project":         "echoes-of-nyx",
						"lore_visibility":      "internal",
						"lore_actions":         []string{"read", "write"},
						"lore_can_create_repo": false,
					},
				},
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/me/workspaces", nil)
		req.Header.Set("Authorization", "Bearer "+tok)

		p, err := authn.Authenticate(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.ClientApp != "geppetto" {
			t.Fatalf("expected clientApp=geppetto, got %q", p.ClientApp)
		}
		if len(p.LoreGrants) != 1 || p.LoreGrants[0].Project != "echoes-of-nyx" {
			t.Fatalf("expected parsed rfs_lore grant, got %+v", p.LoreGrants)
		}
	})

	t.Run("rejects untrusted Authentik slug and expired token", func(t *testing.T) {
		untrusted := idp.signRS256(t, "unapproved-app", map[string]any{"sub": "u1"})
		req1 := httptest.NewRequest(http.MethodGet, "/v1/me/projects", nil)
		req1.Header.Set("Authorization", "Bearer "+untrusted)
		if _, err := authn.Authenticate(req1); err == nil {
			t.Fatal("expected untrusted slug to be rejected")
		}

		expired := idp.signRS256(t, "aetherion", map[string]any{
			"sub": "u1",
			"exp": time.Now().Add(-5 * time.Minute).Unix(),
		})
		req2 := httptest.NewRequest(http.MethodGet, "/v1/me/projects", nil)
		req2.Header.Set("Authorization", "Bearer "+expired)
		if _, err := authn.Authenticate(req2); err == nil {
			t.Fatal("expected expired token to be rejected")
		}
	})

	t.Run("validates Wiki.js internal delegation and rejects Guest ID 2", func(t *testing.T) {
		actorJSON, _ := json.Marshal(auth.DelegatedActor{
			ID:          7,
			Email:       "ceo@roguefairy.net",
			Name:        "Founder",
			Groups:      []auth.WikiGroupRef{{ID: 1, Name: "Rogue Fairy Studios - C-Level"}},
			Permissions: []string{"manage:system"},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/projects", nil)
		req.Header.Set(auth.HeaderInternalKey, "internal-shared-secret")
		req.Header.Set(auth.HeaderInternalActor, base64.RawURLEncoding.EncodeToString(actorJSON))

		p, err := authn.Authenticate(req)
		if err != nil {
			t.Fatalf("unexpected delegation error: %v", err)
		}
		if !p.Delegated || !p.IsAdmin || p.WikiUserID != 7 {
			t.Fatalf("unexpected delegated principal: %+v", p)
		}

		guestJSON, _ := json.Marshal(auth.DelegatedActor{ID: 2, Name: "Guest"})
		guestReq := httptest.NewRequest(http.MethodGet, "/v1/projects", nil)
		guestReq.Header.Set(auth.HeaderInternalKey, "internal-shared-secret")
		guestReq.Header.Set(auth.HeaderInternalActor, base64.RawURLEncoding.EncodeToString(guestJSON))
		if _, err := authn.Authenticate(guestReq); err == nil {
			t.Fatal("expected Guest ID 2 to be rejected")
		}
	})

	t.Run("validates static X-Studio-Key and Authentik Directory lookup", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/projects/1/events", nil)
		req.Header.Set(auth.HeaderStudioKey, "stk_static_ci_secret")
		req.Header.Set(auth.HeaderStudioClient, "geppetto")
		p, err := authn.Authenticate(req)
		if err != nil {
			t.Fatalf("unexpected service key error: %v", err)
		}
		if !p.ServiceKey || p.ServiceRole != "editor" || p.ClientApp != "geppetto" {
			t.Fatalf("unexpected service key principal: %+v", p)
		}

		dir := auth.NewAuthentikAPIClient(idp.server.URL, "ak-dir-token", nil)
		u, err := dir.LookupUser(context.Background(), "bianca")
		if err != nil || u == nil {
			t.Fatalf("expected bianca from directory, got user=%+v err=%v", u, err)
		}
		if u.Email != "bianca@roguefairy.net" || len(u.Groups) != 2 {
			t.Fatalf("unexpected directory user: %+v", u)
		}
	})
}
