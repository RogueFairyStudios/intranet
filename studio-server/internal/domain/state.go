package domain

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"
)

// ParseState parses a JSON project state and upgrades it in-place to the current schema.
func ParseState(raw string) (map[string]any, error) {
	var state map[string]any
	if err := json.Unmarshal([]byte(raw), &state); err != nil || state == nil {
		return nil, fmt.Errorf("invalid project state JSON")
	}
	return UpgradeState(state), nil
}

// MarshalState serializes a project state back to compact JSON.
func MarshalState(state map[string]any) (string, error) {
	b, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CreateState builds a fresh Studio project state aggregate matching Wiki.js createState.
func CreateState(name, kind string, ownerID any) (map[string]any, error) {
	if !slices.Contains([]string{"digital", "analogue", "hybrid"}, kind) {
		return nil, errBadInput("Invalid project type")
	}
	cleanName, err := SafeText(name, 500)
	if err != nil {
		return nil, err
	}
	state := map[string]any{
		"projectId":       NewUUID(),
		"name":            cleanName,
		"kind":            kind,
		"engine":          DefaultEngineForKind(kind),
		"phase":           "pre_production",
		"ownerId":         ownerID,
		"access":          []any{},
		"groupAccess":     []any{},
		"participants":    []any{},
		"tasks":           []any{},
		"documents":       []any{},
		"releases":        []any{},
		"credits":         []any{},
		"builds":          []any{},
		"repositories":    []any{},
		"platformTargets": []any{},
		"history":         []any{},
	}
	return UpgradeState(state), nil
}

// UpgradeState ensures all expected collections and defaults exist without losing extra fields.
func UpgradeState(state map[string]any) map[string]any {
	if state == nil {
		state = map[string]any{}
	}
	for _, col := range Collections {
		if _, ok := state[col].([]any); !ok {
			state[col] = []any{}
		}
	}
	if !slices.Contains(Phases, strVal(state["phase"])) {
		state["phase"] = "pre_production"
	}
	if !slices.Contains(Engines, strVal(state["engine"])) {
		state["engine"] = DefaultEngineForKind(strVal(state["kind"]))
	}

	curLore, _ := state["lore"].(map[string]any)
	if curLore == nil {
		curLore = map[string]any{}
	}
	projSlug := strVal(curLore["projectSlug"])
	if !IsValidLoreProjectSlug(projSlug) {
		projSlug = DeriveLoreProjectSlug(strVal(state["name"]))
	}
	vis := "internal"
	if strVal(curLore["visibility"]) == "restricted" {
		vis = "restricted"
	}
	empRole := "none"
	if vis != "restricted" && slices.Contains(LoreEmployeeRoles, strVal(curLore["employeeRole"])) {
		empRole = strVal(curLore["employeeRole"])
	}
	remoteBase := strVal(curLore["remoteBase"])
	if remoteBase == "" {
		remoteBase = DefaultLoreRemoteBase
	}
	bridgeURL := strVal(curLore["bridgeUrl"])
	if bridgeURL == "" {
		bridgeURL = DefaultLoreBridgeURL
	}
	authURL := strVal(curLore["authUrl"])
	if authURL == "" {
		authURL = DefaultLoreAuthURL
	}
	state["lore"] = map[string]any{
		"projectSlug":  projSlug,
		"visibility":   vis,
		"employeeRole": empRole,
		"remoteBase":   remoteBase,
		"bridgeUrl":    bridgeURL,
		"authUrl":      authURL,
	}

	for _, entry := range sliceOfMaps(state["groupAccess"]) {
		rawActions, hasSlice := strSlice(entry["loreActions"])
		if !hasSlice {
			acts, _ := NormalizeLoreActions(nil, false, strVal(entry["role"]))
			entry["loreActions"] = toAnyStrings(acts)
		} else {
			acts, _ := NormalizeLoreActions(rawActions, true, "")
			entry["loreActions"] = toAnyStrings(acts)
		}
		entry["loreCanCreateRepo"] = boolVal(entry["loreCanCreateRepo"])
	}

	for _, repo := range sliceOfMaps(state["repositories"]) {
		if _, exists := repo["loreProject"]; !exists {
			repo["loreProject"] = ""
		}
		if _, exists := repo["loreRepo"]; !exists {
			repo["loreRepo"] = ""
		}
		if _, exists := repo["loreFullName"]; !exists {
			lp, lr := strVal(repo["loreProject"]), strVal(repo["loreRepo"])
			if lp != "" && lr != "" {
				repo["loreFullName"] = lp + "/" + lr
			} else {
				repo["loreFullName"] = ""
			}
		}
		if strVal(repo["system"]) == "lore" && strVal(repo["loreFullName"]) == "" && strVal(repo["remoteUri"]) != "" {
			if u, err := url.Parse(strVal(repo["remoteUri"])); err == nil {
				pPath := strings.Trim(u.Path, "/")
				if strings.Contains(pPath, "/") {
					if p, r, full, pErr := ParseLoreFullName(pPath); pErr == nil {
						repo["loreProject"] = p
						repo["loreRepo"] = r
						repo["loreFullName"] = full
					}
				}
			}
		}
	}

	for _, art := range sliceOfMaps(state["artifacts"]) {
		if !slices.Contains(Stages, strVal(art["stage"])) {
			art["stage"] = "concept"
		}
		if !slices.Contains(Tracks, strVal(art["track"])) {
			art["track"] = "general"
		}
		if !slices.Contains(Tools, strVal(art["tool"])) {
			src := strVal(art["source"])
			if slices.Contains(Tools, src) {
				art["tool"] = src
			} else {
				art["tool"] = "generic"
			}
		}
		if _, ok := art["toolUri"]; !ok {
			art["toolUri"] = nil
		}
		if _, ok := art["repoId"]; !ok {
			art["repoId"] = nil
		}
		if _, ok := art["assetPath"]; !ok {
			art["assetPath"] = ""
		}
	}

	for _, target := range sliceOfMaps(state["platformTargets"]) {
		if _, ok := target["checklist"].([]any); !ok {
			target["checklist"] = []any{}
		}
		if _, ok := target["assets"].([]any); !ok {
			target["assets"] = []any{}
		}
	}

	return state
}

// ExportAuthentikAttributes returns the native Tellurian Authentik group JSON attributes for a groupAccess entry.
func ExportAuthentikAttributes(state, entry map[string]any) map[string]any {
	UpgradeState(state)
	loreMap, _ := state["lore"].(map[string]any)
	rawActions, _ := strSlice(entry["loreActions"])
	ordered := make([]string, 0, len(rawActions))
	for _, a := range LoreActions {
		if slices.Contains(rawActions, a) {
			ordered = append(ordered, a)
		}
	}
	return map[string]any{
		"lore_project":         strVal(loreMap["projectSlug"]),
		"lore_visibility":      strVal(loreMap["visibility"]),
		"lore_actions":         ordered,
		"lore_can_create_repo": boolVal(entry["loreCanCreateRepo"]),
	}
}

func yamlScalar(value string) string {
	lower := strings.ToLower(value)
	reserved := lower == "true" || lower == "false" || lower == "null" || lower == "yes" || lower == "no" || lower == "on" || lower == "off"
	if value != "" && reYAMLSafeScalar.MatchString(value) && !reserved {
		return value
	}
	b, _ := json.Marshal(value)
	return string(b)
}

// ExportAuthentikBlueprint renders the Tellurian Authentik v1 Blueprint YAML for project Lore groups.
func ExportAuthentikBlueprint(state map[string]any) string {
	UpgradeState(state)
	loreMap, _ := state["lore"].(map[string]any)
	projName := strVal(state["name"])
	projSlug := strVal(loreMap["projectSlug"])

	lines := []string{
		"# Tellurian Authentik Blueprint — Rogue Fairy Studios Lore Project Groups",
		"# Target: " + strVal(loreMap["authUrl"]),
		"version: 1",
		"metadata:",
		"  name: " + yamlScalar("Rogue Fairy Studios - Lore - "+projName),
		"entries:",
		"  - model: authentik_core.group",
		"    id: group-rfs-projects-root",
		"    identifiers:",
		"      name: " + yamlScalar(GroupProjectsRoot),
	}

	for _, entry := range sliceOfMaps(state["groupAccess"]) {
		groupLabel := strVal(entry["name"])
		if groupLabel == "" {
			groupLabel = strVal(entry["groupName"])
		}
		if groupLabel == "" {
			groupLabel = fmt.Sprintf("Rogue Fairy Studios - %s - %s", projName, strVal(entry["role"]))
		}
		entrySlug := strings.ReplaceAll(DeriveLoreProjectSlug(projSlug+"-"+groupLabel), ".", "-")
		attrs := ExportAuthentikAttributes(state, entry)
		actions, _ := attrs["lore_actions"].([]string)

		lines = append(lines,
			"  - model: authentik_core.group",
			"    id: group-project-"+entrySlug,
			"    identifiers:",
			"      name: "+yamlScalar(groupLabel),
			"    attrs:",
			"      parent: !KeyOf group-rfs-projects-root",
			"      attributes:",
			"        lore_project: "+yamlScalar(strVal(attrs["lore_project"])),
			"        lore_visibility: "+yamlScalar(strVal(attrs["lore_visibility"])),
		)
		if len(actions) > 0 {
			lines = append(lines, "        lore_actions:")
			for _, a := range actions {
				lines = append(lines, "          - "+a)
			}
		} else {
			lines = append(lines, "        lore_actions: []")
		}
		canCreate := "false"
		if boolVal(attrs["lore_can_create_repo"]) {
			canCreate = "true"
		}
		lines = append(lines, "        lore_can_create_repo: "+canCreate)
	}

	return strings.Join(lines, "\n") + "\n"
}

// CanonicalCreditsManifest ensures deterministic JSON key ordering matching Wiki.js generateCredits / editCredits.
type CanonicalCreditsManifest struct {
	SchemaVersion int                     `json:"schemaVersion"`
	ProjectID     string                  `json:"projectId"`
	ReleaseID     string                  `json:"releaseId"`
	Title         string                  `json:"title"`
	Entries       []CanonicalCreditsEntry `json:"entries"`
}

// CanonicalCreditsEntry holds a single ordered credit row.
type CanonicalCreditsEntry struct {
	ParticipantID string `json:"participantId"`
	Name          string `json:"name"`
	Role          string `json:"role"`
	Section       string `json:"section"`
	Order         int    `json:"order"`
}

// ExportCredits renders an approved credits snapshot in JSON or Markdown.
func ExportCredits(state map[string]any, snapshotID, format string) (string, error) {
	snap, err := findByID(state["credits"], snapshotID)
	if err != nil {
		return "", err
	}
	if strVal(snap["status"]) != "approved" {
		return "", errBadInput("Credits must be approved")
	}
	manifestMap, _ := snap["manifest"].(map[string]any)
	manifest := decodeCreditsManifest(manifestMap)

	if format == "json" {
		b, err := json.Marshal(manifest)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	if format != "markdown" {
		return "", errBadInput("Unsupported format")
	}

	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(escapeMarkdown(manifest.Title))
	b.WriteString("\n\n")
	for i, entry := range manifest.Entries {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("- ")
		b.WriteString(escapeMarkdown(entry.Section))
		b.WriteString(" / ")
		b.WriteString(escapeMarkdown(entry.Role))
		b.WriteString(" — ")
		b.WriteString(escapeMarkdown(entry.Name))
	}
	b.WriteByte('\n')
	return b.String(), nil
}

func decodeCreditsManifest(m map[string]any) CanonicalCreditsManifest {
	if m == nil {
		return CanonicalCreditsManifest{SchemaVersion: 1, Entries: []CanonicalCreditsEntry{}}
	}
	sv, ok := intVal(m["schemaVersion"])
	if !ok || sv == 0 {
		sv = 1
	}
	rawEntries := sliceOfMaps(m["entries"])
	entries := make([]CanonicalCreditsEntry, 0, len(rawEntries))
	for _, e := range rawEntries {
		ord, _ := intVal(e["order"])
		entries = append(entries, CanonicalCreditsEntry{
			ParticipantID: strVal(e["participantId"]),
			Name:          strVal(e["name"]),
			Role:          strVal(e["role"]),
			Section:       strVal(e["section"]),
			Order:         ord,
		})
	}
	return CanonicalCreditsManifest{
		SchemaVersion: sv,
		ProjectID:     strVal(m["projectId"]),
		ReleaseID:     strVal(m["releaseId"]),
		Title:         strVal(m["title"]),
		Entries:       entries,
	}
}

func escapeMarkdown(s string) string {
	const specials = "\\`*_{}[]()#+.!|<>\r\n"
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(specials, r) {
			b.WriteString(fmt.Sprintf("&#%d;", r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Helper functions for dynamic map/slice inspection.

func strVal(v any) string {
	s, _ := v.(string)
	return s
}

func boolVal(v any) bool {
	b, _ := v.(bool)
	return b
}

func intVal(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		if float64(int(n)) == n {
			return int(n), true
		}
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i), true
		}
	}
	return 0, false
}

func floatVal(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return f, true
		}
	}
	return 0, false
}

func strSlice(v any) ([]string, bool) {
	switch s := v.(type) {
	case []string:
		return slices.Clone(s), true
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			str, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, str)
		}
		return out, true
	}
	return nil, false
}

func toAnyStrings(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

func sliceOfMaps(v any) []map[string]any {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func findByID(col any, id string) (map[string]any, error) {
	if id == "" {
		return nil, errBadInput("Record not found")
	}
	for _, m := range sliceOfMaps(col) {
		if strVal(m["id"]) == id {
			return m, nil
		}
	}
	return nil, errBadInput("Record not found")
}

func cloneValue(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

func canonicalValue(v any) any {
	switch val := v.(type) {
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = canonicalValue(item)
		}
		return out
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(val))
		for _, k := range keys {
			out[k] = canonicalValue(val[k])
		}
		return out
	default:
		return val
	}
}
