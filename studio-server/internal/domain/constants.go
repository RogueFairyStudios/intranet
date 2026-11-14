// Package domain implements the Rogue Fairy Studios project management rules,
// access policies, Lore VCS namespaces, storefront compliance presets, and cross-tool queries.
package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// ValidationError marks domain input or permission violations (HTTP 400 / 403).
type ValidationError struct {
	Message   string
	Forbidden bool
}

func (e ValidationError) Error() string { return e.Message }

func errBadInput(msg string) error { return ValidationError{Message: msg} }
func errForbidden(msg string) error {
	return ValidationError{Message: msg, Forbidden: true}
}

var (
	States            = []string{"backlog", "ready", "active", "review", "done"}
	Phases            = []string{"pre_production", "production", "post_production", "live"}
	Engines           = []string{"aetherion", "godot", "carnifex", "uefn", "unity", "custom", "tabletop"}
	Stages            = []string{"concept", "blockout", "authoring", "surface_loc", "engine_integration", "polish"}
	Tracks            = []string{"general", "art_2d", "art_3d", "narrative", "spirit", "scene_level", "audio_vfx", "code_system", "typography", "tabletop_print"}
	Tools             = []string{"generic", "goethe", "geppetto", "aetherion", "godot", "carnifex", "animafaber", "fontpit", "lore", "urc", "uefn", "unity", "git", "github"}
	ConfigTools       = []string{"authentik", "aetherion", "geppetto", "lore", "goethe", "carnifex", "animafaber", "fontpit", "uri-handler", "all"}
	VCSSystems        = []string{"git", "lore", "urc"}
	URIProtocols      = []string{"https:", "http:", "file:", "goethe:", "geppetto:", "lore:", "lores:", "aetherion:", "godot:", "res:", "uid:", "carnifex:", "animafaber:", "fontpit:", "uefn:", "com.epicgames.launcher:"}
	RepoProtocols     = []string{"https:", "http:", "ssh:", "git:", "lore:", "lores:", "urc:", "urcs:", "uefn:", "com.epicgames.launcher:"}
	LoreActions       = []string{"read", "write", "owner", "admin", "migrate", "push-protected", "obliterate", "presign"}
	LoreVisibilities  = []string{"internal", "restricted"}
	LoreEmployeeRoles = []string{"none", "reader", "editor"}

	Collections = []string{
		"access",
		"groupAccess",
		"participants",
		"tasks",
		"documents",
		"releases",
		"credits",
		"builds",
		"history",
		"teams",
		"milestones",
		"cycles",
		"artifacts",
		"artifactVersions",
		"artifactReviews",
		"validations",
		"repositories",
		"platformTargets",
		"integrationEvents",
		"integrationAccess",
	}
)

const (
	GroupCLevel       = "Rogue Fairy Studios - C-Level"
	GroupAdmins       = "authentik Admins"
	GroupEmployees    = "Rogue Fairy Studios"
	GroupPartners     = "Rogue Fairy Studios - Partners"
	GroupLoreCI       = "Rogue Fairy Studios - Lore CI"
	GroupProjectsRoot = "Rogue Fairy Studios - Lore Projects"

	DefaultLoreRemoteBase = "lores://lore.roguefairy.net"
	DefaultLoreBridgeURL  = "https://lore-auth.roguefairy.net"
	DefaultLoreAuthURL    = "https://auth.telluriancorp.info"
)

var (
	reLoreProjectSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	reLoreSegment     = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	reRepoSlug        = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	reGitSCPRemote    = regexp.MustCompile(`^git@[A-Za-z0-9._-]+:[A-Za-z0-9._/-]+(?:\.git)?$`)
	reSHA256Hex       = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
	reYAMLSafeScalar  = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
	reNonSlugChars    = regexp.MustCompile(`[^a-z0-9._-]+`)
	reMultiDash       = regexp.MustCompile(`-+`)
	reTrimSlugEdges   = regexp.MustCompile(`^[^a-z0-9]+|[-._]+$`)
)

// NewUUID generates a RFC 4122 UUID v4 string.
func NewUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("crypto/rand failed: %w", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// SHA256Hex computes the lowercase hex SHA-256 digest of data.
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hasControlChars(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c <= 8 || c == 11 || c == 12 || (c >= 14 && c <= 31) || c == 127 {
			return true
		}
	}
	return false
}

// SafeText validates a non-empty string up to maximum runes/chars with no control characters.
func SafeText(raw string, maximum int) (string, error) {
	if maximum <= 0 {
		maximum = 500
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || len([]rune(raw)) > maximum {
		return "", errBadInput("Invalid text")
	}
	if hasControlChars(raw) {
		return "", errBadInput("Invalid control characters")
	}
	return trimmed, nil
}

// SafeURI validates an allowed Studio protocol URI without embedded credentials.
func SafeURI(raw string) (string, error) {
	cleaned, err := SafeText(raw, 4000)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(cleaned)
	if err != nil || u.Scheme == "" {
		return "", errBadInput("Unsupported artifact location")
	}
	proto := strings.ToLower(u.Scheme) + ":"
	if !slices.Contains(URIProtocols, proto) {
		return "", errBadInput("Unsupported artifact location")
	}
	if u.User != nil && u.User.String() != "" {
		return "", errBadInput("Artifact location must not contain credentials")
	}
	return cleaned, nil
}

// SafeEndpointURL validates an HTTPS (or loopback HTTP) or Lore endpoint URL.
func SafeEndpointURL(raw string, allowedProtos []string, label string) (string, error) {
	cleaned, err := SafeText(raw, 1000)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(cleaned)
	if err != nil || u.Scheme == "" {
		return "", errBadInput(fmt.Sprintf("Unsupported %s protocol", label))
	}
	proto := strings.ToLower(u.Scheme) + ":"
	if !slices.Contains(allowedProtos, proto) {
		return "", errBadInput(fmt.Sprintf("Unsupported %s protocol", label))
	}
	if u.User != nil && u.User.String() != "" {
		return "", errBadInput(fmt.Sprintf("%s must not contain credentials", label))
	}
	host := strings.ToLower(u.Hostname())
	if proto == "http:" && host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return "", errBadInput(fmt.Sprintf("%s requires HTTPS outside loopback", label))
	}
	return strings.TrimRight(cleaned, "/"), nil
}

// SafeRemoteURI validates a Git, Lore, or URC repository remote URI.
func SafeRemoteURI(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	cleaned, err := SafeText(raw, 2000)
	if err != nil {
		return "", err
	}
	if reGitSCPRemote.MatchString(cleaned) {
		return cleaned, nil
	}
	u, err := url.Parse(cleaned)
	if err != nil || u.Scheme == "" {
		return "", errBadInput("Unsupported repository remote URI")
	}
	proto := strings.ToLower(u.Scheme) + ":"
	if !slices.Contains(RepoProtocols, proto) {
		return "", errBadInput("Unsupported repository remote URI")
	}
	if u.User != nil {
		_, hasPass := u.User.Password()
		user := u.User.Username()
		if hasPass || (user != "" && !(proto == "ssh:" && user == "git")) {
			return "", errBadInput("Repository URI must not contain credentials")
		}
	}
	return cleaned, nil
}

// IsValidLoreProjectSlug checks whether s matches `[a-z0-9][a-z0-9._-]{0,63}`.
func IsValidLoreProjectSlug(s string) bool {
	return len(s) >= 1 && len(s) <= 64 && reLoreProjectSlug.MatchString(s)
}

// DeriveLoreProjectSlug derives a compliant Lore project slug from a human project name.
func DeriveLoreProjectSlug(name string) string {
	base := strings.ToLower(strings.TrimSpace(name))
	if base == "" {
		base = "project"
	}
	clean := reNonSlugChars.ReplaceAllString(base, "-")
	clean = reMultiDash.ReplaceAllString(clean, "-")
	clean = reTrimSlugEdges.ReplaceAllString(clean, "")
	if len(clean) > 64 {
		clean = clean[:64]
	}
	if IsValidLoreProjectSlug(clean) {
		return clean
	}
	return "project"
}

// ValidateLoreProjectSlug validates and normalizes a Lore project slug.
func ValidateLoreProjectSlug(raw string) (string, error) {
	clean, err := SafeText(raw, 64)
	if err != nil {
		return "", errBadInput("Invalid Lore project slug")
	}
	clean = strings.ToLower(clean)
	if !IsValidLoreProjectSlug(clean) {
		return "", errBadInput("Invalid Lore project slug")
	}
	return clean, nil
}

// ValidateLoreRepoPath validates a relative Lore repository path under a project slug.
func ValidateLoreRepoPath(raw string) (string, error) {
	clean, err := SafeText(raw, 900)
	if err != nil {
		return "", errBadInput("Invalid Lore repository name")
	}
	clean = strings.Trim(strings.ReplaceAll(clean, "\\", "/"), "/")
	if clean == "" {
		return "", errBadInput("Invalid Lore repository name")
	}
	for _, seg := range strings.Split(clean, "/") {
		if seg == "" || seg[0] == '.' || !reLoreSegment.MatchString(seg) {
			return "", errBadInput("Invalid Lore repository name")
		}
	}
	return clean, nil
}

// ParseLoreFullName splits `<project>/<repo>` and validates both parts.
func ParseLoreFullName(raw string) (project, repo, fullName string, err error) {
	clean, err := SafeText(raw, 1000)
	if err != nil {
		return "", "", "", errBadInput("Lore repository URI must use <project>/<repo> namespace")
	}
	clean = strings.Trim(strings.ReplaceAll(clean, "\\", "/"), "/")
	slash := strings.IndexByte(clean, '/')
	if slash <= 0 || slash == len(clean)-1 {
		return "", "", "", errBadInput("Lore repository URI must use <project>/<repo> namespace")
	}
	p, err := ValidateLoreProjectSlug(clean[:slash])
	if err != nil {
		return "", "", "", err
	}
	r, err := ValidateLoreRepoPath(clean[slash+1:])
	if err != nil {
		return "", "", "", err
	}
	return p, r, p + "/" + r, nil
}

// NormalizeLoreActions validates or derives Lore ACL actions in canonical order.
func NormalizeLoreActions(rawActions []string, hasExplicit bool, fallbackRole string) ([]string, error) {
	if !hasExplicit {
		switch fallbackRole {
		case "manager":
			return []string{"read", "write", "admin", "presign"}, nil
		case "editor":
			return []string{"read", "write"}, nil
		case "reader":
			return []string{"read"}, nil
		default:
			return []string{}, nil
		}
	}
	for _, a := range rawActions {
		if !slices.Contains(LoreActions, a) {
			return nil, errBadInput("Invalid Lore action")
		}
	}
	out := make([]string, 0, len(rawActions))
	for _, canonical := range LoreActions {
		if slices.Contains(rawActions, canonical) {
			out = append(out, canonical)
		}
	}
	return out, nil
}

// DefaultEngineForKind returns the default studio engine key for a project kind.
func DefaultEngineForKind(kind string) string {
	if kind == "analogue" {
		return "tabletop"
	}
	return "aetherion"
}

// IsValidationError reports whether err is a domain ValidationError and whether it represents HTTP 403 Forbidden.
func IsValidationError(err error) (ValidationError, bool) {
	var ve ValidationError
	if errors.As(err, &ve) {
		return ve, true
	}
	return ValidationError{}, false
}
