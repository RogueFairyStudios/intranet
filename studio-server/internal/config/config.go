// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"strconv"
	"strings"
)

// StaticServiceKey defines a pre-configured service key loaded from STUDIO_SERVICE_KEYS.
type StaticServiceKey struct {
	Name   string
	Role   string // "reader", "editor", "admin"
	Secret string
	Groups []string
}

// Config holds every runtime setting of the Studio Management & Integration server.
type Config struct {
	Addr             string
	DatabaseURL      string
	OIDCBaseURL      string
	OIDCAllowedSlugs []string
	OIDCIssuers      []string
	OIDCAudiences    []string
	InternalKey      string
	StaticKeys       []StaticServiceKey

	AuthentikAPIURL   string
	AuthentikAPIToken string

	PublicURL      string
	IntranetURL    string
	LoreRemoteBase string
	LoreBridgeURL  string
	LoreAuthURL    string
	OIDCClientIDs  map[string]string
	MaxUploadBytes int64
	CORSOrigins    []string

	AdminGroups    []string
	EmployeeGroups []string
	PartnerGroups  []string
}

// ClientIDFor returns the configured Tellurian Authentik OAuth2 client_id for an application slug.
func (c Config) ClientIDFor(slug string) string {
	s := strings.ToLower(strings.TrimSpace(slug))
	if s == "" {
		return ""
	}
	if c.OIDCClientIDs != nil {
		if cid, ok := c.OIDCClientIDs[s]; ok && cid != "" {
			return cid
		}
	}
	if s == "rfs-intranet" {
		return "intranet-rfs"
	}
	return s
}

const defaultMaxUploadMB = 50

// DefaultAllowedSlugs lists the standard Rogue Fairy Studios OIDC application slugs in Tellurian Authentik.
var DefaultAllowedSlugs = []string{
	"aetherion",
	"geppetto",
	"rfs-intranet",
	"rfs-lore-cli",
	"rfs-lore-ci",
	"goethe",
	"animafaber",
	"fontpit",
	"carnifex",
	"rfs-studio",
	"studio",
}

// Load parses configuration from getenv.
func Load(getenv func(string) string) (Config, error) {
	dbURL := strings.TrimSpace(getenv("STUDIO_DATABASE_URL"))
	if dbURL == "" {
		dbURL = strings.TrimSpace(getenv("DATABASE_URL"))
	}

	oidcBase := strings.TrimRight(strings.TrimSpace(getenv("STUDIO_OIDC_BASE_URL")), "/")
	if oidcBase == "" {
		oidcBase = "https://auth.telluriancorp.info"
	}

	authentikAPI := strings.TrimRight(strings.TrimSpace(getenv("STUDIO_AUTHENTIK_API_URL")), "/")
	if authentikAPI == "" {
		authentikAPI = oidcBase
	}

	c := Config{
		Addr:              strings.TrimSpace(getenv("STUDIO_ADDR")),
		DatabaseURL:       dbURL,
		OIDCBaseURL:       oidcBase,
		OIDCAllowedSlugs:  splitCSV(getenv("STUDIO_OIDC_ALLOWED_SLUGS"), DefaultAllowedSlugs...),
		OIDCIssuers:       splitCSV(getenv("STUDIO_OIDC_ISSUERS")),
		OIDCAudiences:     splitCSV(getenv("STUDIO_OIDC_AUDIENCES")),
		InternalKey:       strings.TrimSpace(getenv("STUDIO_INTERNAL_KEY")),
		AuthentikAPIURL:   authentikAPI,
		AuthentikAPIToken: strings.TrimSpace(getenv("STUDIO_AUTHENTIK_API_TOKEN")),
		PublicURL:         strings.TrimRight(strings.TrimSpace(getenv("STUDIO_PUBLIC_URL")), "/"),
		IntranetURL:       strings.TrimRight(orDefault(strings.TrimSpace(getenv("STUDIO_INTRANET_URL")), "https://docs.roguefairy.net"), "/"),
		LoreRemoteBase:    strings.TrimRight(orDefault(strings.TrimSpace(getenv("STUDIO_LORE_REMOTE_BASE")), "lores://lore.roguefairy.net"), "/"),
		LoreBridgeURL:     strings.TrimRight(orDefault(strings.TrimSpace(getenv("STUDIO_LORE_BRIDGE_URL")), "https://lore-auth.roguefairy.net"), "/"),
		LoreAuthURL:       strings.TrimRight(orDefault(strings.TrimSpace(getenv("STUDIO_LORE_AUTH_URL")), oidcBase), "/"),
		OIDCClientIDs:     parseClientIDMap(getenv("STUDIO_OIDC_CLIENT_IDS")),
		CORSOrigins: splitCSV(
			getenv("STUDIO_CORS_ORIGINS"),
			"https://docs.roguefairy.net",
			"http://localhost:3000",
			"http://127.0.0.1:3000",
			"http://localhost:5173",
			"http://127.0.0.1:5173",
		),
		AdminGroups: splitCSV(
			getenv("STUDIO_GROUPS_ADMIN"),
			"Rogue Fairy Studios - C-Level",
			"authentik Admins",
		),
		EmployeeGroups: splitCSV(
			getenv("STUDIO_GROUPS_EMPLOYEES"),
			"Rogue Fairy Studios",
		),
		PartnerGroups: splitCSV(
			getenv("STUDIO_GROUPS_PARTNERS"),
			"Rogue Fairy Studios - Partners",
		),
	}

	if c.Addr == "" {
		if port := strings.TrimSpace(getenv("PORT")); port != "" {
			c.Addr = ":" + port
		} else {
			c.Addr = ":8080"
		}
	}

	mb := int64(defaultMaxUploadMB)
	if raw := strings.TrimSpace(getenv("STUDIO_MAX_UPLOAD_MB")); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v <= 0 || v > 500 {
			return Config{}, fmt.Errorf("STUDIO_MAX_UPLOAD_MB must be between 1 and 500, got %q", raw)
		}
		mb = v
	}
	c.MaxUploadBytes = mb << 20

	keys, err := parseStaticKeys(getenv("STUDIO_SERVICE_KEYS"))
	if err != nil {
		return Config{}, err
	}
	c.StaticKeys = keys

	return c, nil
}

func parseStaticKeys(raw string) ([]StaticServiceKey, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var out []StaticServiceKey
	for idx, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 3)
		switch len(parts) {
		case 1:
			out = append(out, StaticServiceKey{
				Name:   fmt.Sprintf("bootstrap-%d", idx+1),
				Role:   "admin",
				Secret: parts[0],
			})
		case 3:
			name := strings.TrimSpace(parts[0])
			role := strings.ToLower(strings.TrimSpace(parts[1]))
			secret := strings.TrimSpace(parts[2])
			if name == "" || secret == "" || (role != "reader" && role != "editor" && role != "admin") {
				return nil, fmt.Errorf("invalid STUDIO_SERVICE_KEYS entry at index %d (expected name:role:secret)", idx)
			}
			out = append(out, StaticServiceKey{
				Name:   name,
				Role:   role,
				Secret: secret,
			})
		default:
			return nil, fmt.Errorf("invalid STUDIO_SERVICE_KEYS entry at index %d", idx)
		}
	}
	return out, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func parseClientIDMap(raw string) map[string]string {
	out := map[string]string{
		"rfs-intranet": "intranet-rfs",
	}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		sep := strings.IndexAny(entry, ":=")
		if sep <= 0 || sep == len(entry)-1 {
			continue
		}
		slug := strings.ToLower(strings.TrimSpace(entry[:sep]))
		cid := strings.TrimSpace(entry[sep+1:])
		if slug != "" && cid != "" {
			out[slug] = cid
		}
	}
	return out
}

func splitCSV(raw string, defaults ...string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return defaults
	}
	return out
}
