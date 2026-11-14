package domain

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/roguefairystudios/intranet/studio-server/internal/store"
)

// ConfigSchemaVersion identifies the Studio configuration manifest contract.
const ConfigSchemaVersion = "rfs.studio.config/v1"

// ConfigEnvOptions supplies studio-wide URLs and OIDC client_ids when rendering tool configurations.
type ConfigEnvOptions struct {
	StudioAPIURL   string
	IntranetURL    string
	OIDCBaseURL    string
	LoreRemoteBase string
	LoreBridgeURL  string
	LoreAuthURL    string
	OIDCClientIDs  map[string]string
	MaxUploadMB    int
}

func (o ConfigEnvOptions) withDefaults() ConfigEnvOptions {
	if strings.TrimSpace(o.IntranetURL) == "" {
		o.IntranetURL = "https://docs.roguefairy.net"
	}
	o.IntranetURL = strings.TrimRight(strings.TrimSpace(o.IntranetURL), "/")

	if strings.TrimSpace(o.StudioAPIURL) == "" {
		o.StudioAPIURL = o.IntranetURL + "/p/studio"
	}
	o.StudioAPIURL = strings.TrimRight(strings.TrimSpace(o.StudioAPIURL), "/")

	if strings.TrimSpace(o.OIDCBaseURL) == "" {
		o.OIDCBaseURL = DefaultLoreAuthURL
	}
	o.OIDCBaseURL = strings.TrimRight(strings.TrimSpace(o.OIDCBaseURL), "/")

	if strings.TrimSpace(o.LoreRemoteBase) == "" {
		o.LoreRemoteBase = DefaultLoreRemoteBase
	}
	o.LoreRemoteBase = strings.TrimRight(strings.TrimSpace(o.LoreRemoteBase), "/")

	if strings.TrimSpace(o.LoreBridgeURL) == "" {
		o.LoreBridgeURL = DefaultLoreBridgeURL
	}
	o.LoreBridgeURL = strings.TrimRight(strings.TrimSpace(o.LoreBridgeURL), "/")

	if strings.TrimSpace(o.LoreAuthURL) == "" {
		o.LoreAuthURL = o.OIDCBaseURL
	}
	o.LoreAuthURL = strings.TrimRight(strings.TrimSpace(o.LoreAuthURL), "/")

	if o.MaxUploadMB <= 0 {
		o.MaxUploadMB = 50
	}
	return o
}

func (o ConfigEnvOptions) clientIDFor(slug string) string {
	s := strings.ToLower(strings.TrimSpace(slug))
	if o.OIDCClientIDs != nil {
		if cid, ok := o.OIDCClientIDs[s]; ok && cid != "" {
			return cid
		}
	}
	if s == "rfs-intranet" {
		return "intranet-rfs"
	}
	return s
}

// NormalizeConfigTool extracts an optional .json/.yaml/.yml extension from raw and validates the tool key.
func NormalizeConfigTool(raw string) (tool string, extFormat string, err error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.HasSuffix(s, ".yaml"):
		extFormat = "yaml"
		s = strings.TrimSuffix(s, ".yaml")
	case strings.HasSuffix(s, ".yml"):
		extFormat = "yaml"
		s = strings.TrimSuffix(s, ".yml")
	case strings.HasSuffix(s, ".json"):
		extFormat = "json"
		s = strings.TrimSuffix(s, ".json")
	}

	switch s {
	case "uri_handler", "urihandler":
		s = "uri-handler"
	case "bundle", "workstation":
		s = "all"
	}

	if s == "godot" || slices.Contains(ConfigTools, s) {
		return s, extFormat, nil
	}
	return "", "", errBadInput("Unsupported configuration tool profile")
}

// ResolveVisibleProject resolves a project by either exact UUID or Lore/derived project slug,
// failing closed with store.ErrNotFound whenever the project does not exist or caller/target lacks access.
func ResolveVisibleProject(
	ctx context.Context,
	st store.Store,
	policy PolicyConfig,
	projectRef string,
	target SubjectProfile,
	viewer *SubjectProfile,
) (*store.ProjectRow, map[string]any, string, error) {
	ref := strings.TrimSpace(projectRef)
	if ref == "" || st == nil {
		return nil, nil, "", store.ErrNotFound
	}

	checkVisible := func(row store.ProjectRow) (*store.ProjectRow, map[string]any, string, bool) {
		state, err := ParseState(row.State)
		if err != nil {
			return nil, nil, "", false
		}
		if viewer != nil && !viewer.IsAdmin && RoleFor(state, *viewer, policy) == "" {
			return nil, nil, "", false
		}
		role := RoleFor(state, target, policy)
		if role == "" {
			return nil, nil, "", false
		}
		cp := row
		return &cp, state, role, true
	}

	if exact, err := st.GetProject(ctx, ref); err == nil && exact != nil {
		if r, stMap, role, ok := checkVisible(*exact); ok {
			return r, stMap, role, nil
		}
		return nil, nil, "", store.ErrNotFound
	}

	rows, err := st.ListProjects(ctx)
	if err != nil {
		return nil, nil, "", err
	}
	for _, row := range rows {
		r, stMap, role, ok := checkVisible(row)
		if !ok {
			continue
		}
		loreMap, _ := stMap["lore"].(map[string]any)
		loreSlug := strVal(loreMap["projectSlug"])
		derivedSlug := DeriveLoreProjectSlug(strVal(stMap["name"]))
		if strings.EqualFold(ref, loreSlug) || strings.EqualFold(ref, derivedSlug) {
			return r, stMap, role, nil
		}
	}
	return nil, nil, "", store.ErrNotFound
}

// ConfigCatalogEntry describes an available downloadable JSON/YAML configuration profile.
type ConfigCatalogEntry struct {
	Tool        string `json:"tool"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Recommended bool   `json:"recommended,omitempty"`
	JSONURL     string `json:"jsonUrl"`
	YAMLURL     string `json:"yamlUrl"`
}

// StudioConfigCatalog lists generalist Studio tool configuration endpoints.
type StudioConfigCatalog struct {
	SchemaVersion string               `json:"schemaVersion"`
	Scope         string               `json:"scope"`
	Studio        StudioEndpointBlock  `json:"studio"`
	Subject       SubjectIdentity      `json:"subject"`
	Profiles      []ConfigCatalogEntry `json:"profiles"`
}

// ProjectConfigCatalog lists project-specific tool configuration endpoints.
type ProjectConfigCatalog struct {
	SchemaVersion string               `json:"schemaVersion"`
	Scope         string               `json:"scope"`
	Studio        StudioEndpointBlock  `json:"studio"`
	Subject       SubjectIdentity      `json:"subject"`
	Project       ProjectConfigMeta    `json:"project"`
	Profiles      []ConfigCatalogEntry `json:"profiles"`
}

// StudioEndpointBlock holds the Studio API and Intranet URLs.
type StudioEndpointBlock struct {
	Name        string `json:"name"`
	APIURL      string `json:"apiUrl"`
	IntranetURL string `json:"intranetUrl"`
	GraphQLURL  string `json:"graphqlUrl"`
}

// OIDCAppConfig holds the Tellurian Authentik OIDC parameters for a specific studio tool.
type OIDCAppConfig struct {
	BaseURL                     string `json:"baseUrl"`
	AppSlug                     string `json:"appSlug"`
	ClientID                    string `json:"clientId"`
	IssuerURL                   string `json:"issuerUrl"`
	OIDCConfigurationURL        string `json:"oidcConfigurationUrl"`
	AuthorizationEndpoint       string `json:"authorizationEndpoint"`
	TokenEndpoint               string `json:"tokenEndpoint"`
	UserinfoEndpoint            string `json:"userinfoEndpoint"`
	DeviceAuthorizationEndpoint string `json:"deviceAuthorizationEndpoint"`
	RevocationEndpoint          string `json:"revocationEndpoint"`
	Scopes                      string `json:"scopes"`
	PKCEMethod                  string `json:"pkceMethod"`
	LoopbackHost                string `json:"loopbackHost,omitempty"`
	RedirectPort                int    `json:"redirectPort"`
	RedirectPath                string `json:"redirectPath,omitempty"`
	RedirectURI                 string `json:"redirectUri,omitempty"`
	LoopbackRedirectRegex       string `json:"loopbackRedirectRegex,omitempty"`
}

// LoreEndpointConfig describes the Lore VCS server & OIDC bridge endpoints.
type LoreEndpointConfig struct {
	DriverID        string   `json:"driverId"`
	RemoteBase      string   `json:"remoteBase"`
	BridgeURL       string   `json:"bridgeUrl"`
	AuthURL         string   `json:"authUrl"`
	CLIAppSlug      string   `json:"cliAppSlug"`
	CIAppSlug       string   `json:"ciAppSlug,omitempty"`
	CASCompression  string   `json:"casCompression"`
	NamespaceFormat string   `json:"namespaceFormat"`
	ProjectSlug     string   `json:"projectSlug,omitempty"`
	Namespace       string   `json:"namespace,omitempty"`
	Visibility      string   `json:"visibility,omitempty"`
	LoginCommand    []string `json:"loginCommand"`
}

// ProjectConfigMeta identifies a Studio project inside a configuration manifest.
type ProjectConfigMeta struct {
	ID              string            `json:"id"`
	Revision        int               `json:"revision"`
	Slug            string            `json:"slug"`
	Name            string            `json:"name"`
	Kind            string            `json:"kind"`
	Engine          string            `json:"engine"`
	UsesAetherion   bool              `json:"usesAetherion"`
	Phase           string            `json:"phase"`
	Archived        bool              `json:"archived"`
	IntranetURL     string            `json:"intranetUrl"`
	GodotURI        string            `json:"godotUri,omitempty"`
	AetherionURI    string            `json:"aetherionUri,omitempty"`
	GeppettoURI     string            `json:"geppettoUri,omitempty"`
	GoetheURI       string            `json:"goetheUri,omitempty"`
	ActiveMilestone *MilestoneSummary `json:"activeMilestone,omitempty"`
	ActiveCycle     *CycleSummary     `json:"activeCycle,omitempty"`
}

// GroupGrantSummary summarizes a Tellurian Authentik group grant on a project.
type GroupGrantSummary struct {
	GroupID           int      `json:"groupId,omitempty"`
	GroupName         string   `json:"groupName"`
	StudioRole        string   `json:"studioRole"`
	LoreActions       []string `json:"loreActions"`
	LoreCanCreateRepo bool     `json:"loreCanCreateRepo"`
}

// ProjectAccessBlock details the target user's effective permissions on the project and Lore VCS.
type ProjectAccessBlock struct {
	StudioRole        string              `json:"studioRole"`
	CanManage         bool                `json:"canManage"`
	CanEdit           bool                `json:"canEdit"`
	CanPublishArtwork bool                `json:"canPublishArtwork"`
	ParticipantIDs    []string            `json:"participantIds"`
	ParticipantRoles  []string            `json:"participantRoles"`
	Lore              EffectiveLoreGrant  `json:"lore"`
	GroupGrants       []GroupGrantSummary `json:"groupGrants"`
}

func buildStudioBlock(opts ConfigEnvOptions) StudioEndpointBlock {
	return StudioEndpointBlock{
		Name:        "Rogue Fairy Studios",
		APIURL:      opts.StudioAPIURL,
		IntranetURL: opts.IntranetURL,
		GraphQLURL:  opts.IntranetURL + "/graphql",
	}
}

func buildOIDCApp(opts ConfigEnvOptions, slug string, redirectPort int) OIDCAppConfig {
	base := opts.OIDCBaseURL
	issuer := base + "/application/o/" + slug
	cfg := OIDCAppConfig{
		BaseURL:                     base,
		AppSlug:                     slug,
		ClientID:                    opts.clientIDFor(slug),
		IssuerURL:                   issuer,
		OIDCConfigurationURL:        issuer + "/.well-known/openid-configuration",
		AuthorizationEndpoint:       base + "/application/o/authorize/",
		TokenEndpoint:               base + "/application/o/token/",
		UserinfoEndpoint:            base + "/application/o/userinfo/",
		DeviceAuthorizationEndpoint: base + "/application/o/device/",
		RevocationEndpoint:          base + "/application/o/revoke/",
		Scopes:                      "openid profile email offline_access",
		PKCEMethod:                  "S256",
		LoopbackHost:                "127.0.0.1",
		RedirectPort:                redirectPort,
		RedirectPath:                "/callback",
		LoopbackRedirectRegex:       `^http://127\.0\.0\.1(:[0-9]+)?/callback$`,
	}
	if redirectPort > 0 {
		cfg.RedirectURI = fmt.Sprintf("http://127.0.0.1:%d/callback", redirectPort)
	}
	return cfg
}

func buildLoreBlock(opts ConfigEnvOptions, stateLore map[string]any) LoreEndpointConfig {
	remoteBase := opts.LoreRemoteBase
	bridgeURL := opts.LoreBridgeURL
	authURL := opts.LoreAuthURL
	var projSlug, ns, vis string
	if stateLore != nil {
		if v := strVal(stateLore["remoteBase"]); v != "" {
			remoteBase = strings.TrimRight(v, "/")
		}
		if v := strVal(stateLore["bridgeUrl"]); v != "" {
			bridgeURL = strings.TrimRight(v, "/")
		}
		if v := strVal(stateLore["authUrl"]); v != "" {
			authURL = strings.TrimRight(v, "/")
		}
		projSlug = strVal(stateLore["projectSlug"])
		if projSlug != "" {
			ns = projSlug + "/*"
		}
		vis = strVal(stateLore["visibility"])
	}
	return LoreEndpointConfig{
		DriverID:        "lore",
		RemoteBase:      remoteBase,
		BridgeURL:       bridgeURL,
		AuthURL:         authURL,
		CLIAppSlug:      "rfs-lore-cli",
		CIAppSlug:       "rfs-lore-ci",
		CASCompression:  "zstd",
		NamespaceFormat: "<project>/<repo>",
		ProjectSlug:     projSlug,
		Namespace:       ns,
		Visibility:      vis,
		LoginCommand:    []string{"lore", "auth", "login", "--auth-url", bridgeURL},
	}
}

func toolCatalogMetadata() []struct {
	tool, title, desc string
} {
	return []struct {
		tool, title, desc string
	}{
		{"aetherion", "Aetherion Engine", "Engine configuration (Tellurian Authentik SSO, Studio API, Git/Lore workspace & project settings)"},
		{"geppetto", "Geppetto (2D Art & Animation)", "Artist & animator configuration (Tellurian Authentik, Lore VCS server, repositories, locks & access levels)"},
		{"authentik", "Tellurian Authentik SSO", "OIDC provider endpoints, PKCE loopback rules, studio groups & application slugs"},
		{"lore", "Lore VCS (GhostLore)", "Lore VCS server, OIDC bridge, namespace permissions & repository clone commands"},
		{"goethe", "Goethe Narrative Studio", "Dialogue & narrative workspace (.gproj.yaml template, .gdkg zstd archive & narrative assets)"},
		{"carnifex", "Carnifex Build & Release", "Build pipeline, release targets, approved credits snapshot & storefront gates"},
		{"animafaber", "Animafaber", "Character & spirit pipeline OIDC loopback (port 44200), Studio API & assets"},
		{"fontpit", "Fontpit", "Typography & font pipeline OIDC, Studio API & assets"},
		{"uri-handler", "Rogue URI Handler", "Workstation URI scheme dispatcher (geppetto://, goethe://, aetherion://, godot://, lore://)"},
		{"all", "All Studio Tools Bundle", "Complete composite workstation configuration across all Rogue Fairy Studios tools"},
	}
}

// BuildStudioConfigCatalog lists all generalist Studio tool configuration profiles.
func BuildStudioConfigCatalog(subj SubjectProfile, opts ConfigEnvOptions) StudioConfigCatalog {
	opts = opts.withDefaults()
	meta := toolCatalogMetadata()
	profiles := make([]ConfigCatalogEntry, 0, len(meta))
	for _, m := range meta {
		base := opts.StudioAPIURL
		pathPrefix := "/v1/configs/"
		if strings.HasSuffix(base, "/p/studio") {
			pathPrefix = "/configs/"
		}
		profiles = append(profiles, ConfigCatalogEntry{
			Tool:        m.tool,
			Title:       m.title,
			Description: m.desc,
			Recommended: m.tool == "aetherion" || m.tool == "geppetto" || m.tool == "lore",
			JSONURL:     base + pathPrefix + m.tool + "?format=json",
			YAMLURL:     base + pathPrefix + m.tool + "?format=yaml",
		})
	}
	return StudioConfigCatalog{
		SchemaVersion: ConfigSchemaVersion,
		Scope:         "studio",
		Studio:        buildStudioBlock(opts),
		Subject:       toSubjectIdentity(subj),
		Profiles:      profiles,
	}
}

// BuildProjectConfigCatalog lists all project-specific tool configuration profiles for a project.
func BuildProjectConfigCatalog(
	ctx context.Context,
	st store.Store,
	policy PolicyConfig,
	row store.ProjectRow,
	state map[string]any,
	target SubjectProfile,
	opts ConfigEnvOptions,
) ProjectConfigCatalog {
	opts = opts.withDefaults()
	UpgradeState(state)
	loreGrant := EffectiveLoreGrantFor(state, target, policy)
	projMeta := buildProjectMeta(row, state, loreGrant, opts)

	base := opts.StudioAPIURL
	projPrefix := "/v1/projects/" + projMeta.Slug + "/configs/"
	if strings.HasSuffix(base, "/p/studio") {
		projPrefix = "/projects/" + projMeta.Slug + "/configs/"
	}

	meta := toolCatalogMetadata()
	profiles := make([]ConfigCatalogEntry, 0, len(meta))
	for _, m := range meta {
		rec := m.tool == "geppetto" || m.tool == "lore" || (m.tool == "aetherion" && projMeta.Engine == "aetherion")
		profiles = append(profiles, ConfigCatalogEntry{
			Tool:        m.tool,
			Title:       m.title,
			Description: m.desc,
			Recommended: rec,
			JSONURL:     base + projPrefix + m.tool + "?format=json",
			YAMLURL:     base + projPrefix + m.tool + "?format=yaml",
		})
	}
	_ = ctx
	_ = st
	return ProjectConfigCatalog{
		SchemaVersion: ConfigSchemaVersion,
		Scope:         "project",
		Studio:        buildStudioBlock(opts),
		Subject:       toSubjectIdentity(target),
		Project:       projMeta,
		Profiles:      profiles,
	}
}

// BuildStudioToolConfig builds a generalist (studio-wide) configuration manifest for tool.
func BuildStudioToolConfig(tool string, subj SubjectProfile, opts ConfigEnvOptions) (any, string, []string, error) {
	opts = opts.withDefaults()
	normTool, _, err := NormalizeConfigTool(tool)
	if err != nil {
		return nil, "", nil, err
	}

	studioBlk := buildStudioBlock(opts)
	ident := toSubjectIdentity(subj)
	genAt := nowISO()
	filenameBase := "roguefairy-" + normTool
	headers := []string{
		fmt.Sprintf("Rogue Fairy Studios — Generalist Tool Configuration (%s)", normTool),
		fmt.Sprintf("Schema: %s | Generated: %s", ConfigSchemaVersion, genAt),
	}

	switch normTool {
	case "authentik":
		apps := make(map[string]OIDCAppConfig)
		for _, slug := range []string{"aetherion", "geppetto", "rfs-intranet", "rfs-lore-cli", "rfs-lore-ci", "goethe", "animafaber", "fontpit", "carnifex", "rfs-studio"} {
			port := 0
			if slug == "animafaber" {
				port = 44200
			}
			apps[slug] = buildOIDCApp(opts, slug, port)
		}
		doc := map[string]any{
			"schemaVersion": ConfigSchemaVersion,
			"generatedAt":   genAt,
			"scope":         "studio",
			"tool":          "authentik",
			"studio":        studioBlk,
			"subject":       ident,
			"authentik": map[string]any{
				"baseUrl":                     opts.OIDCBaseURL,
				"issuerTemplate":              opts.OIDCBaseURL + "/application/o/{slug}/",
				"authorizationEndpoint":       opts.OIDCBaseURL + "/application/o/authorize/",
				"tokenEndpoint":               opts.OIDCBaseURL + "/application/o/token/",
				"userinfoEndpoint":            opts.OIDCBaseURL + "/application/o/userinfo/",
				"deviceAuthorizationEndpoint": opts.OIDCBaseURL + "/application/o/device/",
				"revocationEndpoint":          opts.OIDCBaseURL + "/application/o/revoke/",
				"pkceMethod":                  "S256",
				"defaultScopes":               []string{"openid", "profile", "email", "offline_access"},
				"loopbackRedirectRegex":       `^http://127\.0\.0\.1(:[0-9]+)?/callback$`,
				"groups": map[string]string{
					"cLevel":       GroupCLevel,
					"admins":       GroupAdmins,
					"employees":    GroupEmployees,
					"partners":     GroupPartners,
					"loreCi":       GroupLoreCI,
					"projectsRoot": GroupProjectsRoot,
				},
				"applications": apps,
			},
		}
		return doc, filenameBase, headers, nil

	case "aetherion", "godot":
		doc := buildGeneralistAetherionMap(normTool, genAt, studioBlk, ident, opts)
		return doc, filenameBase, headers, nil

	case "geppetto":
		doc := buildGeneralistGeppettoMap(genAt, studioBlk, ident, opts)
		return doc, filenameBase, headers, nil

	case "lore":
		doc := buildGeneralistLoreMap(genAt, studioBlk, ident, opts)
		return doc, filenameBase, headers, nil

	case "goethe":
		doc := buildGeneralistGoetheMap(genAt, studioBlk, ident, opts)
		return doc, filenameBase, headers, nil

	case "carnifex":
		doc := buildGeneralistCarnifexMap(genAt, studioBlk, ident, opts)
		return doc, filenameBase, headers, nil

	case "animafaber":
		doc := buildGeneralistAnimafaberMap(genAt, studioBlk, ident, opts)
		return doc, filenameBase, headers, nil

	case "fontpit":
		doc := buildGeneralistFontpitMap(genAt, studioBlk, ident, opts)
		return doc, filenameBase, headers, nil

	case "uri-handler":
		doc := buildGeneralistURIHandlerMap(genAt, studioBlk, ident, opts)
		return doc, filenameBase, headers, nil

	case "all":
		doc := map[string]any{
			"schemaVersion": ConfigSchemaVersion,
			"generatedAt":   genAt,
			"scope":         "studio",
			"tool":          "all",
			"studio":        studioBlk,
			"subject":       ident,
			"authentik": map[string]any{
				"baseUrl":               opts.OIDCBaseURL,
				"loopbackRedirectRegex": `^http://127\.0\.0\.1(:[0-9]+)?/callback$`,
				"groups": map[string]string{
					"cLevel":       GroupCLevel,
					"admins":       GroupAdmins,
					"employees":    GroupEmployees,
					"partners":     GroupPartners,
					"loreCi":       GroupLoreCI,
					"projectsRoot": GroupProjectsRoot,
				},
			},
			"lore": buildLoreBlock(opts, nil),
			"tools": map[string]any{
				"aetherion":  buildGeneralistAetherionMap("aetherion", genAt, studioBlk, ident, opts),
				"geppetto":   buildGeneralistGeppettoMap(genAt, studioBlk, ident, opts),
				"lore":       buildGeneralistLoreMap(genAt, studioBlk, ident, opts),
				"goethe":     buildGeneralistGoetheMap(genAt, studioBlk, ident, opts),
				"carnifex":   buildGeneralistCarnifexMap(genAt, studioBlk, ident, opts),
				"animafaber": buildGeneralistAnimafaberMap(genAt, studioBlk, ident, opts),
				"fontpit":    buildGeneralistFontpitMap(genAt, studioBlk, ident, opts),
				"uriHandler": buildGeneralistURIHandlerMap(genAt, studioBlk, ident, opts),
			},
		}
		return doc, filenameBase, headers, nil
	}

	return nil, "", nil, errBadInput("Unsupported configuration tool profile")
}

func buildGeneralistAetherionMap(toolKey, genAt string, studioBlk StudioEndpointBlock, ident SubjectIdentity, opts ConfigEnvOptions) map[string]any {
	oidc := buildOIDCApp(opts, "aetherion", 0)
	lore := buildLoreBlock(opts, nil)
	engineName := "Aetherion Engine"
	repoSSH := "git@github.com:RogueFairyStudios/aetherion.git"
	repoHTTPS := "https://github.com/RogueFairyStudios/aetherion.git"
	branch := "aetherion"
	if toolKey == "godot" {
		engineName = "Godot Engine (Aetherion Compatible)"
	}
	return map[string]any{
		"schemaVersion": ConfigSchemaVersion,
		"generatedAt":   genAt,
		"scope":         "studio",
		"tool":          toolKey,
		"studio":        studioBlk,
		"subject":       ident,
		"authentik":     oidc,
		"lore":          lore,
		"engine": map[string]any{
			"key":               toolKey,
			"name":              engineName,
			"repository":        repoSSH,
			"httpsRepository":   repoHTTPS,
			"branch":            branch,
			"upstreamGodotBase": "4.6.0-dev",
		},
		"editorSettings": map[string]any{
			"network/authentik/issuer_url":      oidc.IssuerURL,
			"network/authentik/client_id":       oidc.ClientID,
			"network/authentik/scopes":          oidc.Scopes,
			"network/authentik/studio_api_url":  studioBlk.APIURL,
			"network/authentik/redirect_port":   0,
			"network/authentik/persist_session": true,
			"network/authentik/auto_refresh":    true,
		},
		"environment": map[string]string{
			"AETHERION_AUTHENTIK_ISSUER_URL": oidc.IssuerURL,
			"AETHERION_AUTHENTIK_CLIENT_ID":  oidc.ClientID,
			"AETHERION_AUTHENTIK_SCOPES":     oidc.Scopes,
			"AETHERION_STUDIO_API_URL":       studioBlk.APIURL,
		},
	}
}

func buildGeneralistGeppettoMap(genAt string, studioBlk StudioEndpointBlock, ident SubjectIdentity, opts ConfigEnvOptions) map[string]any {
	oidc := buildOIDCApp(opts, "geppetto", 0)
	lore := buildLoreBlock(opts, nil)
	return map[string]any{
		"schemaVersion": ConfigSchemaVersion,
		"generatedAt":   genAt,
		"scope":         "studio",
		"tool":          "geppetto",
		"studio":        studioBlk,
		"subject":       ident,
		"authentik":     oidc,
		"lore":          lore,
		"kritaVersioningSettings": map[string]any{
			"configGroup":         "Versioning",
			"driverId":            "lore",
			"serverAddress":       lore.RemoteBase,
			"authServerUrl":       lore.BridgeURL,
			"authentikIssuerUrl":  oidc.IssuerURL,
			"authentikClientId":   oidc.ClientID,
			"projectsRoot":        "~/Documents/Geppetto",
			"neverSyncedPatterns": []string{"*.kra~", ".*-autosave.kra", "*-autosave.kra"},
		},
		"rogueStudioPlugin": map[string]any{
			"clientApp":          "geppetto",
			"apiUrl":             studioBlk.APIURL,
			"graphqlUrl":         studioBlk.GraphQLURL,
			"workspacesEndpoint": studioBlk.APIURL + "/v1/me/workspaces?client=geppetto&vcs=lore",
		},
		"environment": map[string]string{
			"STUDIO_API_URL":    studioBlk.APIURL,
			"STUDIO_URL":        studioBlk.GraphQLURL,
			"STUDIO_CLIENT_APP": "geppetto",
			"LORE_REMOTE_BASE":  lore.RemoteBase,
			"LORE_AUTH_URL":     lore.BridgeURL,
		},
	}
}

func buildGeneralistLoreMap(genAt string, studioBlk StudioEndpointBlock, ident SubjectIdentity, opts ConfigEnvOptions) map[string]any {
	cliOIDC := buildOIDCApp(opts, "rfs-lore-cli", 0)
	ciOIDC := buildOIDCApp(opts, "rfs-lore-ci", 0)
	lore := buildLoreBlock(opts, nil)
	return map[string]any{
		"schemaVersion": ConfigSchemaVersion,
		"generatedAt":   genAt,
		"scope":         "studio",
		"tool":          "lore",
		"studio":        studioBlk,
		"subject":       ident,
		"authentik":     cliOIDC,
		"ciAuthentik":   ciOIDC,
		"lore":          lore,
		"environment": map[string]string{
			"LORE_REMOTE_BASE": lore.RemoteBase,
			"LORE_AUTH_URL":    lore.BridgeURL,
			"STUDIO_API_URL":   studioBlk.APIURL,
		},
	}
}

func buildGeneralistGoetheMap(genAt string, studioBlk StudioEndpointBlock, ident SubjectIdentity, opts ConfigEnvOptions) map[string]any {
	oidc := buildOIDCApp(opts, "goethe", 0)
	lore := buildLoreBlock(opts, nil)
	return map[string]any{
		"schemaVersion": ConfigSchemaVersion,
		"generatedAt":   genAt,
		"scope":         "studio",
		"tool":          "goethe",
		"studio":        studioBlk,
		"subject":       ident,
		"authentik":     oidc,
		"lore":          lore,
		"goethe": map[string]any{
			"schemaVersion":    1,
			"archiveExtension": ".gdkg",
			"compression":      "zstd",
			"uriScheme":        "goethe",
			"defaultDirs": map[string]string{
				"dialoguesDir": "dialogues",
				"portraitsDir": "portraits",
				"localesDir":   "locales",
			},
		},
		"environment": map[string]string{
			"STUDIO_API_URL":    studioBlk.APIURL,
			"STUDIO_URL":        studioBlk.GraphQLURL,
			"STUDIO_CLIENT_APP": "goethe",
		},
	}
}

func buildGeneralistCarnifexMap(genAt string, studioBlk StudioEndpointBlock, ident SubjectIdentity, opts ConfigEnvOptions) map[string]any {
	oidc := buildOIDCApp(opts, "carnifex", 0)
	lore := buildLoreBlock(opts, nil)
	return map[string]any{
		"schemaVersion": ConfigSchemaVersion,
		"generatedAt":   genAt,
		"scope":         "studio",
		"tool":          "carnifex",
		"studio":        studioBlk,
		"subject":       ident,
		"authentik":     oidc,
		"lore":          lore,
		"carnifex": map[string]any{
			"uriScheme":              "carnifex",
			"maxUploadMB":            opts.MaxUploadMB,
			"requireApprovedCredits": true,
		},
		"environment": map[string]string{
			"STUDIO_API_URL":    studioBlk.APIURL,
			"STUDIO_URL":        studioBlk.GraphQLURL,
			"STUDIO_CLIENT_APP": "carnifex",
		},
	}
}

func buildGeneralistAnimafaberMap(genAt string, studioBlk StudioEndpointBlock, ident SubjectIdentity, opts ConfigEnvOptions) map[string]any {
	oidc := buildOIDCApp(opts, "animafaber", 44200)
	lore := buildLoreBlock(opts, nil)
	return map[string]any{
		"schemaVersion": ConfigSchemaVersion,
		"generatedAt":   genAt,
		"scope":         "studio",
		"tool":          "animafaber",
		"studio":        studioBlk,
		"subject":       ident,
		"authentik":     oidc,
		"lore":          lore,
		"animafaber": map[string]any{
			"uriScheme":        "animafaber",
			"loopbackCallback": "http://127.0.0.1:44200/callback",
		},
		"environment": map[string]string{
			"STUDIO_API_URL":    studioBlk.APIURL,
			"STUDIO_URL":        studioBlk.GraphQLURL,
			"STUDIO_CLIENT_APP": "animafaber",
		},
	}
}

func buildGeneralistFontpitMap(genAt string, studioBlk StudioEndpointBlock, ident SubjectIdentity, opts ConfigEnvOptions) map[string]any {
	oidc := buildOIDCApp(opts, "fontpit", 0)
	lore := buildLoreBlock(opts, nil)
	return map[string]any{
		"schemaVersion": ConfigSchemaVersion,
		"generatedAt":   genAt,
		"scope":         "studio",
		"tool":          "fontpit",
		"studio":        studioBlk,
		"subject":       ident,
		"authentik":     oidc,
		"lore":          lore,
		"fontpit": map[string]any{
			"uriScheme":        "fontpit",
			"supportedFormats": []string{"ttf", "otf", "woff2", "fnt"},
		},
		"environment": map[string]string{
			"STUDIO_API_URL":    studioBlk.APIURL,
			"STUDIO_URL":        studioBlk.GraphQLURL,
			"STUDIO_CLIENT_APP": "fontpit",
		},
	}
}

func buildGeneralistURIHandlerMap(genAt string, studioBlk StudioEndpointBlock, ident SubjectIdentity, opts ConfigEnvOptions) map[string]any {
	return map[string]any{
		"schemaVersion": ConfigSchemaVersion,
		"generatedAt":   genAt,
		"scope":         "studio",
		"tool":          "uri-handler",
		"studio":        studioBlk,
		"subject":       ident,
		"uriHandler": map[string]any{
			"schemes":          []string{"aetherion", "godot", "geppetto", "goethe", "lore", "lores", "carnifex", "animafaber", "fontpit"},
			"apiUrl":           studioBlk.APIURL,
			"intranetUrl":      studioBlk.IntranetURL,
			"authentikBaseUrl": opts.OIDCBaseURL,
		},
	}
}

func buildProjectMeta(row store.ProjectRow, state map[string]any, loreGrant EffectiveLoreGrant, opts ConfigEnvOptions) ProjectConfigMeta {
	kind := strVal(state["kind"])
	engine := strVal(state["engine"])
	if !slices.Contains(Engines, engine) {
		engine = DefaultEngineForKind(kind)
	}
	slug := loreGrant.ProjectSlug
	if slug == "" {
		slug = DeriveLoreProjectSlug(strVal(state["name"]))
	}
	meta := ProjectConfigMeta{
		ID:              row.ID,
		Revision:        row.Revision,
		Slug:            slug,
		Name:            strVal(state["name"]),
		Kind:            kind,
		Engine:          engine,
		UsesAetherion:   engine == "aetherion",
		Phase:           strVal(state["phase"]),
		Archived:        boolVal(state["archived"]),
		IntranetURL:     opts.IntranetURL + "/p/studio?project=" + row.ID,
		GeppettoURI:     "geppetto://project/" + slug,
		GoetheURI:       "goethe://project/" + slug,
		ActiveMilestone: pickActiveMilestone(state),
		ActiveCycle:     pickActiveCycle(state),
	}
	if kind == "digital" || kind == "hybrid" {
		meta.GodotURI = "godot://project/" + slug
		if engine == "aetherion" {
			meta.AetherionURI = "aetherion://project/" + slug
		}
	}
	return meta
}

func collectProjectGroupGrants(ctx context.Context, st store.Store, state map[string]any, target SubjectProfile, includeAll bool) []GroupGrantSummary {
	grants := make([]GroupGrantSummary, 0)
	for _, entry := range sliceOfMaps(state["groupAccess"]) {
		gid, _ := intVal(entry["groupId"])
		gName := strVal(entry["groupName"])
		if gName == "" {
			gName = strVal(entry["name"])
		}
		if gName == "" && gid > 0 && st != nil {
			if wg, err := st.GetWikiGroupByID(ctx, gid); err == nil && wg != nil {
				gName = wg.Name
			}
		}
		if !includeAll {
			matched := (gid > 0 && slices.Contains(target.GroupIDs, gid)) ||
				(gName != "" && hasAnyGroupFold(target.GroupNames, []string{gName}))
			if !matched {
				continue
			}
		}
		acts, ok := strSlice(entry["loreActions"])
		if !ok {
			acts, _ = NormalizeLoreActions(nil, false, strVal(entry["role"]))
		}
		grants = append(grants, GroupGrantSummary{
			GroupID:           gid,
			GroupName:         gName,
			StudioRole:        strVal(entry["role"]),
			LoreActions:       acts,
			LoreCanCreateRepo: boolVal(entry["loreCanCreateRepo"]),
		})
	}
	return grants
}

func deriveGeppettoArtFolders(repos []RepositorySummary, artifacts []WorkspaceArtifact) []string {
	folders := make([]string, 0)
	addFolder := func(f string) {
		f = strings.Trim(strings.ReplaceAll(strings.TrimSpace(f), "\\", "/"), "/")
		if f == "" || f == "." {
			return
		}
		if idx := strings.IndexByte(f, '/'); idx > 0 {
			f = f[:idx]
		}
		for _, existing := range folders {
			if strings.EqualFold(existing, f) {
				return
			}
		}
		folders = append(folders, f)
	}
	for _, r := range repos {
		if r.System == "lore" && r.LoreRepo != "" {
			addFolder(r.LoreRepo)
		} else if r.WorkspacePath != "" && r.WorkspacePath != "." {
			addFolder(r.WorkspacePath)
		}
	}
	for _, a := range artifacts {
		if a.AssetPath != "" {
			addFolder(a.AssetPath)
		}
	}
	if len(folders) == 0 {
		return []string{"art", "characters", "animations", "environments", "ui", "capsules"}
	}
	return folders
}

func buildBootstrapCommands(lore LoreEndpointConfig, repos []RepositorySummary) [][]string {
	cmds := make([][]string, 0, len(repos)+1)
	hasLore := false
	for _, r := range repos {
		if r.System == "lore" {
			hasLore = true
			break
		}
	}
	if hasLore && len(lore.LoginCommand) > 0 {
		cmds = append(cmds, slices.Clone(lore.LoginCommand))
	}
	for _, r := range repos {
		if len(r.CloneCommand) == 0 {
			continue
		}
		cmd := slices.Clone(r.CloneCommand)
		if r.WorkspacePath != "" && r.WorkspacePath != "." {
			cmd = append(cmd, r.WorkspacePath)
		}
		cmds = append(cmds, cmd)
	}
	return cmds
}

func loreAccessLevelLabel(grant EffectiveLoreGrant) string {
	if slices.Contains(grant.Actions, "admin") || slices.Contains(grant.Actions, "owner") {
		return "admin"
	}
	if grant.CanWrite {
		return "read-write"
	}
	if grant.CanRead {
		return "read-only"
	}
	return "none"
}

// BuildProjectToolConfig builds a project-specific configuration manifest tailored to target's effective permissions.
func BuildProjectToolConfig(
	ctx context.Context,
	st store.Store,
	policy PolicyConfig,
	row store.ProjectRow,
	state map[string]any,
	target SubjectProfile,
	viewer *SubjectProfile,
	tool string,
	opts ConfigEnvOptions,
) (any, string, []string, error) {
	opts = opts.withDefaults()
	normTool, _, err := NormalizeConfigTool(tool)
	if err != nil {
		return nil, "", nil, err
	}

	UpgradeState(state)
	if viewer != nil && !viewer.IsAdmin && RoleFor(state, *viewer, policy) == "" {
		return nil, "", nil, store.ErrNotFound
	}
	role := RoleFor(state, target, policy)
	if role == "" {
		return nil, "", nil, store.ErrNotFound
	}

	studioBlk := buildStudioBlock(opts)
	ident := toSubjectIdentity(target)
	loreGrant := EffectiveLoreGrantFor(state, target, policy)
	stateLore, _ := state["lore"].(map[string]any)
	loreBlk := buildLoreBlock(opts, stateLore)
	projMeta := buildProjectMeta(row, state, loreGrant, opts)

	pIDs, pRoles := matchParticipantRecords(ctx, st, state, target)
	canSeeAllGroups := role == "manager" && (viewer == nil || viewer.IsAdmin || RoleFor(state, *viewer, policy) == "manager")
	groupGrants := collectProjectGroupGrants(ctx, st, state, target, canSeeAllGroups)

	accessBlk := ProjectAccessBlock{
		StudioRole:        role,
		CanManage:         role == "manager",
		CanEdit:           role == "manager" || role == "editor",
		CanPublishArtwork: role == "manager" || role == "editor",
		ParticipantIDs:    pIDs,
		ParticipantRoles:  pRoles,
		Lore:              loreGrant,
		GroupGrants:       groupGrants,
	}

	genAt := nowISO()
	filenameBase := projMeta.Slug + "-" + normTool
	headers := []string{
		fmt.Sprintf("Rogue Fairy Studios — Project Configuration: %s (%s)", projMeta.Name, normTool),
		fmt.Sprintf("Project Slug: %s | Engine: %s | Role: %s | Lore Access: %s", projMeta.Slug, projMeta.Engine, role, loreAccessLevelLabel(loreGrant)),
		fmt.Sprintf("Schema: %s | Generated: %s", ConfigSchemaVersion, genAt),
	}

	allRepos := summarizeRepositories(state, role, loreGrant, target, "")
	loreRepos := summarizeRepositories(state, role, loreGrant, target, "lore")

	switch normTool {
	case "authentik":
		resp := map[string]any{
			"schemaVersion": ConfigSchemaVersion,
			"generatedAt":   genAt,
			"scope":         "project",
			"tool":          "authentik",
			"studio":        studioBlk,
			"subject":       ident,
			"project":       projMeta,
			"access":        accessBlk,
			"authentik": map[string]any{
				"baseUrl":               opts.OIDCBaseURL,
				"loopbackRedirectRegex": `^http://127\.0\.0\.1(:[0-9]+)?/callback$`,
				"loreProjectAttribute":  projMeta.Slug,
				"loreVisibility":        loreGrant.Visibility,
				"effectiveLoreActions":  loreGrant.Actions,
				"applications": map[string]OIDCAppConfig{
					"aetherion": buildOIDCApp(opts, "aetherion", 0),
					"geppetto":  buildOIDCApp(opts, "geppetto", 0),
					"loreCli":   buildOIDCApp(opts, "rfs-lore-cli", 0),
					"goethe":    buildOIDCApp(opts, "goethe", 0),
				},
			},
		}
		if canSeeAllGroups {
			resp["authentikBlueprintYaml"] = ExportAuthentikBlueprint(state)
		}
		return resp, filenameBase, headers, nil

	case "aetherion", "godot":
		doc := buildProjectAetherionMap(normTool, genAt, studioBlk, ident, projMeta, accessBlk, loreBlk, state, pIDs, allRepos, opts)
		return doc, filenameBase, headers, nil

	case "geppetto":
		doc := buildProjectGeppettoMap(genAt, studioBlk, ident, projMeta, accessBlk, loreBlk, state, pIDs, loreRepos, allRepos, opts)
		return doc, filenameBase, headers, nil

	case "lore":
		doc := buildProjectLoreMap(genAt, studioBlk, ident, projMeta, accessBlk, loreBlk, loreRepos, opts)
		return doc, filenameBase, headers, nil

	case "goethe":
		doc := buildProjectGoetheMap(genAt, studioBlk, ident, projMeta, accessBlk, loreBlk, state, pIDs, allRepos, opts)
		return doc, filenameBase, headers, nil

	case "carnifex":
		doc := buildProjectCarnifexMap(genAt, studioBlk, ident, projMeta, accessBlk, loreBlk, state, allRepos, opts)
		return doc, filenameBase, headers, nil

	case "animafaber":
		doc := buildProjectTrackToolMap("animafaber", "spirit", 44200, genAt, studioBlk, ident, projMeta, accessBlk, loreBlk, state, pIDs, allRepos, opts)
		return doc, filenameBase, headers, nil

	case "fontpit":
		doc := buildProjectTrackToolMap("fontpit", "typography", 0, genAt, studioBlk, ident, projMeta, accessBlk, loreBlk, state, pIDs, allRepos, opts)
		return doc, filenameBase, headers, nil

	case "uri-handler":
		repoMap := make(map[string]map[string]any, len(allRepos))
		for _, r := range allRepos {
			repoMap[r.Slug] = map[string]any{
				"name":          r.Name,
				"system":        r.System,
				"remoteUri":     r.RemoteURI,
				"loreFullName":  r.LoreFullName,
				"workspacePath": r.WorkspacePath,
				"accessLevel":   r.AccessLevel,
			}
		}
		doc := map[string]any{
			"schemaVersion": ConfigSchemaVersion,
			"generatedAt":   genAt,
			"scope":         "project",
			"tool":          "uri-handler",
			"studio":        studioBlk,
			"subject":       ident,
			"project":       projMeta,
			"access":        accessBlk,
			"uriHandler": map[string]any{
				"schemes":           []string{"aetherion", "godot", "geppetto", "goethe", "lore", "lores", "carnifex", "animafaber", "fontpit"},
				"recommendedRoot":   "~/Projects/" + projMeta.Slug,
				"repositoryMapping": repoMap,
			},
		}
		return doc, filenameBase, headers, nil

	case "all":
		doc := map[string]any{
			"schemaVersion": ConfigSchemaVersion,
			"generatedAt":   genAt,
			"scope":         "project",
			"tool":          "all",
			"studio":        studioBlk,
			"subject":       ident,
			"project":       projMeta,
			"access":        accessBlk,
			"lore":          loreBlk,
			"repositories":  allRepos,
			"tools": map[string]any{
				"aetherion": buildProjectAetherionMap("aetherion", genAt, studioBlk, ident, projMeta, accessBlk, loreBlk, state, pIDs, allRepos, opts),
				"geppetto":  buildProjectGeppettoMap(genAt, studioBlk, ident, projMeta, accessBlk, loreBlk, state, pIDs, loreRepos, allRepos, opts),
				"lore":      buildProjectLoreMap(genAt, studioBlk, ident, projMeta, accessBlk, loreBlk, loreRepos, opts),
				"goethe":    buildProjectGoetheMap(genAt, studioBlk, ident, projMeta, accessBlk, loreBlk, state, pIDs, allRepos, opts),
				"carnifex":  buildProjectCarnifexMap(genAt, studioBlk, ident, projMeta, accessBlk, loreBlk, state, allRepos, opts),
			},
		}
		return doc, filenameBase, headers, nil
	}

	return nil, "", nil, errBadInput("Unsupported configuration tool profile")
}

func buildProjectAetherionMap(
	toolKey, genAt string,
	studioBlk StudioEndpointBlock,
	ident SubjectIdentity,
	projMeta ProjectConfigMeta,
	accessBlk ProjectAccessBlock,
	loreBlk LoreEndpointConfig,
	state map[string]any,
	participantIDs []string,
	allRepos []RepositorySummary,
	opts ConfigEnvOptions,
) map[string]any {
	oidc := buildOIDCApp(opts, "aetherion", 0)
	openTasks, assignedOpen := countTasks(state, participantIDs)
	primaryWS := inferPrimaryWorkspacePath(allRepos)
	tasks := collectWorkspaceTasks(state, participantIDs, "", "godot")
	artifacts := collectWorkspaceArtifacts(state, allRepos, "", "aetherion")
	if len(artifacts) == 0 {
		artifacts = collectWorkspaceArtifacts(state, allRepos, "", "godot")
	}

	return map[string]any{
		"schemaVersion": ConfigSchemaVersion,
		"generatedAt":   genAt,
		"scope":         "project",
		"tool":          toolKey,
		"studio":        studioBlk,
		"subject":       ident,
		"project":       projMeta,
		"access":        accessBlk,
		"authentik":     oidc,
		"lore":          loreBlk,
		"engine": map[string]any{
			"projectEngine":     projMeta.Engine,
			"usesAetherion":     projMeta.UsesAetherion,
			"engineRepository":  "git@github.com:RogueFairyStudios/aetherion.git",
			"httpsRepository":   "https://github.com/RogueFairyStudios/aetherion.git",
			"branch":            "aetherion",
			"upstreamGodotBase": "4.6.0-dev",
			"godotUri":          projMeta.GodotURI,
			"aetherionUri":      projMeta.AetherionURI,
		},
		"editorSettings": map[string]any{
			"network/authentik/issuer_url":      oidc.IssuerURL,
			"network/authentik/client_id":       oidc.ClientID,
			"network/authentik/scopes":          oidc.Scopes,
			"network/authentik/studio_api_url":  studioBlk.APIURL,
			"network/authentik/redirect_port":   0,
			"network/authentik/persist_session": true,
			"network/authentik/auto_refresh":    true,
		},
		"projectSettings": map[string]any{
			"application/config/name":      projMeta.Name,
			"network/authentik/issuer_url": oidc.IssuerURL,
			"network/authentik/client_id":  oidc.ClientID,
			"network/authentik/scopes":     oidc.Scopes,
		},
		"workspace": map[string]any{
			"recommendedRoot":       "~/Projects/" + projMeta.Slug,
			"primaryWorkspacePath":  primaryWS,
			"bootstrapCommands":     buildBootstrapCommands(loreBlk, allRepos),
			"openTaskCount":         openTasks,
			"assignedOpenTaskCount": assignedOpen,
		},
		"repositories": allRepos,
		"tasks":        tasks,
		"artifacts":    artifacts,
		"environment": map[string]string{
			"AETHERION_AUTHENTIK_ISSUER_URL": oidc.IssuerURL,
			"AETHERION_AUTHENTIK_CLIENT_ID":  oidc.ClientID,
			"AETHERION_AUTHENTIK_SCOPES":     oidc.Scopes,
			"AETHERION_STUDIO_API_URL":       studioBlk.APIURL,
			"STUDIO_API_URL":                 studioBlk.APIURL,
			"STUDIO_PROJECT_ID":              projMeta.ID,
		},
	}
}

func buildProjectGeppettoMap(
	genAt string,
	studioBlk StudioEndpointBlock,
	ident SubjectIdentity,
	projMeta ProjectConfigMeta,
	accessBlk ProjectAccessBlock,
	loreBlk LoreEndpointConfig,
	state map[string]any,
	participantIDs []string,
	loreRepos []RepositorySummary,
	allRepos []RepositorySummary,
	opts ConfigEnvOptions,
) map[string]any {
	oidc := buildOIDCApp(opts, "geppetto", 0)
	artArtifacts := collectWorkspaceArtifacts(state, allRepos, "art_2d", "geppetto")
	artTasks := collectWorkspaceTasks(state, participantIDs, "art_2d", "geppetto")
	sfSlots := collectStorefrontArtSlots(state, "geppetto", "art_2d")

	// Order Lore repositories first (primary for .kra / 2D art & animation), followed by any additional repositories.
	repos := make([]RepositorySummary, 0, len(allRepos))
	seen := make(map[string]bool, len(allRepos))
	for _, r := range loreRepos {
		repos = append(repos, r)
		seen[r.ID] = true
	}
	for _, r := range allRepos {
		if !seen[r.ID] {
			repos = append(repos, r)
		}
	}

	folders := deriveGeppettoArtFolders(repos, artArtifacts)

	return map[string]any{
		"schemaVersion": ConfigSchemaVersion,
		"generatedAt":   genAt,
		"scope":         "project",
		"tool":          "geppetto",
		"studio":        studioBlk,
		"subject":       ident,
		"project":       projMeta,
		"access":        accessBlk,
		"authentik":     oidc,
		"lore":          loreBlk,
		"kritaVersioningSettings": map[string]any{
			"configGroup":         "Versioning",
			"driverId":            "lore",
			"serverAddress":       loreBlk.RemoteBase,
			"authServerUrl":       loreBlk.BridgeURL,
			"authentikIssuerUrl":  oidc.IssuerURL,
			"authentikClientId":   oidc.ClientID,
			"projectsRoot":        "~/Documents/Geppetto",
			"neverSyncedPatterns": []string{"*.kra~", ".*-autosave.kra", "*-autosave.kra"},
			"projectEntry": map[string]any{
				"projectId":          projMeta.ID,
				"projectSlug":        projMeta.Slug,
				"projectName":        projMeta.Name,
				"driverId":           "lore",
				"serverAddress":      loreBlk.RemoteBase,
				"authServerUrl":      loreBlk.BridgeURL,
				"localWorkspaceRoot": "~/Documents/Geppetto/" + projMeta.Slug,
				"accessLevel":        loreAccessLevelLabel(accessBlk.Lore),
				"writable":           accessBlk.Lore.CanWrite,
				"canCreateRepo":      accessBlk.Lore.CanCreateRepo,
				"allowedActions":     accessBlk.Lore.Actions,
				"folders":            folders,
			},
		},
		"rogueStudioPlugin": map[string]any{
			"clientApp":          "geppetto",
			"apiUrl":             studioBlk.APIURL,
			"graphqlUrl":         studioBlk.GraphQLURL,
			"projectId":          projMeta.ID,
			"projectSlug":        projMeta.Slug,
			"workspacesEndpoint": studioBlk.APIURL + "/v1/me/workspaces?client=geppetto&vcs=lore",
		},
		"repositories": repos,
		"artPipeline": map[string]any{
			"geppettoUri":       projMeta.GeppettoURI,
			"bootstrapCommands": buildBootstrapCommands(loreBlk, repos),
			"artifacts":         artArtifacts,
			"tasks":             artTasks,
			"storefrontSlots":   sfSlots,
		},
		"environment": map[string]string{
			"STUDIO_API_URL":    studioBlk.APIURL,
			"STUDIO_URL":        studioBlk.GraphQLURL,
			"STUDIO_PROJECT_ID": projMeta.ID,
			"STUDIO_CLIENT_APP": "geppetto",
			"LORE_REMOTE_BASE":  loreBlk.RemoteBase,
			"LORE_AUTH_URL":     loreBlk.BridgeURL,
		},
	}
}

func buildProjectLoreMap(
	genAt string,
	studioBlk StudioEndpointBlock,
	ident SubjectIdentity,
	projMeta ProjectConfigMeta,
	accessBlk ProjectAccessBlock,
	loreBlk LoreEndpointConfig,
	loreRepos []RepositorySummary,
	opts ConfigEnvOptions,
) map[string]any {
	cliOIDC := buildOIDCApp(opts, "rfs-lore-cli", 0)
	repoConfigs := make([]map[string]any, 0, len(loreRepos))
	for _, r := range loreRepos {
		repoConfigs = append(repoConfigs, map[string]any{
			"id":            r.ID,
			"name":          r.Name,
			"slug":          r.Slug,
			"loreFullName":  r.LoreFullName,
			"remoteUri":     r.RemoteURI,
			"defaultBranch": r.DefaultBranch,
			"workspacePath": r.WorkspacePath,
			"accessLevel":   r.AccessLevel,
			"canRead":       r.CanRead,
			"canWrite":      r.CanWrite,
			"cloneCommand":  r.CloneCommand,
			"configToml":    fmt.Sprintf("remote_url = %q\n", r.RemoteURI),
			"lockCount":     r.LockCount,
			"locks":         r.Locks,
		})
	}

	loreSection := map[string]any{
		"remoteBase":      loreBlk.RemoteBase,
		"bridgeUrl":       loreBlk.BridgeURL,
		"authUrl":         loreBlk.AuthURL,
		"projectSlug":     projMeta.Slug,
		"namespace":       projMeta.Slug + "/*",
		"visibility":      accessBlk.Lore.Visibility,
		"accessLevel":     loreAccessLevelLabel(accessBlk.Lore),
		"allowedActions":  accessBlk.Lore.Actions,
		"canRead":         accessBlk.Lore.CanRead,
		"canWrite":        accessBlk.Lore.CanWrite,
		"canCreateRepo":   accessBlk.Lore.CanCreateRepo,
		"loginCommand":    loreBlk.LoginCommand,
		"casCompression":  "zstd",
		"bootstrapScript": buildBootstrapCommands(loreBlk, loreRepos),
	}
	if accessBlk.Lore.CanCreateRepo {
		loreSection["createRepoExample"] = []string{"lore", "repo", "create", loreBlk.RemoteBase + "/" + projMeta.Slug + "/art"}
	}

	return map[string]any{
		"schemaVersion": ConfigSchemaVersion,
		"generatedAt":   genAt,
		"scope":         "project",
		"tool":          "lore",
		"studio":        studioBlk,
		"subject":       ident,
		"project":       projMeta,
		"access":        accessBlk,
		"authentik":     cliOIDC,
		"lore":          loreSection,
		"repositories":  repoConfigs,
		"environment": map[string]string{
			"LORE_REMOTE_BASE":  loreBlk.RemoteBase,
			"LORE_AUTH_URL":     loreBlk.BridgeURL,
			"STUDIO_API_URL":    studioBlk.APIURL,
			"STUDIO_PROJECT_ID": projMeta.ID,
		},
	}
}

func buildProjectGoetheMap(
	genAt string,
	studioBlk StudioEndpointBlock,
	ident SubjectIdentity,
	projMeta ProjectConfigMeta,
	accessBlk ProjectAccessBlock,
	loreBlk LoreEndpointConfig,
	state map[string]any,
	participantIDs []string,
	allRepos []RepositorySummary,
	opts ConfigEnvOptions,
) map[string]any {
	oidc := buildOIDCApp(opts, "goethe", 0)
	artifacts := collectWorkspaceArtifacts(state, allRepos, "narrative", "goethe")
	tasks := collectWorkspaceTasks(state, participantIDs, "narrative", "goethe")

	return map[string]any{
		"schemaVersion": ConfigSchemaVersion,
		"generatedAt":   genAt,
		"scope":         "project",
		"tool":          "goethe",
		"studio":        studioBlk,
		"subject":       ident,
		"project":       projMeta,
		"access":        accessBlk,
		"authentik":     oidc,
		"lore":          loreBlk,
		"gproj": map[string]any{
			"filename":       projMeta.Slug + ".gproj.yaml",
			"name":           projMeta.Name,
			"version":        "0.1.0",
			"author":         "Rogue Fairy Studios",
			"default_locale": "en",
			"dialogues_dir":  "dialogues",
			"portraits_dir":  "portraits",
			"locales_dir":    "locales",
		},
		"gdkgBuild": map[string]any{
			"outputArchive": "build/" + projMeta.Slug + ".gdkg",
			"compression":   "zstd",
			"schemaVersion": 1,
		},
		"repositories": allRepos,
		"artifacts":    artifacts,
		"tasks":        tasks,
		"environment": map[string]string{
			"STUDIO_API_URL":    studioBlk.APIURL,
			"STUDIO_URL":        studioBlk.GraphQLURL,
			"STUDIO_PROJECT_ID": projMeta.ID,
			"STUDIO_CLIENT_APP": "goethe",
		},
	}
}

func buildProjectCarnifexMap(
	genAt string,
	studioBlk StudioEndpointBlock,
	ident SubjectIdentity,
	projMeta ProjectConfigMeta,
	accessBlk ProjectAccessBlock,
	loreBlk LoreEndpointConfig,
	state map[string]any,
	allRepos []RepositorySummary,
	opts ConfigEnvOptions,
) map[string]any {
	oidc := buildOIDCApp(opts, "carnifex", 0)
	releases := make([]map[string]any, 0)
	for _, r := range sliceOfMaps(state["releases"]) {
		releases = append(releases, map[string]any{
			"id":          strVal(r["id"]),
			"name":        strVal(r["name"]),
			"publishedAt": r["publishedAt"],
		})
	}
	var latestApprovedCredits map[string]any
	for _, c := range sliceOfMaps(state["credits"]) {
		if strVal(c["status"]) == "approved" {
			latestApprovedCredits = map[string]any{
				"snapshotId": strVal(c["id"]),
				"releaseId":  strVal(c["releaseId"]),
				"checksum":   strVal(c["checksum"]),
			}
		}
	}
	storefronts := make([]map[string]any, 0)
	for _, t := range sliceOfMaps(state["platformTargets"]) {
		if boolVal(t["archived"]) {
			continue
		}
		storefronts = append(storefronts, map[string]any{
			"id":            strVal(t["id"]),
			"name":          strVal(t["name"]),
			"storefront":    strVal(t["storefront"]),
			"appIdentifier": strVal(t["appIdentifier"]),
			"status":        strVal(t["status"]),
		})
	}

	return map[string]any{
		"schemaVersion": ConfigSchemaVersion,
		"generatedAt":   genAt,
		"scope":         "project",
		"tool":          "carnifex",
		"studio":        studioBlk,
		"subject":       ident,
		"project":       projMeta,
		"access":        accessBlk,
		"authentik":     oidc,
		"lore":          loreBlk,
		"carnifex": map[string]any{
			"projectEngine":         projMeta.Engine,
			"maxUploadMB":           opts.MaxUploadMB,
			"latestApprovedCredits": latestApprovedCredits,
			"releases":              releases,
			"storefronts":           storefronts,
		},
		"repositories": allRepos,
		"environment": map[string]string{
			"STUDIO_API_URL":    studioBlk.APIURL,
			"STUDIO_URL":        studioBlk.GraphQLURL,
			"STUDIO_PROJECT_ID": projMeta.ID,
			"STUDIO_CLIENT_APP": "carnifex",
		},
	}
}

func buildProjectTrackToolMap(
	toolKey, trackKey string,
	redirectPort int,
	genAt string,
	studioBlk StudioEndpointBlock,
	ident SubjectIdentity,
	projMeta ProjectConfigMeta,
	accessBlk ProjectAccessBlock,
	loreBlk LoreEndpointConfig,
	state map[string]any,
	participantIDs []string,
	allRepos []RepositorySummary,
	opts ConfigEnvOptions,
) map[string]any {
	oidc := buildOIDCApp(opts, toolKey, redirectPort)
	artifacts := collectWorkspaceArtifacts(state, allRepos, trackKey, toolKey)
	tasks := collectWorkspaceTasks(state, participantIDs, trackKey, toolKey)

	return map[string]any{
		"schemaVersion": ConfigSchemaVersion,
		"generatedAt":   genAt,
		"scope":         "project",
		"tool":          toolKey,
		"studio":        studioBlk,
		"subject":       ident,
		"project":       projMeta,
		"access":        accessBlk,
		"authentik":     oidc,
		"lore":          loreBlk,
		"repositories":  allRepos,
		"artifacts":     artifacts,
		"tasks":         tasks,
		"environment": map[string]string{
			"STUDIO_API_URL":    studioBlk.APIURL,
			"STUDIO_URL":        studioBlk.GraphQLURL,
			"STUDIO_PROJECT_ID": projMeta.ID,
			"STUDIO_CLIENT_APP": toolKey,
		},
	}
}

