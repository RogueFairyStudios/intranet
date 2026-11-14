package domain

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/roguefairystudios/intranet/studio-server/internal/auth"
	"github.com/roguefairystudios/intranet/studio-server/internal/store"
)

// PolicyConfig defines the Tellurian Authentik group names governing Studio & Lore access.
type PolicyConfig struct {
	AdminGroups    []string
	EmployeeGroups []string
	PartnerGroups  []string
}

// DefaultPolicy returns the canonical Rogue Fairy Studios Tellurian Authentik group policy.
func DefaultPolicy() PolicyConfig {
	return PolicyConfig{
		AdminGroups:    []string{GroupCLevel, GroupAdmins},
		EmployeeGroups: []string{GroupEmployees},
		PartnerGroups:  []string{GroupPartners},
	}
}

func (p PolicyConfig) withDefaults() PolicyConfig {
	def := DefaultPolicy()
	if len(p.AdminGroups) == 0 {
		p.AdminGroups = def.AdminGroups
	}
	if len(p.EmployeeGroups) == 0 {
		p.EmployeeGroups = def.EmployeeGroups
	}
	if len(p.PartnerGroups) == 0 {
		p.PartnerGroups = def.PartnerGroups
	}
	return p
}

// HasAdminGroup reports whether any group in names matches an configured admin group.
func (p PolicyConfig) HasAdminGroup(names []string) bool {
	p = p.withDefaults()
	return hasAnyGroupFold(names, p.AdminGroups)
}

// HasEmployeeGroup reports whether any group in names matches an employee group.
func (p PolicyConfig) HasEmployeeGroup(names []string) bool {
	p = p.withDefaults()
	return hasAnyGroupFold(names, p.EmployeeGroups)
}

// HasPartnerGroup reports whether any group in names matches a partner group (fail-closed isolation).
func (p PolicyConfig) HasPartnerGroup(names []string) bool {
	p = p.withDefaults()
	return hasAnyGroupFold(names, p.PartnerGroups)
}

func hasAnyGroupFold(have, targets []string) bool {
	for _, h := range have {
		ht := strings.TrimSpace(h)
		if ht == "" {
			continue
		}
		for _, t := range targets {
			if strings.EqualFold(ht, strings.TrimSpace(t)) {
				return true
			}
		}
	}
	return false
}

// SubjectProfile holds the resolved identity, Wiki.js account mapping, and Authentik group memberships
// for either the calling Principal or a queried target user (e.g. User X / User Y).
type SubjectProfile struct {
	Subject     string                  `json:"sub"`
	WikiUserID  int                     `json:"wikiUserId,omitempty"`
	WikiAPIID   int                     `json:"wikiApiId,omitempty"`
	Username    string                  `json:"username,omitempty"`
	Name        string                  `json:"name,omitempty"`
	Email       string                  `json:"email,omitempty"`
	GroupIDs    []int                   `json:"groupIds,omitempty"`
	GroupNames  []string                `json:"groups,omitempty"`
	Permissions []string                `json:"permissions,omitempty"`
	LoreGrants  []auth.LoreProjectClaim `json:"loreGrants,omitempty"`
	ClientApp   string                  `json:"clientApp,omitempty"`
	ServiceKey  bool                    `json:"serviceKey,omitempty"`
	ServiceRole string                  `json:"serviceRole,omitempty"`
	Delegated   bool                    `json:"delegated,omitempty"`
	IsAdmin     bool                    `json:"isAdmin"`
}

// ActorID returns the canonical audit actor value stored in project history.
func (s SubjectProfile) ActorID() any {
	if s.WikiAPIID > 0 {
		return fmt.Sprintf("api:%d", s.WikiAPIID)
	}
	if s.ServiceKey {
		if s.Name != "" {
			return "key:" + s.Name
		}
		return s.Subject
	}
	if s.WikiUserID > 0 {
		return s.WikiUserID
	}
	if s.Email != "" {
		return s.Email
	}
	return s.Subject
}

// ResolvePrincipalProfile enriches an authenticated Principal with Wiki.js user & group mappings when available.
func ResolvePrincipalProfile(ctx context.Context, st store.Store, policy PolicyConfig, p auth.Principal) (SubjectProfile, error) {
	policy = policy.withDefaults()
	prof := SubjectProfile{
		Subject:     p.Subject,
		WikiUserID:  p.WikiUserID,
		WikiAPIID:   p.WikiAPIID,
		Username:    p.Username,
		Name:        p.Name,
		Email:       p.Email,
		GroupIDs:    slices.Clone(p.GroupIDs),
		GroupNames:  slices.Clone(p.Groups),
		Permissions: slices.Clone(p.Permissions),
		LoreGrants:  slices.Clone(p.LoreGrants),
		ClientApp:   p.ClientApp,
		ServiceKey:  p.ServiceKey,
		ServiceRole: p.ServiceRole,
		Delegated:   p.Delegated,
		IsAdmin:     p.IsAdmin,
	}

	if st != nil && !prof.ServiceKey && prof.WikiAPIID == 0 {
		wu, err := st.ResolveWikiUser(ctx, prof.Subject, prof.Email, prof.Username, prof.WikiUserID)
		if err != nil {
			return SubjectProfile{}, err
		}
		if wu != nil {
			if prof.WikiUserID == 0 {
				prof.WikiUserID = wu.ID
			}
			if prof.Email == "" {
				prof.Email = wu.Email
			}
			if prof.Name == "" {
				prof.Name = wu.Name
			}
			if prof.Username == "" && wu.Email != "" {
				prof.Username = strings.Split(wu.Email, "@")[0]
			}
			mergeWikiGroups(&prof, wu.Groups)
		}
	}

	if st != nil {
		enrichGroupIDsByName(ctx, st, &prof)
	}

	if slices.Contains(prof.Permissions, "manage:system") || (!prof.ServiceKey && prof.WikiAPIID == 0 && policy.HasAdminGroup(prof.GroupNames)) {
		prof.IsAdmin = true
	}
	return prof, nil
}

// ResolveTargetUser resolves an arbitrary user identifier (Wiki ID, OIDC sub, email, or username)
// across the shared Wiki.js database and optional Tellurian Authentik Core API.
func ResolveTargetUser(ctx context.Context, st store.Store, dir auth.DirectoryClient, policy PolicyConfig, identifier string) (SubjectProfile, error) {
	policy = policy.withDefaults()
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return SubjectProfile{}, errBadInput("Missing user identifier")
	}

	var numericID int
	if n, err := strconv.Atoi(identifier); err == nil && n > 0 {
		if n == 2 {
			return SubjectProfile{}, store.ErrNotFound
		}
		numericID = n
	}

	var wu *store.WikiUser
	if st != nil {
		var err error
		wu, err = st.ResolveWikiUser(ctx, identifier, identifier, identifier, numericID)
		if err != nil {
			return SubjectProfile{}, err
		}
	}

	var au *auth.AuthentikUser
	if dir != nil {
		lookupKey := identifier
		if wu != nil && wu.Email != "" && numericID > 0 {
			lookupKey = wu.Email
		}
		if found, err := dir.LookupUser(ctx, lookupKey); err == nil && found != nil {
			au = found
		}
	}

	if wu == nil && au != nil && st != nil {
		wu, _ = st.ResolveWikiUser(ctx, au.UUID, au.Email, au.Username, 0)
	}

	if wu == nil && au == nil {
		return SubjectProfile{}, store.ErrNotFound
	}

	var prof SubjectProfile
	if wu != nil {
		prof.WikiUserID = wu.ID
		prof.Subject = wu.ProviderID
		if prof.Subject == "" {
			prof.Subject = fmt.Sprintf("wiki:%d", wu.ID)
		}
		prof.Email = wu.Email
		prof.Name = wu.Name
		if wu.Email != "" {
			prof.Username = strings.Split(wu.Email, "@")[0]
		}
		mergeWikiGroups(&prof, wu.Groups)
	}
	if au != nil {
		if au.UUID != "" && (prof.Subject == "" || strings.HasPrefix(prof.Subject, "wiki:")) {
			prof.Subject = au.UUID
		}
		if au.Username != "" {
			prof.Username = au.Username
		}
		if prof.Name == "" {
			prof.Name = au.Name
		}
		if prof.Email == "" {
			prof.Email = au.Email
		}
		for _, g := range au.Groups {
			addUniqueString(&prof.GroupNames, g)
		}
	}

	if st != nil {
		enrichGroupIDsByName(ctx, st, &prof)
	}

	if slices.Contains(prof.Permissions, "manage:system") || policy.HasAdminGroup(prof.GroupNames) {
		prof.IsAdmin = true
	}
	return prof, nil
}

func mergeWikiGroups(prof *SubjectProfile, groups []store.WikiGroup) {
	for _, g := range groups {
		if g.ID > 0 && !slices.Contains(prof.GroupIDs, g.ID) {
			prof.GroupIDs = append(prof.GroupIDs, g.ID)
		}
		addUniqueString(&prof.GroupNames, g.Name)
		for _, perm := range g.Permissions {
			addUniqueString(&prof.Permissions, perm)
		}
	}
}

func enrichGroupIDsByName(ctx context.Context, st store.Store, prof *SubjectProfile) {
	for _, gName := range prof.GroupNames {
		if g, err := st.GetWikiGroupByName(ctx, gName); err == nil && g != nil {
			if g.ID > 0 && !slices.Contains(prof.GroupIDs, g.ID) {
				prof.GroupIDs = append(prof.GroupIDs, g.ID)
			}
			for _, perm := range g.Permissions {
				addUniqueString(&prof.Permissions, perm)
			}
		}
	}
}

func addUniqueString(slice *[]string, v string) {
	v = strings.TrimSpace(v)
	if v == "" {
		return
	}
	for _, existing := range *slice {
		if strings.EqualFold(existing, v) {
			return
		}
	}
	*slice = append(*slice, v)
}

var roleRank = map[string]int{
	"reader":  1,
	"editor":  2,
	"manager": 3,
}

func pickHigherRole(cur, candidate string) string {
	if roleRank[candidate] == 0 {
		return cur
	}
	if roleRank[candidate] > roleRank[cur] {
		return candidate
	}
	return cur
}

func capRole(role, maxRole string) string {
	if role == "" || maxRole == "" {
		return role
	}
	if roleRank[maxRole] > 0 && roleRank[role] > roleRank[maxRole] {
		return maxRole
	}
	return role
}

func isProjectOwner(state map[string]any, subj SubjectProfile) bool {
	owner := state["ownerId"]
	if owner == nil {
		return false
	}
	if ownerInt, ok := intVal(owner); ok && ownerInt > 0 && subj.WikiUserID == ownerInt {
		return true
	}
	ownerStr := strings.TrimSpace(strVal(owner))
	if ownerStr == "" {
		return false
	}
	if subj.Subject != "" && strings.EqualFold(ownerStr, subj.Subject) {
		return true
	}
	if subj.Email != "" && strings.EqualFold(ownerStr, subj.Email) {
		return true
	}
	if subj.WikiAPIID > 0 && ownerStr == fmt.Sprintf("api:%d", subj.WikiAPIID) {
		return true
	}
	return false
}

// RoleFor computes a subject's Studio role ("manager", "editor", "reader", or "") on state,
// maintaining strict parity with Wiki.js studio-access.js and fail-closed Partner isolation.
func RoleFor(state map[string]any, subj SubjectProfile, policy PolicyConfig) string {
	policy = policy.withDefaults()
	UpgradeState(state)

	// 1. Service Key callers (X-Studio-Key / STUDIO_SERVICE_KEYS)
	if subj.ServiceKey {
		if subj.IsAdmin || subj.ServiceRole == "admin" {
			return "manager"
		}
		if len(subj.GroupIDs) == 0 && len(subj.GroupNames) == 0 {
			if roleRank[subj.ServiceRole] > 0 {
				return subj.ServiceRole
			}
			return ""
		}
		var keyRole string
		for _, entry := range sliceOfMaps(state["integrationAccess"]) {
			if gid, ok := intVal(entry["groupId"]); ok && slices.Contains(subj.GroupIDs, gid) {
				keyRole = pickHigherRole(keyRole, strVal(entry["role"]))
			}
			if gName := strVal(entry["groupName"]); gName != "" && hasAnyGroupFold(subj.GroupNames, []string{gName}) {
				keyRole = pickHigherRole(keyRole, strVal(entry["role"]))
			}
		}
		for _, entry := range sliceOfMaps(state["groupAccess"]) {
			if gid, ok := intVal(entry["groupId"]); ok && gid > 0 && slices.Contains(subj.GroupIDs, gid) {
				keyRole = pickHigherRole(keyRole, strVal(entry["role"]))
			}
			if gName := strVal(entry["groupName"]); gName != "" && hasAnyGroupFold(subj.GroupNames, []string{gName}) {
				keyRole = pickHigherRole(keyRole, strVal(entry["role"]))
			}
		}
		return capRole(keyRole, subj.ServiceRole)
	}

	// 2. Wiki.js API keys (delegated with apiId)
	if subj.WikiAPIID > 0 {
		if subj.IsAdmin || slices.Contains(subj.Permissions, "manage:system") {
			return "manager"
		}
		var apiRole string
		for _, entry := range sliceOfMaps(state["integrationAccess"]) {
			if gid, ok := intVal(entry["groupId"]); ok && slices.Contains(subj.GroupIDs, gid) {
				apiRole = pickHigherRole(apiRole, strVal(entry["role"]))
			}
		}
		return apiRole
	}

	// 3. Human / OIDC accounts
	if isProjectOwner(state, subj) ||
		subj.IsAdmin ||
		slices.Contains(subj.Permissions, "manage:system") ||
		policy.HasAdminGroup(subj.GroupNames) {
		return "manager"
	}

	var role string
	for _, entry := range sliceOfMaps(state["access"]) {
		if uid, ok := intVal(entry["userId"]); ok && uid > 0 && subj.WikiUserID == uid {
			role = pickHigherRole(role, strVal(entry["role"]))
			continue
		}
		if eSub := strVal(entry["sub"]); eSub != "" && subj.Subject != "" && strings.EqualFold(eSub, subj.Subject) {
			role = pickHigherRole(role, strVal(entry["role"]))
			continue
		}
		if eMail := strVal(entry["email"]); eMail != "" && subj.Email != "" && strings.EqualFold(eMail, subj.Email) {
			role = pickHigherRole(role, strVal(entry["role"]))
		}
	}

	for _, entry := range sliceOfMaps(state["groupAccess"]) {
		gid, hasGID := intVal(entry["groupId"])
		gName := strVal(entry["groupName"])
		if (hasGID && gid > 0 && slices.Contains(subj.GroupIDs, gid)) ||
			(gName != "" && hasAnyGroupFold(subj.GroupNames, []string{gName})) {
			role = pickHigherRole(role, strVal(entry["role"]))
		}
	}

	loreMap, _ := state["lore"].(map[string]any)
	visibility := "internal"
	if strVal(loreMap["visibility"]) == "restricted" {
		visibility = "restricted"
	}
	empRole := strVal(loreMap["employeeRole"])
	isPartner := policy.HasPartnerGroup(subj.GroupNames)
	isEmployee := policy.HasEmployeeGroup(subj.GroupNames)

	if visibility == "internal" && !isPartner && isEmployee && (empRole == "reader" || empRole == "editor") {
		role = pickHigherRole(role, empRole)
	}
	return role
}

// EffectiveLoreGrant summarizes a subject's Lore VCS access on a project (<projectSlug>/*).
type EffectiveLoreGrant struct {
	ProjectSlug   string   `json:"projectSlug"`
	Visibility    string   `json:"visibility"`
	RemoteBase    string   `json:"remoteBase"`
	BridgeURL     string   `json:"bridgeUrl"`
	AuthURL       string   `json:"authUrl"`
	Actions       []string `json:"actions"`
	CanRead       bool     `json:"canRead"`
	CanWrite      bool     `json:"canWrite"`
	CanCreateRepo bool     `json:"canCreateRepo"`
}

// EffectiveLoreGrantFor computes the subject's effective Lore VCS permissions on state,
// matching Tellurian Authentik's Rogue Fairy Lore Scope & rfs-lore-bridge policy.
func EffectiveLoreGrantFor(state map[string]any, subj SubjectProfile, policy PolicyConfig) EffectiveLoreGrant {
	policy = policy.withDefaults()
	UpgradeState(state)

	loreMap, _ := state["lore"].(map[string]any)
	projSlug := strVal(loreMap["projectSlug"])
	vis := "internal"
	if strVal(loreMap["visibility"]) == "restricted" {
		vis = "restricted"
	}

	out := EffectiveLoreGrant{
		ProjectSlug: projSlug,
		Visibility:  vis,
		RemoteBase:  strVal(loreMap["remoteBase"]),
		BridgeURL:   strVal(loreMap["bridgeUrl"]),
		AuthURL:     strVal(loreMap["authUrl"]),
		Actions:     []string{},
	}

	actionSet := make(map[string]bool)
	addActions := func(acts []string) {
		for _, a := range acts {
			if slices.Contains(LoreActions, a) {
				actionSet[a] = true
			}
		}
	}

	isPartner := policy.HasPartnerGroup(subj.GroupNames)
	isEmployee := policy.HasEmployeeGroup(subj.GroupNames)
	isAdmin := subj.IsAdmin || slices.Contains(subj.Permissions, "manage:system") || (!subj.ServiceKey && subj.WikiAPIID == 0 && policy.HasAdminGroup(subj.GroupNames))

	if isAdmin || isProjectOwner(state, subj) {
		addActions(LoreActions)
		out.CanCreateRepo = true
	}

	if subj.ServiceKey {
		role := RoleFor(state, subj, policy)
		acts, _ := NormalizeLoreActions(nil, false, role)
		addActions(acts)
		if role == "manager" {
			out.CanCreateRepo = true
		}
	}

	var hasExplicitGrant bool
	for _, entry := range sliceOfMaps(state["access"]) {
		matched := false
		if uid, ok := intVal(entry["userId"]); ok && uid > 0 && subj.WikiUserID == uid {
			matched = true
		} else if eSub := strVal(entry["sub"]); eSub != "" && subj.Subject != "" && strings.EqualFold(eSub, subj.Subject) {
			matched = true
		} else if eMail := strVal(entry["email"]); eMail != "" && subj.Email != "" && strings.EqualFold(eMail, subj.Email) {
			matched = true
		}
		if matched {
			hasExplicitGrant = true
			acts, _ := NormalizeLoreActions(nil, false, strVal(entry["role"]))
			addActions(acts)
		}
	}

	for _, entry := range sliceOfMaps(state["groupAccess"]) {
		gid, hasGID := intVal(entry["groupId"])
		gName := strVal(entry["groupName"])
		if (hasGID && gid > 0 && slices.Contains(subj.GroupIDs, gid)) ||
			(gName != "" && hasAnyGroupFold(subj.GroupNames, []string{gName})) {
			hasExplicitGrant = true
			if acts, ok := strSlice(entry["loreActions"]); ok {
				addActions(acts)
			} else {
				derived, _ := NormalizeLoreActions(nil, false, strVal(entry["role"]))
				addActions(derived)
			}
			if boolVal(entry["loreCanCreateRepo"]) {
				out.CanCreateRepo = true
			}
		}
	}

	// Employee baseline on internal projects (Partners strictly excluded fail-closed).
	if vis == "internal" && !isPartner && isEmployee {
		addActions([]string{"read", "write"})
	}

	// Explicit rfs_lore token claims emitted by Tellurian Authentik.
	for _, claim := range subj.LoreGrants {
		if !strings.EqualFold(claim.Project, projSlug) {
			continue
		}
		if isPartner && vis != "restricted" && !hasExplicitGrant && !isAdmin {
			continue
		}
		addActions(claim.Actions)
		if claim.CanCreateRepo {
			out.CanCreateRepo = true
		}
	}

	for _, canonical := range LoreActions {
		if actionSet[canonical] {
			out.Actions = append(out.Actions, canonical)
		}
	}
	out.CanRead = actionSet["read"] || actionSet["write"] || actionSet["admin"] || actionSet["owner"]
	out.CanWrite = actionSet["write"] || actionSet["admin"] || actionSet["owner"]
	return out
}

// PresentedProject is the serialized representation returned to Wiki.js and REST callers.
type PresentedProject struct {
	ID        string         `json:"id"`
	Revision  int            `json:"revision"`
	State     string         `json:"state"`
	StateObj  map[string]any `json:"project,omitempty"`
	Role      string         `json:"role"`
	CanManage bool           `json:"canManage"`
	CanEdit   bool           `json:"canEdit"`
}

// PresentProject upgrades, filters sensitive ACLs for non-managers, and enriches user/group metadata for managers.
func PresentProject(ctx context.Context, st store.Store, row store.ProjectRow, subj SubjectProfile, policy PolicyConfig) (PresentedProject, error) {
	state, err := ParseState(row.State)
	if err != nil {
		return PresentedProject{}, err
	}
	role := RoleFor(state, subj, policy)
	if role == "" {
		return PresentedProject{}, store.ErrNotFound
	}

	if role != "manager" {
		state["access"] = []any{}
		state["groupAccess"] = []any{}
		state["integrationAccess"] = []any{}
	} else {
		for _, entry := range sliceOfMaps(state["access"]) {
			if st != nil {
				if uid, ok := intVal(entry["userId"]); ok && uid > 0 {
					if account, err := st.GetWikiUserByID(ctx, uid); err == nil && account != nil && account.Name != "" {
						entry["name"] = account.Name
					}
				}
			}
		}
		for _, entry := range sliceOfMaps(state["groupAccess"]) {
			if st != nil {
				if gid, ok := intVal(entry["groupId"]); ok && gid > 0 {
					if g, err := st.GetWikiGroupByID(ctx, gid); err == nil && g != nil && g.Name != "" {
						entry["name"] = g.Name
						entry["groupName"] = g.Name
					}
				}
			}
			if gName := strVal(entry["groupName"]); gName != "" && strVal(entry["name"]) == "" {
				entry["name"] = gName
			}
			entry["authentikAttributes"] = ExportAuthentikAttributes(state, entry)
		}
		for _, entry := range sliceOfMaps(state["integrationAccess"]) {
			if st != nil {
				if gid, ok := intVal(entry["groupId"]); ok && gid > 0 {
					if g, err := st.GetWikiGroupByID(ctx, gid); err == nil && g != nil && g.Name != "" {
						entry["name"] = g.Name
					}
				}
			}
		}
		state["loreBlueprint"] = ExportAuthentikBlueprint(state)
	}

	encoded, err := MarshalState(state)
	if err != nil {
		return PresentedProject{}, err
	}
	return PresentedProject{
		ID:        row.ID,
		Revision:  row.Revision,
		State:     encoded,
		StateObj:  state,
		Role:      role,
		CanManage: role == "manager",
		CanEdit:   role == "manager" || role == "editor",
	}, nil
}
