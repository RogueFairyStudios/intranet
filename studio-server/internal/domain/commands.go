package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/roguefairystudios/intranet/studio-server/internal/store"
)

var managerOnlyCommands = []string{
	"access",
	"groupAccess",
	"integrationAccess",
	"loreConfig",
	"approveCredits",
	"reviewArtifact",
	"publishRelease",
	"project",
}

func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

// ApplyCommand validates permissions and mutates state in-place according to command and input.
func ApplyCommand(
	ctx context.Context,
	st store.Store,
	state map[string]any,
	subj SubjectProfile,
	role string,
	command string,
	input map[string]any,
) error {
	if role != "manager" && role != "editor" {
		return errForbidden("Forbidden")
	}
	if slices.Contains(managerOnlyCommands, command) && role != "manager" {
		return errForbidden("Manager required")
	}
	if input == nil {
		return errBadInput("Invalid input")
	}

	UpgradeState(state)
	if boolVal(state["archived"]) && command != "project" {
		return errBadInput("Project is archived")
	}

	actor := subj.ActorID()

	// Special ACL & Wiki document commands write their own tailored history entry, matching Wiki.js resolvers/studio.js.
	switch command {
	case "access":
		return cmdAccess(ctx, st, state, subj, input, actor)
	case "groupAccess":
		return cmdGroupAccess(ctx, st, state, subj, input, actor)
	case "integrationAccess":
		return cmdIntegrationAccess(ctx, st, state, subj, input, actor)
	case "linkDocument", "document", "createDocument":
		return cmdLinkDocument(state, command, input, actor)
	case "refreshDocument":
		return cmdRefreshDocument(state, input, actor)
	}

	// Standard domain commands append a uniform audit entry to state.history.
	switch command {
	case "project":
		if err := cmdProject(state, input); err != nil {
			return err
		}
	case "participant":
		if err := cmdParticipant(ctx, st, state, subj, input); err != nil {
			return err
		}
	case "endParticipation":
		rec, err := findByID(state["participants"], strVal(input["id"]))
		if err != nil {
			return err
		}
		rec["endedAt"] = nowISO()
	case "team":
		if err := cmdTeam(state, input); err != nil {
			return err
		}
	case "milestone":
		if err := cmdMilestone(state, input); err != nil {
			return err
		}
	case "cycle":
		if err := cmdCycle(state, input); err != nil {
			return err
		}
	case "loreConfig":
		if err := cmdLoreConfig(state, input); err != nil {
			return err
		}
	case "repository":
		if err := cmdRepository(state, input); err != nil {
			return err
		}
	case "vcsStatus":
		if err := cmdVCSStatus(state, input, actor); err != nil {
			return err
		}
	case "artifact":
		if err := cmdArtifact(state, input); err != nil {
			return err
		}
	case "artifactStage":
		if err := cmdArtifactStage(state, input); err != nil {
			return err
		}
	case "artifactVersion":
		if err := cmdArtifactVersion(state, input); err != nil {
			return err
		}
	case "reviewArtifact":
		if err := cmdReviewArtifact(state, input, actor); err != nil {
			return err
		}
	case "validation":
		if err := cmdValidation(state, input, actor); err != nil {
			return err
		}
	case "platformTarget":
		if err := cmdPlatformTarget(state, input); err != nil {
			return err
		}
	case "platformCheckItem":
		if err := cmdPlatformCheckItem(state, input, actor); err != nil {
			return err
		}
	case "platformAssetSlot":
		if err := cmdPlatformAssetSlot(state, input, actor); err != nil {
			return err
		}
	case "release":
		if err := cmdRelease(state, input); err != nil {
			return err
		}
	case "publishRelease":
		if err := cmdPublishRelease(state, input, actor); err != nil {
			return err
		}
	case "task":
		if err := cmdTask(state, input); err != nil {
			return err
		}
	case "transition":
		if err := cmdTransition(state, input); err != nil {
			return err
		}
	case "generateCredits":
		if err := cmdGenerateCredits(state, input); err != nil {
			return err
		}
	case "editCredits":
		if err := cmdEditCredits(state, input); err != nil {
			return err
		}
	case "reviseCredits":
		if err := cmdReviseCredits(state, input); err != nil {
			return err
		}
	case "approveCredits":
		if err := cmdApproveCredits(state, input, actor); err != nil {
			return err
		}
	case "build":
		if err := cmdBuild(state, input); err != nil {
			return err
		}
	case "integrationEvent":
		if err := cmdIntegrationEvent(state, input, actor); err != nil {
			return err
		}
	default:
		return errBadInput("Unknown command")
	}

	var reason any
	if r := strVal(input["reason"]); r != "" {
		reason = r
	}
	appendSliceMap(state, "history", map[string]any{
		"id":     NewUUID(),
		"action": command,
		"actor":  actor,
		"at":     nowISO(),
		"reason": reason,
		"input":  cloneValue(input),
	})
	return nil
}

func appendSliceMap(parent map[string]any, key string, row map[string]any) {
	cur, _ := parent[key].([]any)
	parent[key] = append(cur, row)
}

func saveRow(parent map[string]any, key string, input map[string]any, values map[string]any) error {
	if id := strVal(input["id"]); id != "" {
		existing, err := findByID(parent[key], id)
		if err != nil {
			return err
		}
		for k, v := range values {
			existing[k] = v
		}
		return nil
	}
	row := make(map[string]any, len(values)+1)
	row["id"] = NewUUID()
	for k, v := range values {
		row[k] = v
	}
	appendSliceMap(parent, key, row)
	return nil
}

func refID(col any, raw any) (any, error) {
	s := strVal(raw)
	if s == "" {
		return nil, nil
	}
	rec, err := findByID(col, s)
	if err != nil {
		return nil, err
	}
	return strVal(rec["id"]), nil
}

func refIDs(col any, raw any) ([]any, error) {
	if raw == nil {
		return []any{}, nil
	}
	list, ok := strSlice(raw)
	if !ok {
		return nil, errBadInput("Expected references")
	}
	out := make([]any, 0, len(list))
	seen := make(map[string]bool, len(list))
	for _, item := range list {
		rec, err := findByID(col, item)
		if err != nil {
			return nil, err
		}
		rid := strVal(rec["id"])
		if !seen[rid] {
			seen[rid] = true
			out = append(out, rid)
		}
	}
	return out, nil
}

func parseOptionalDate(raw any) (any, error) {
	if raw == nil {
		return nil, nil
	}
	s, ok := raw.(string)
	if !ok {
		return nil, errBadInput("Invalid date")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format("2006-01-02T15:04:05.000Z"), nil
		}
	}
	return nil, errBadInput("Invalid date")
}

func validatePhase(raw, fallback string) (string, error) {
	if raw == "" {
		return fallback, nil
	}
	if !slices.Contains(Phases, raw) {
		return "", errBadInput("Invalid production phase")
	}
	return raw, nil
}

func validateStage(raw, fallback string) (string, error) {
	if raw == "" {
		return fallback, nil
	}
	if !slices.Contains(Stages, raw) {
		return "", errBadInput("Invalid pipeline stage")
	}
	return raw, nil
}

func validateTrack(raw, fallback string) (string, error) {
	if raw == "" {
		return fallback, nil
	}
	if !slices.Contains(Tracks, raw) {
		return "", errBadInput("Invalid pipeline track")
	}
	return raw, nil
}

func validateTool(raw, fallback string) (string, error) {
	if raw == "" {
		return fallback, nil
	}
	if !slices.Contains(Tools, raw) {
		return "", errBadInput("Invalid studio tool")
	}
	return raw, nil
}

func validateRepoSlug(raw string) (string, error) {
	clean, err := SafeText(raw, 120)
	if err != nil {
		return "", errBadInput("Invalid repository slug")
	}
	clean = strings.ToLower(clean)
	if !reRepoSlug.MatchString(clean) {
		return "", errBadInput("Invalid repository slug")
	}
	return clean, nil
}

func cleanAssetPath(raw any) (string, error) {
	if raw == nil {
		return "", nil
	}
	s := strVal(raw)
	if strings.TrimSpace(s) == "" {
		return "", nil
	}
	clean, err := SafeText(s, 1000)
	if err != nil {
		return "", err
	}
	clean = strings.TrimLeft(strings.ReplaceAll(clean, "\\", "/"), "/")
	if slices.Contains(strings.Split(clean, "/"), "..") {
		return "", errBadInput("Invalid asset path")
	}
	return clean, nil
}

func parseCount(raw any) (int, error) {
	if raw == nil || raw == "" {
		return 0, nil
	}
	n, ok := intVal(raw)
	if !ok || n < 0 || n > 1000000 {
		return 0, errBadInput("Invalid count")
	}
	return n, nil
}

func cmdProject(state, input map[string]any) error {
	kind := strVal(input["kind"])
	if !slices.Contains([]string{"digital", "analogue", "hybrid"}, kind) {
		return errBadInput("Invalid project type")
	}
	name, err := SafeText(strVal(input["name"]), 500)
	if err != nil {
		return err
	}
	state["name"] = name
	state["kind"] = kind
	state["archived"] = boolVal(input["archived"])
	if p := strVal(input["phase"]); p != "" {
		vp, err := validatePhase(p, "")
		if err != nil {
			return err
		}
		state["phase"] = vp
	}
	if e := strVal(input["engine"]); e != "" {
		if !slices.Contains(Engines, e) {
			return errBadInput("Invalid project engine")
		}
		state["engine"] = e
	}
	return nil
}

func cmdParticipant(ctx context.Context, st store.Store, state map[string]any, subj SubjectProfile, input map[string]any) error {
	rawRoles, ok := strSlice(input["roles"])
	if !ok || len(rawRoles) == 0 || len(rawRoles) > 30 {
		return errBadInput("Specify credit roles")
	}
	roles := make([]any, 0, len(rawRoles))
	seenRoles := make(map[string]bool, len(rawRoles))
	for _, r := range rawRoles {
		cleanRole, err := SafeText(r, 120)
		if err != nil {
			return err
		}
		if !seenRoles[cleanRole] {
			seenRoles[cleanRole] = true
			roles = append(roles, cleanRole)
		}
	}

	order := 0
	if rawOrder, exists := input["order"]; exists && rawOrder != nil {
		o, ok := intVal(rawOrder)
		if !ok {
			return errBadInput("Invalid order")
		}
		order = o
	}

	var team map[string]any
	if tid := strVal(input["teamId"]); tid != "" {
		var err error
		team, err = findByID(state["teams"], tid)
		if err != nil {
			return err
		}
		if boolVal(team["archived"]) {
			return errBadInput("Team is archived")
		}
	}

	var userID any
	var linkedEmail string
	if rawUID, exists := input["userId"]; exists && rawUID != nil && rawUID != 0 && rawUID != float64(0) {
		uid, ok := intVal(rawUID)
		if !ok || uid < 1 || uid == 2 {
			return errBadInput("Invalid linked account")
		}
		if st != nil {
			account, err := st.GetWikiUserByID(ctx, uid)
			if err != nil {
				return err
			}
			if account != nil {
				if !account.IsActive || account.ID == 2 {
					return errBadInput("Invalid linked account")
				}
				linkedEmail = account.Email
			} else if !subj.Delegated {
				return errBadInput("Invalid linked account")
			}
		}
		userID = uid
	}

	name, err := SafeText(strVal(input["name"]), 255)
	if err != nil {
		return err
	}
	teamLabel := ""
	var teamID any
	if team != nil {
		teamLabel = strVal(team["name"])
		teamID = strVal(team["id"])
	} else if rawTeam := strVal(input["team"]); rawTeam != "" {
		teamLabel, err = SafeText(rawTeam, 120)
		if err != nil {
			return err
		}
	}

	values := map[string]any{
		"name":   name,
		"roles":  roles,
		"team":   teamLabel,
		"teamId": teamID,
		"userId": userID,
		"order":  order,
	}
	if rawEmail := strVal(input["email"]); rawEmail != "" {
		if cleanEmail, eErr := SafeText(rawEmail, 255); eErr == nil {
			values["email"] = cleanEmail
		}
	} else if linkedEmail != "" {
		values["email"] = linkedEmail
	}

	if pid := strVal(input["id"]); pid != "" {
		existing, err := findByID(state["participants"], pid)
		if err != nil {
			return err
		}
		for k, v := range values {
			existing[k] = v
		}
		return nil
	}

	values["id"] = NewUUID()
	values["startedAt"] = nowISO()
	values["endedAt"] = nil
	appendSliceMap(state, "participants", values)
	return nil
}

func cmdTeam(state, input map[string]any) error {
	name, err := SafeText(strVal(input["name"]), 120)
	if err != nil {
		return err
	}
	return saveRow(state, "teams", input, map[string]any{
		"name":     name,
		"archived": boolVal(input["archived"]),
	})
}

func cmdMilestone(state, input map[string]any) error {
	var existing map[string]any
	if id := strVal(input["id"]); id != "" {
		var err error
		existing, err = findByID(state["milestones"], id)
		if err != nil {
			return err
		}
	}
	name, err := SafeText(strVal(input["name"]), 500)
	if err != nil {
		return err
	}
	fallbackPhase := strVal(state["phase"])
	if fallbackPhase == "" {
		fallbackPhase = "pre_production"
	}
	if existing != nil && strVal(existing["phase"]) != "" {
		fallbackPhase = strVal(existing["phase"])
	}
	ph, err := validatePhase(strVal(input["phase"]), fallbackPhase)
	if err != nil {
		return err
	}
	criteria := ""
	if c := strVal(input["criteria"]); c != "" {
		criteria, err = SafeText(c, 10000)
		if err != nil {
			return err
		}
	}
	dueDate, err := parseOptionalDate(input["dueDate"])
	if err != nil {
		return err
	}
	return saveRow(state, "milestones", input, map[string]any{
		"name":     name,
		"phase":    ph,
		"criteria": criteria,
		"dueDate":  dueDate,
	})
}

func cmdCycle(state, input map[string]any) error {
	name, err := SafeText(strVal(input["name"]), 500)
	if err != nil {
		return err
	}
	startDate, err := parseOptionalDate(input["startDate"])
	if err != nil {
		return err
	}
	endDate, err := parseOptionalDate(input["endDate"])
	if err != nil {
		return err
	}
	if startDate != nil && endDate != nil && strVal(startDate) > strVal(endDate) {
		return errBadInput("Cycle ends before it starts")
	}
	var capacity any = 0
	if rawCap, exists := input["capacity"]; exists && rawCap != nil {
		f, ok := floatVal(rawCap)
		if !ok || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
			return errBadInput("Invalid capacity")
		}
		if float64(int(f)) == f {
			capacity = int(f)
		} else {
			capacity = f
		}
	}
	goal := ""
	if g := strVal(input["goal"]); g != "" {
		goal, err = SafeText(g, 10000)
		if err != nil {
			return err
		}
	}
	return saveRow(state, "cycles", input, map[string]any{
		"name":      name,
		"goal":      goal,
		"startDate": startDate,
		"endDate":   endDate,
		"capacity":  capacity,
	})
}

func cmdLoreConfig(state, input map[string]any) error {
	curLore, _ := state["lore"].(map[string]any)
	projectSlug := strVal(curLore["projectSlug"])
	if _, has := input["projectSlug"]; has {
		var err error
		projectSlug, err = ValidateLoreProjectSlug(strVal(input["projectSlug"]))
		if err != nil {
			return err
		}
	}
	visibility := strVal(curLore["visibility"])
	if _, has := input["visibility"]; has {
		visibility = strVal(input["visibility"])
	}
	if !slices.Contains(LoreVisibilities, visibility) {
		return errBadInput("Invalid Lore project visibility")
	}
	if strVal(curLore["visibility"]) == "restricted" && visibility == "internal" && !boolVal(input["confirmUnrestrict"]) {
		return errBadInput("Confirm before relaxing restricted project visibility")
	}

	employeeRole := strVal(curLore["employeeRole"])
	if _, has := input["employeeRole"]; has {
		employeeRole = strVal(input["employeeRole"])
	}
	if !slices.Contains(LoreEmployeeRoles, employeeRole) {
		return errBadInput("Invalid baseline employee role")
	}
	if visibility == "restricted" {
		if rawEmp := strVal(input["employeeRole"]); rawEmp != "" && rawEmp != "none" {
			return errBadInput("Restricted projects cannot grant baseline employee access")
		}
		employeeRole = "none"
	}

	remoteBase := strVal(curLore["remoteBase"])
	if remoteBase == "" {
		remoteBase = DefaultLoreRemoteBase
	}
	if rb := strVal(input["remoteBase"]); rb != "" {
		var err error
		remoteBase, err = SafeEndpointURL(rb, []string{"lores:", "lore:"}, "Lore remote base")
		if err != nil {
			return err
		}
	}

	bridgeURL := strVal(curLore["bridgeUrl"])
	if bridgeURL == "" {
		bridgeURL = DefaultLoreBridgeURL
	}
	if bu := strVal(input["bridgeUrl"]); bu != "" {
		var err error
		bridgeURL, err = SafeEndpointURL(bu, []string{"https:", "http:"}, "Lore OIDC bridge URL")
		if err != nil {
			return err
		}
	}

	authURL := strVal(curLore["authUrl"])
	if authURL == "" {
		authURL = DefaultLoreAuthURL
	}
	if au := strVal(input["authUrl"]); au != "" {
		var err error
		authURL, err = SafeEndpointURL(au, []string{"https:", "http:"}, "Authentik URL")
		if err != nil {
			return err
		}
	}

	state["lore"] = map[string]any{
		"projectSlug":  projectSlug,
		"visibility":   visibility,
		"employeeRole": employeeRole,
		"remoteBase":   remoteBase,
		"bridgeUrl":    bridgeURL,
		"authUrl":      authURL,
	}
	return nil
}

func extractOptionalLoreFields(system, cleanedRemote string, input map[string]any) (string, string, string, error) {
	if system != "lore" {
		return "", "", "", nil
	}
	if strVal(input["loreProject"]) != "" && strVal(input["loreRepo"]) != "" {
		p, err := ValidateLoreProjectSlug(strVal(input["loreProject"]))
		if err != nil {
			return "", "", "", err
		}
		r, err := ValidateLoreRepoPath(strVal(input["loreRepo"]))
		if err != nil {
			return "", "", "", err
		}
		return p, r, p + "/" + r, nil
	}
	if cleanedRemote != "" {
		if u, err := url.Parse(cleanedRemote); err == nil {
			pPath := strings.Trim(u.Path, "/")
			if strings.Contains(pPath, "/") {
				if p, r, full, pErr := ParseLoreFullName(pPath); pErr == nil {
					return p, r, full, nil
				}
			}
		}
	}
	return "", "", "", nil
}

func cmdRepository(state, input map[string]any) error {
	var existing map[string]any
	if id := strVal(input["id"]); id != "" {
		var err error
		existing, err = findByID(state["repositories"], id)
		if err != nil {
			return err
		}
	}
	system := strVal(input["system"])
	if !slices.Contains(VCSSystems, system) {
		return errBadInput("Invalid VCS system")
	}

	loreMap, _ := state["lore"].(map[string]any)
	projSlug := strVal(loreMap["projectSlug"])

	fallbackSlug := strVal(input["slug"])
	if fallbackSlug == "" {
		if system == "lore" && strVal(input["loreRepo"]) != "" {
			fallbackSlug = projSlug + "-" + strings.ReplaceAll(strVal(input["loreRepo"]), "/", "-")
		} else {
			fallbackSlug = strVal(input["name"])
		}
	}
	normalizedCandidate := reTrimSlugEdges.ReplaceAllString(reNonSlugChars.ReplaceAllString(strings.ToLower(fallbackSlug), "-"), "")
	repoSlug, err := validateRepoSlug(normalizedCandidate)
	if err != nil {
		return err
	}
	for _, row := range sliceOfMaps(state["repositories"]) {
		if strVal(row["slug"]) == repoSlug && (existing == nil || strVal(row["id"]) != strVal(existing["id"])) {
			return errBadInput("Repository slug already exists")
		}
	}

	cleanedRemote, err := SafeRemoteURI(strVal(input["remoteUri"]))
	if err != nil {
		return err
	}

	var loreProject, loreRepo, loreFullName string
	if system == "lore" {
		base := strings.TrimRight(strVal(loreMap["remoteBase"]), "/")
		if base == "" {
			base = DefaultLoreRemoteBase
		}
		if strVal(input["loreRepo"]) != "" {
			if strVal(input["loreProject"]) != "" {
				loreProject, err = ValidateLoreProjectSlug(strVal(input["loreProject"]))
				if err != nil {
					return err
				}
			} else {
				loreProject = projSlug
			}
			loreRepo, err = ValidateLoreRepoPath(strVal(input["loreRepo"]))
			if err != nil {
				return err
			}
			loreFullName = loreProject + "/" + loreRepo
			if cleanedRemote == "" {
				cleanedRemote, err = SafeRemoteURI(base + "/" + loreFullName)
				if err != nil {
					return err
				}
			} else {
				u, uErr := url.Parse(cleanedRemote)
				if uErr != nil {
					return errBadInput("Unsupported repository remote URI")
				}
				_, _, parsedFull, pErr := ParseLoreFullName(u.Path)
				if pErr != nil {
					return pErr
				}
				if parsedFull != loreFullName {
					return errBadInput("Lore remote URI path does not match <project>/<repo>")
				}
			}
		} else if cleanedRemote != "" {
			u, uErr := url.Parse(cleanedRemote)
			if uErr != nil {
				return errBadInput("Unsupported repository remote URI")
			}
			loreProject, loreRepo, loreFullName, err = ParseLoreFullName(u.Path)
			if err != nil {
				return err
			}
		} else {
			loreProject = projSlug
			loreRepo, err = ValidateLoreRepoPath(repoSlug)
			if err != nil {
				return err
			}
			loreFullName = loreProject + "/" + loreRepo
			cleanedRemote, err = SafeRemoteURI(base + "/" + loreFullName)
			if err != nil {
				return err
			}
		}
	}

	name, err := SafeText(strVal(input["name"]), 200)
	if err != nil {
		return err
	}
	defaultBranch := "main"
	if db := strVal(input["defaultBranch"]); db != "" {
		defaultBranch, err = SafeText(db, 200)
		if err != nil {
			return err
		}
	}
	wsPath, err := cleanAssetPath(input["workspacePath"])
	if err != nil {
		return err
	}
	var prevStatus any
	if existing != nil {
		prevStatus = existing["status"]
	}

	return saveRow(state, "repositories", input, map[string]any{
		"name":          name,
		"slug":          repoSlug,
		"system":        system,
		"remoteUri":     cleanedRemote,
		"loreProject":   loreProject,
		"loreRepo":      loreRepo,
		"loreFullName":  loreFullName,
		"defaultBranch": defaultBranch,
		"workspacePath": wsPath,
		"status":        prevStatus,
	})
}

func cmdVCSStatus(state, input map[string]any, actor any) error {
	var repo map[string]any
	if rid := strVal(input["repositoryId"]); rid != "" {
		var err error
		repo, err = findByID(state["repositories"], rid)
		if err != nil {
			return err
		}
	} else if rawSlug := strVal(input["slug"]); rawSlug != "" {
		repoSlug, err := validateRepoSlug(rawSlug)
		if err != nil {
			return err
		}
		for _, r := range sliceOfMaps(state["repositories"]) {
			if strVal(r["slug"]) == repoSlug {
				repo = r
				break
			}
		}
		if repo == nil {
			sys := "git"
			if slices.Contains(VCSSystems, strVal(input["system"])) {
				sys = strVal(input["system"])
			}
			initialRemote, err := SafeRemoteURI(strVal(input["remoteUri"]))
			if err != nil {
				return err
			}
			lp, lr, lf, err := extractOptionalLoreFields(sys, initialRemote, input)
			if err != nil {
				return err
			}
			repoName := repoSlug
			if n := strVal(input["name"]); n != "" {
				repoName, err = SafeText(n, 200)
				if err != nil {
					return err
				}
			}
			defBranch := "main"
			if db := strVal(input["defaultBranch"]); db != "" {
				defBranch, err = SafeText(db, 200)
				if err != nil {
					return err
				}
			} else if b := strVal(input["branch"]); b != "" {
				defBranch, err = SafeText(b, 200)
				if err != nil {
					return err
				}
			}
			wsPath, err := cleanAssetPath(input["workspacePath"])
			if err != nil {
				return err
			}
			repo = map[string]any{
				"id":            NewUUID(),
				"name":          repoName,
				"slug":          repoSlug,
				"system":        sys,
				"remoteUri":     initialRemote,
				"loreProject":   lp,
				"loreRepo":      lr,
				"loreFullName":  lf,
				"defaultBranch": defBranch,
				"workspacePath": wsPath,
				"status":        nil,
			}
			appendSliceMap(state, "repositories", repo)
		}
	} else {
		return errBadInput("Specify repositoryId or slug")
	}

	if ru := strVal(input["remoteUri"]); ru != "" {
		cleanRemote, err := SafeRemoteURI(ru)
		if err != nil {
			return err
		}
		repo["remoteUri"] = cleanRemote
	}
	if strVal(repo["system"]) == "lore" {
		lp, lr, lf, err := extractOptionalLoreFields("lore", strVal(repo["remoteUri"]), input)
		if err != nil {
			return err
		}
		if lf != "" {
			repo["loreProject"] = lp
			repo["loreRepo"] = lr
			repo["loreFullName"] = lf
		}
	}

	modifiedFiles := []any{}
	if rawMods, ok := input["modifiedFiles"].([]any); ok {
		limit := len(rawMods)
		if limit > 100 {
			limit = 100
		}
		for _, item := range rawMods[:limit] {
			p, err := cleanAssetPath(item)
			if err != nil {
				return err
			}
			if p != "" {
				modifiedFiles = append(modifiedFiles, p)
			}
		}
	}

	locks := []any{}
	if rawLocks, ok := input["locks"].([]any); ok {
		limit := len(rawLocks)
		if limit > 200 {
			limit = 200
		}
		for _, item := range rawLocks[:limit] {
			lm, ok := item.(map[string]any)
			if !ok {
				return errBadInput("Invalid lock entry")
			}
			p, err := cleanAssetPath(lm["path"])
			if err != nil {
				return err
			}
			owner, err := SafeText(strVal(lm["owner"]), 200)
			if err != nil {
				return err
			}
			branch := ""
			if b := strVal(lm["branch"]); b != "" {
				branch, err = SafeText(b, 200)
				if err != nil {
					return err
				}
			}
			lockedAt, err := parseOptionalDate(lm["lockedAt"])
			if err != nil {
				return err
			}
			if p != "" {
				locks = append(locks, map[string]any{
					"path":     p,
					"owner":    owner,
					"branch":   branch,
					"lockedAt": lockedAt,
				})
			}
		}
	}

	branch := strVal(repo["defaultBranch"])
	if branch == "" {
		branch = "main"
	}
	if b := strVal(input["branch"]); b != "" {
		var err error
		branch, err = SafeText(b, 200)
		if err != nil {
			return err
		}
	}

	revision := ""
	if r := strVal(input["revision"]); r != "" {
		var err error
		revision, err = SafeText(r, 200)
		if err != nil {
			return err
		}
	}
	var revisionNumber any
	if rn, ok := intVal(input["revisionNumber"]); ok && rn >= 0 {
		revisionNumber = rn
	}
	commitMessage := ""
	if cm := strVal(input["commitMessage"]); cm != "" {
		var err error
		commitMessage, err = SafeText(cm, 1000)
		if err != nil {
			return err
		}
	}
	author := ""
	if au := strVal(input["author"]); au != "" {
		var err error
		author, err = SafeText(au, 200)
		if err != nil {
			return err
		}
	}

	modCountInput := input["modifiedCount"]
	explicitModCount, err := parseCount(modCountInput)
	if err != nil {
		return err
	}
	modCount := explicitModCount
	if modCountInput == nil {
		modCount = len(modifiedFiles)
	}
	stagedCount, err := parseCount(input["stagedCount"])
	if err != nil {
		return err
	}
	untrackedCount, err := parseCount(input["untrackedCount"])
	if err != nil {
		return err
	}
	ahead, err := parseCount(input["ahead"])
	if err != nil {
		return err
	}
	behind, err := parseCount(input["behind"])
	if err != nil {
		return err
	}

	cleanFlag := true
	if rawClean, ok := input["clean"].(bool); ok && !rawClean {
		cleanFlag = false
	}
	isClean := cleanFlag && len(modifiedFiles) == 0 && explicitModCount == 0 && stagedCount == 0

	compression := ""
	if comp := strVal(input["compression"]); comp != "" {
		compression, err = SafeText(comp, 40)
		if err != nil {
			return err
		}
	}

	repo["status"] = map[string]any{
		"branch":         branch,
		"revision":       revision,
		"revisionNumber": revisionNumber,
		"commitMessage":  commitMessage,
		"author":         author,
		"clean":          isClean,
		"modifiedCount":  modCount,
		"stagedCount":    stagedCount,
		"untrackedCount": untrackedCount,
		"ahead":          ahead,
		"behind":         behind,
		"modifiedFiles":  modifiedFiles,
		"locks":          locks,
		"compression":    compression,
		"updatedAt":      nowISO(),
		"reportedBy":     fmt.Sprintf("%v", actor),
	}
	return nil
}

func cmdArtifact(state, input map[string]any) error {
	var existing map[string]any
	if id := strVal(input["id"]); id != "" {
		var err error
		existing, err = findByID(state["artifacts"], id)
		if err != nil {
			return err
		}
	}
	defaultTool := "generic"
	if existing != nil && strVal(existing["tool"]) != "" {
		defaultTool = strVal(existing["tool"])
	} else if src := strVal(input["source"]); slices.Contains(Tools, src) {
		defaultTool = src
	}

	var resolvedToolURI any
	if rawTU, has := input["toolUri"]; has {
		if s := strVal(rawTU); s != "" {
			cleanURI, err := SafeURI(s)
			if err != nil {
				return err
			}
			resolvedToolURI = cleanURI
		} else {
			resolvedToolURI = nil
		}
	} else if existing != nil {
		resolvedToolURI = existing["toolUri"]
	}

	name, err := SafeText(strVal(input["name"]), 500)
	if err != nil {
		return err
	}
	kind, err := SafeText(strVal(input["kind"]), 120)
	if err != nil {
		return err
	}

	fbTrack := "general"
	if existing != nil && strVal(existing["track"]) != "" {
		fbTrack = strVal(existing["track"])
	}
	tr, err := validateTrack(strVal(input["track"]), fbTrack)
	if err != nil {
		return err
	}

	fbStage := "concept"
	if existing != nil && strVal(existing["stage"]) != "" {
		fbStage = strVal(existing["stage"])
	}
	stVal, err := validateStage(strVal(input["stage"]), fbStage)
	if err != nil {
		return err
	}

	tl, err := validateTool(strVal(input["tool"]), defaultTool)
	if err != nil {
		return err
	}

	var repoID any
	if _, has := input["repoId"]; has {
		repoID, err = refID(state["repositories"], input["repoId"])
		if err != nil {
			return err
		}
	} else if existing != nil {
		repoID = existing["repoId"]
	}

	assetPath := ""
	if _, has := input["assetPath"]; has {
		assetPath, err = cleanAssetPath(input["assetPath"])
		if err != nil {
			return err
		}
	} else if existing != nil {
		assetPath = strVal(existing["assetPath"])
	}

	var taskID any
	if _, has := input["taskId"]; has {
		taskID, err = refID(state["tasks"], input["taskId"])
		if err != nil {
			return err
		}
	} else if existing != nil {
		taskID = existing["taskId"]
	}

	var docIDs []any
	if _, has := input["documentIds"]; has {
		docIDs, err = refIDs(state["documents"], input["documentIds"])
		if err != nil {
			return err
		}
	} else if existing != nil {
		if cur, ok := existing["documentIds"].([]any); ok {
			docIDs = cur
		} else {
			docIDs = []any{}
		}
	} else {
		docIDs = []any{}
	}

	return saveRow(state, "artifacts", input, map[string]any{
		"name":        name,
		"kind":        kind,
		"track":       tr,
		"stage":       stVal,
		"tool":        tl,
		"toolUri":     resolvedToolURI,
		"repoId":      repoID,
		"assetPath":   assetPath,
		"taskId":      taskID,
		"documentIds": docIDs,
	})
}

func cmdArtifactStage(state, input map[string]any) error {
	var art map[string]any
	if id := strVal(input["id"]); id != "" {
		var err error
		art, err = findByID(state["artifacts"], id)
		if err != nil {
			return err
		}
	} else if extID, src := strVal(input["externalId"]), strVal(input["source"]); extID != "" && src != "" {
		for _, r := range sliceOfMaps(state["artifacts"]) {
			if strVal(r["externalId"]) == extID && strVal(r["source"]) == src {
				art = r
				break
			}
		}
		if art == nil {
			return errBadInput("External artifact not registered")
		}
	} else {
		return errBadInput("Artifact not found")
	}

	stVal, err := validateStage(strVal(input["stage"]), "")
	if err != nil || stVal == "" {
		return errBadInput("Invalid pipeline stage")
	}
	art["stage"] = stVal
	if tu := strVal(input["toolUri"]); tu != "" {
		cleanURI, err := SafeURI(tu)
		if err != nil {
			return err
		}
		art["toolUri"] = cleanURI
	}
	if rev := strVal(input["vcsRevision"]); rev != "" {
		cleanRev, err := SafeText(rev, 200)
		if err != nil {
			return err
		}
		art["lastVcsRevision"] = cleanRev
	}
	note := ""
	if n := strVal(input["note"]); n != "" {
		note, err = SafeText(n, 2000)
		if err != nil {
			return err
		}
	}
	art["stageNote"] = note
	art["stageUpdatedAt"] = nowISO()
	return nil
}

func cmdArtifactVersion(state, input map[string]any) error {
	artID := strVal(input["artifactId"])
	if artID == "" && strVal(input["externalId"]) != "" {
		extID, src := strVal(input["externalId"]), strVal(input["source"])
		for _, r := range sliceOfMaps(state["artifacts"]) {
			if strVal(r["externalId"]) == extID && strVal(r["source"]) == src {
				artID = strVal(r["id"])
				break
			}
		}
		if artID == "" {
			return errBadInput("External artifact not registered")
		}
	}
	parent, err := findByID(state["artifacts"], artID)
	if err != nil {
		return err
	}
	version, err := SafeText(strVal(input["version"]), 120)
	if err != nil {
		return err
	}
	checksumRaw := strVal(input["checksum"])
	if !reSHA256Hex.MatchString(checksumRaw) {
		return errBadInput("Expected SHA-256 checksum")
	}
	checksum := strings.ToLower(checksumRaw)

	fbStage := strVal(parent["stage"])
	if fbStage == "" {
		fbStage = "concept"
	}
	resolvedStage, err := validateStage(strVal(input["stage"]), fbStage)
	if err != nil {
		return err
	}

	cleanURI, err := SafeURI(strVal(input["uri"]))
	if err != nil {
		return err
	}
	var previewURI any
	if pu := strVal(input["previewUri"]); pu != "" {
		c, err := SafeURI(pu)
		if err != nil {
			return err
		}
		previewURI = c
	}
	var toolURI any
	if tu := strVal(input["toolUri"]); tu != "" {
		c, err := SafeURI(tu)
		if err != nil {
			return err
		}
		toolURI = c
	}
	var vcsRevision any
	if vr := strVal(input["vcsRevision"]); vr != "" {
		c, err := SafeText(vr, 200)
		if err != nil {
			return err
		}
		vcsRevision = c
	}
	var vcsBranch any
	if vb := strVal(input["vcsBranch"]); vb != "" {
		c, err := SafeText(vb, 200)
		if err != nil {
			return err
		}
		vcsBranch = c
	}

	for _, existing := range sliceOfMaps(state["artifactVersions"]) {
		if strVal(existing["artifactId"]) == artID && strVal(existing["version"]) == version {
			if strVal(existing["checksum"]) != checksum ||
				strVal(existing["uri"]) != cleanURI ||
				strVal(existing["previewUri"]) != strVal(previewURI) {
				return errBadInput("Artifact version already exists with different content")
			}
			return nil
		}
	}

	if strVal(input["stage"]) != "" {
		parent["stage"] = resolvedStage
	}
	if toolURI != nil && strVal(parent["toolUri"]) == "" {
		parent["toolUri"] = toolURI
	}
	if vcsRevision != nil {
		parent["lastVcsRevision"] = vcsRevision
	}

	appendSliceMap(state, "artifactVersions", map[string]any{
		"id":          NewUUID(),
		"artifactId":  artID,
		"version":     version,
		"uri":         cleanURI,
		"checksum":    checksum,
		"previewUri":  previewURI,
		"toolUri":     toolURI,
		"stage":       resolvedStage,
		"vcsRevision": vcsRevision,
		"vcsBranch":   vcsBranch,
		"createdAt":   nowISO(),
	})
	return nil
}

func cmdReviewArtifact(state, input map[string]any, actor any) error {
	vid := strVal(input["versionId"])
	if _, err := findByID(state["artifactVersions"], vid); err != nil {
		return err
	}
	decision := strVal(input["decision"])
	if decision != "approved" && decision != "changes" {
		return errBadInput("Invalid review decision")
	}
	comment := ""
	if c := strVal(input["comment"]); c != "" {
		var err error
		comment, err = SafeText(c, 10000)
		if err != nil {
			return err
		}
	}
	appendSliceMap(state, "artifactReviews", map[string]any{
		"id":        NewUUID(),
		"versionId": vid,
		"decision":  decision,
		"comment":   comment,
		"actor":     actor,
		"at":        nowISO(),
	})
	return nil
}

func cmdValidation(state, input map[string]any, actor any) error {
	kind := strVal(input["kind"])
	result := strVal(input["result"])
	if (kind != "test" && kind != "playtest") || !slices.Contains([]string{"passed", "failed", "pending"}, result) {
		return errBadInput("Invalid validation")
	}
	name, err := SafeText(strVal(input["name"]), 500)
	if err != nil {
		return err
	}
	notes := ""
	if n := strVal(input["notes"]); n != "" {
		notes, err = SafeText(n, 20000)
		if err != nil {
			return err
		}
	}
	relID, err := refID(state["releases"], input["releaseId"])
	if err != nil {
		return err
	}
	verID, err := refID(state["artifactVersions"], input["artifactVersionId"])
	if err != nil {
		return err
	}
	taskID, err := refID(state["tasks"], input["taskId"])
	if err != nil {
		return err
	}
	appendSliceMap(state, "validations", map[string]any{
		"id":                NewUUID(),
		"name":              name,
		"kind":              kind,
		"result":            result,
		"notes":             notes,
		"releaseId":         relID,
		"artifactVersionId": verID,
		"taskId":            taskID,
		"actor":             actor,
		"at":                nowISO(),
	})
	return nil
}

func cmdPlatformTarget(state, input map[string]any) error {
	var existing map[string]any
	if id := strVal(input["id"]); id != "" {
		var err error
		existing, err = findByID(state["platformTargets"], id)
		if err != nil {
			return err
		}
	}
	sf := strVal(input["storefront"])
	if sf == "" && existing != nil {
		sf = strVal(existing["storefront"])
	}
	if !isValidStorefront(sf) {
		return errBadInput("Invalid storefront platform")
	}

	subStatus := strVal(input["submissionStatus"])
	if subStatus == "" {
		if existing != nil {
			subStatus = strVal(existing["submissionStatus"])
		} else {
			subStatus = "preparing"
		}
	}
	if !slices.Contains(SubmissionStatuses, subStatus) {
		return errBadInput("Invalid submission status")
	}

	var preset BuiltStorefrontPreset
	if existing == nil {
		locale := strVal(input["locale"])
		if locale == "" {
			locale = "pt"
		}
		preset = BuildStorefrontPreset(sf, locale)
	}

	var name string
	if n := strVal(input["name"]); n != "" {
		var err error
		name, err = SafeText(n, 200)
		if err != nil {
			return err
		}
	} else if existing != nil {
		name = strVal(existing["name"])
	} else {
		name = preset.DefaultName
	}

	var relID any
	if _, has := input["releaseId"]; has {
		var err error
		relID, err = refID(state["releases"], input["releaseId"])
		if err != nil {
			return err
		}
	} else if existing != nil {
		relID = existing["releaseId"]
	}

	storeURL := ""
	if rawURL, has := input["storeUrl"]; has {
		if s := strVal(rawURL); s != "" {
			var err error
			storeURL, err = SafeURI(s)
			if err != nil {
				return err
			}
		}
	} else if existing != nil {
		storeURL = strVal(existing["storeUrl"])
	}

	appID := ""
	if rawApp, has := input["appIdentifier"]; has {
		if s := strVal(rawApp); s != "" {
			var err error
			appID, err = SafeText(s, 200)
			if err != nil {
				return err
			}
		}
	} else if existing != nil {
		appID = strVal(existing["appIdentifier"])
	}

	var targetDate any
	if _, has := input["targetDate"]; has {
		var err error
		targetDate, err = parseOptionalDate(input["targetDate"])
		if err != nil {
			return err
		}
	} else if existing != nil {
		targetDate = existing["targetDate"]
	}

	notes := ""
	if rawNotes, has := input["notes"]; has {
		if s := strVal(rawNotes); s != "" {
			var err error
			notes, err = SafeText(s, 10000)
			if err != nil {
				return err
			}
		}
	} else if existing != nil {
		notes = strVal(existing["notes"])
	}

	values := map[string]any{
		"storefront":       sf,
		"name":             name,
		"releaseId":        relID,
		"storeUrl":         storeURL,
		"appIdentifier":    appID,
		"submissionStatus": subStatus,
		"targetDate":       targetDate,
		"notes":            notes,
		"archived":         boolVal(input["archived"]),
	}
	if existing != nil {
		for k, v := range values {
			existing[k] = v
		}
		return nil
	}

	usePreset := true
	if rawPreset, ok := input["applyPreset"].(bool); ok && !rawPreset {
		usePreset = false
	}
	values["id"] = NewUUID()
	if usePreset {
		values["checklist"] = preset.Checklist
		values["assets"] = preset.Assets
	} else {
		values["checklist"] = []any{}
		values["assets"] = []any{}
	}
	values["createdAt"] = nowISO()
	appendSliceMap(state, "platformTargets", values)
	return nil
}

func cmdPlatformCheckItem(state, input map[string]any, actor any) error {
	target, err := findByID(state["platformTargets"], strVal(input["targetId"]))
	if err != nil {
		return err
	}
	var existing map[string]any
	if id := strVal(input["id"]); id != "" {
		existing, err = findByID(target["checklist"], id)
		if err != nil {
			return err
		}
	}
	cat := strVal(input["category"])
	if cat == "" && existing != nil {
		cat = strVal(existing["category"])
	}
	if !slices.Contains(CheckCategories, cat) {
		return errBadInput("Invalid checklist category")
	}

	status := strVal(input["status"])
	if status == "" {
		if existing != nil {
			status = strVal(existing["status"])
		} else {
			status = "pending"
		}
	}
	if !slices.Contains(ItemStatuses, status) {
		return errBadInput("Invalid checklist status")
	}

	required := true
	if rawReq, has := input["required"]; has {
		if b, ok := rawReq.(bool); ok {
			required = b
		}
	} else if existing != nil {
		if b, ok := existing["required"].(bool); ok {
			required = b
		}
	}

	waiverReason := ""
	if rawWaiver, has := input["waiverReason"]; has {
		if s := strVal(rawWaiver); s != "" {
			waiverReason, err = SafeText(s, 2000)
			if err != nil {
				return err
			}
		}
	} else if existing != nil {
		waiverReason = strVal(existing["waiverReason"])
	}
	if status == "na" && required && waiverReason == "" {
		return errBadInput("Specify waiver justification for N/A item")
	}

	var title string
	if t := strVal(input["title"]); t != "" || existing == nil {
		title, err = SafeText(t, 300)
		if err != nil {
			return err
		}
	} else {
		title = strVal(existing["title"])
	}

	spec := ""
	if rawSpec, has := input["spec"]; has {
		if s := strVal(rawSpec); s != "" {
			spec, err = SafeText(s, 2000)
			if err != nil {
				return err
			}
		}
	} else if existing != nil {
		spec = strVal(existing["spec"])
	}

	var assigneeID, docID, taskID any
	if _, has := input["assigneeId"]; has {
		assigneeID, err = refID(state["participants"], input["assigneeId"])
		if err != nil {
			return err
		}
	} else if existing != nil {
		assigneeID = existing["assigneeId"]
	}
	if _, has := input["documentId"]; has {
		docID, err = refID(state["documents"], input["documentId"])
		if err != nil {
			return err
		}
	} else if existing != nil {
		docID = existing["documentId"]
	}
	if _, has := input["taskId"]; has {
		taskID, err = refID(state["tasks"], input["taskId"])
		if err != nil {
			return err
		}
	} else if existing != nil {
		taskID = existing["taskId"]
	}

	notes := ""
	if rawNotes, has := input["notes"]; has {
		if s := strVal(rawNotes); s != "" {
			notes, err = SafeText(s, 4000)
			if err != nil {
				return err
			}
		}
	} else if existing != nil {
		notes = strVal(existing["notes"])
	}

	resolved := status == "done" || status == "na"
	var verifiedBy, verifiedAt any
	if resolved {
		verifiedBy = actor
		verifiedAt = nowISO()
	}
	if status != "na" {
		waiverReason = ""
	}

	return saveRow(target, "checklist", input, map[string]any{
		"category":     cat,
		"title":        title,
		"spec":         spec,
		"required":     required,
		"status":       status,
		"waiverReason": waiverReason,
		"assigneeId":   assigneeID,
		"documentId":   docID,
		"taskId":       taskID,
		"notes":        notes,
		"verifiedBy":   verifiedBy,
		"verifiedAt":   verifiedAt,
	})
}

func cmdPlatformAssetSlot(state, input map[string]any, actor any) error {
	target, err := findByID(state["platformTargets"], strVal(input["targetId"]))
	if err != nil {
		return err
	}
	var existing map[string]any
	if id := strVal(input["id"]); id != "" {
		existing, err = findByID(target["assets"], id)
		if err != nil {
			return err
		}
	}

	status := strVal(input["status"])
	if status == "" {
		if existing != nil {
			status = strVal(existing["status"])
		} else {
			status = "pending"
		}
	}
	if !slices.Contains(ItemStatuses, status) {
		return errBadInput("Invalid storefront asset status")
	}

	required := true
	if rawReq, has := input["required"]; has {
		if b, ok := rawReq.(bool); ok {
			required = b
		}
	} else if existing != nil {
		if b, ok := existing["required"].(bool); ok {
			required = b
		}
	}

	waiverReason := ""
	if rawWaiver, has := input["waiverReason"]; has {
		if s := strVal(rawWaiver); s != "" {
			waiverReason, err = SafeText(s, 2000)
			if err != nil {
				return err
			}
		}
	} else if existing != nil {
		waiverReason = strVal(existing["waiverReason"])
	}
	if status == "na" && required && waiverReason == "" {
		return errBadInput("Specify waiver justification for N/A storefront asset")
	}

	var artVerID any
	if _, has := input["artifactVersionId"]; has {
		artVerID, err = refID(state["artifactVersions"], input["artifactVersionId"])
		if err != nil {
			return err
		}
	} else if existing != nil {
		artVerID = existing["artifactVersionId"]
	}

	var resolvedURI any
	if rawURI, has := input["uri"]; has {
		if s := strVal(rawURI); s != "" {
			c, err := SafeURI(s)
			if err != nil {
				return err
			}
			resolvedURI = c
		}
	} else if existing != nil {
		resolvedURI = existing["uri"]
	}

	var resolvedToolURI any
	if rawTU, has := input["toolUri"]; has {
		if s := strVal(rawTU); s != "" {
			c, err := SafeURI(s)
			if err != nil {
				return err
			}
			resolvedToolURI = c
		}
	} else if existing != nil {
		resolvedToolURI = existing["toolUri"]
	}

	if status == "done" && strVal(artVerID) == "" && strVal(resolvedURI) == "" {
		return errBadInput("Attach an artifact version or file URI before marking storefront asset done")
	}

	var checksum any
	if existing != nil {
		checksum = existing["checksum"]
	}
	if rawCS, has := input["checksum"]; has {
		if rawCS == nil || strVal(rawCS) == "" {
			checksum = nil
		} else {
			cs := strVal(rawCS)
			if !reSHA256Hex.MatchString(cs) {
				return errBadInput("Expected SHA-256 checksum")
			}
			checksum = strings.ToLower(cs)
		}
	}

	var name string
	if n := strVal(input["name"]); n != "" || existing == nil {
		name, err = SafeText(n, 200)
		if err != nil {
			return err
		}
	} else {
		name = strVal(existing["name"])
	}

	dims := ""
	if rawDims, has := input["dimensions"]; has {
		if s := strVal(rawDims); s != "" {
			dims, err = SafeText(s, 120)
			if err != nil {
				return err
			}
		}
	} else if existing != nil {
		dims = strVal(existing["dimensions"])
	}

	format := ""
	if rawFmt, has := input["format"]; has {
		if s := strVal(rawFmt); s != "" {
			format, err = SafeText(s, 120)
			if err != nil {
				return err
			}
		}
	} else if existing != nil {
		format = strVal(existing["format"])
	}

	spec := ""
	if rawSpec, has := input["spec"]; has {
		if s := strVal(rawSpec); s != "" {
			spec, err = SafeText(s, 2000)
			if err != nil {
				return err
			}
		}
	} else if existing != nil {
		spec = strVal(existing["spec"])
	}

	locale := "all"
	if l := strVal(input["locale"]); l != "" {
		locale, err = SafeText(l, 20)
		if err != nil {
			return err
		}
	} else if existing != nil && strVal(existing["locale"]) != "" {
		locale = strVal(existing["locale"])
	}

	notes := ""
	if rawNotes, has := input["notes"]; has {
		if s := strVal(rawNotes); s != "" {
			notes, err = SafeText(s, 4000)
			if err != nil {
				return err
			}
		}
	} else if existing != nil {
		notes = strVal(existing["notes"])
	}

	resolved := status == "done" || status == "na"
	var verifiedBy, verifiedAt any
	if resolved {
		verifiedBy = actor
		verifiedAt = nowISO()
	}
	if status != "na" {
		waiverReason = ""
	}

	return saveRow(target, "assets", input, map[string]any{
		"name":              name,
		"dimensions":        dims,
		"format":            format,
		"spec":              spec,
		"required":          required,
		"status":            status,
		"waiverReason":      waiverReason,
		"artifactVersionId": artVerID,
		"uri":               resolvedURI,
		"checksum":          checksum,
		"toolUri":           resolvedToolURI,
		"locale":            locale,
		"notes":             notes,
		"verifiedBy":        verifiedBy,
		"verifiedAt":        verifiedAt,
	})
}

func cmdRelease(state, input map[string]any) error {
	var existing map[string]any
	if id := strVal(input["id"]); id != "" {
		var err error
		existing, err = findByID(state["releases"], id)
		if err != nil {
			return err
		}
		if strVal(existing["publishedAt"]) != "" {
			return errBadInput("Published release is immutable")
		}
	}
	name, err := SafeText(strVal(input["name"]), 500)
	if err != nil {
		return err
	}
	criteria := ""
	if c := strVal(input["criteria"]); c != "" {
		criteria, err = SafeText(c, 10000)
		if err != nil {
			return err
		}
	}
	artVerIDs, err := refIDs(state["artifactVersions"], input["artifactVersionIds"])
	if err != nil {
		return err
	}
	valIDs, err := refIDs(state["validations"], input["validationIds"])
	if err != nil {
		return err
	}
	var ptIDs []any
	if _, has := input["platformTargetIds"]; has {
		ptIDs, err = refIDs(state["platformTargets"], input["platformTargetIds"])
		if err != nil {
			return err
		}
	} else if existing != nil {
		if cur, ok := existing["platformTargetIds"].([]any); ok {
			ptIDs = cur
		} else {
			ptIDs = []any{}
		}
	} else {
		ptIDs = []any{}
	}

	return saveRow(state, "releases", input, map[string]any{
		"name":               name,
		"criteria":           criteria,
		"artifactVersionIds": artVerIDs,
		"validationIds":      valIDs,
		"platformTargetIds":  ptIDs,
		"publishedAt":        nil,
	})
}

func isArtifactVersionApproved(state map[string]any, versionID string) (string, bool) {
	var lastReview map[string]any
	for _, rev := range sliceOfMaps(state["artifactReviews"]) {
		if strVal(rev["versionId"]) == versionID {
			lastReview = rev
		}
	}
	if lastReview != nil && strVal(lastReview["decision"]) == "approved" {
		return strVal(lastReview["id"]), true
	}
	return "", false
}

func cmdPublishRelease(state, input map[string]any, actor any) error {
	rel, err := findByID(state["releases"], strVal(input["id"]))
	if err != nil {
		return err
	}
	if strVal(rel["publishedAt"]) != "" {
		return errBadInput("Release already published")
	}
	relID := strVal(rel["id"])

	var buildIDs []any
	for _, b := range sliceOfMaps(state["builds"]) {
		if strVal(b["releaseId"]) == relID {
			buildIDs = append(buildIDs, strVal(b["id"]))
		}
	}
	if len(buildIDs) == 0 {
		return errBadInput("Register a build or prototype with approved credits")
	}
	if strVal(rel["criteria"]) == "" {
		return errBadInput("Specify release acceptance criteria")
	}
	valIDs, _ := strSlice(rel["validationIds"])
	if len(valIDs) == 0 {
		return errBadInput("Select validation evidence")
	}
	artVerIDs, _ := strSlice(rel["artifactVersionIds"])
	for _, vid := range valIDs {
		v, err := findByID(state["validations"], vid)
		if err != nil {
			return err
		}
		if strVal(v["result"]) != "passed" {
			return errBadInput("Selected validation has not passed")
		}
		if vRel := strVal(v["releaseId"]); vRel != "" && vRel != relID {
			return errBadInput("Validation belongs to another release")
		}
		if vArt := strVal(v["artifactVersionId"]); vArt != "" && !slices.Contains(artVerIDs, vArt) {
			return errBadInput("Validation refers to a different artifact version")
		}
	}

	reviewIDs := make([]any, 0, len(artVerIDs))
	for _, avid := range artVerIDs {
		revID, ok := isArtifactVersionApproved(state, avid)
		if !ok {
			return errBadInput("Artifact version requires approval")
		}
		reviewIDs = append(reviewIDs, revID)
	}

	explicitPTs, _ := strSlice(rel["platformTargetIds"])
	var linkedTargetIDs []any
	for _, target := range sliceOfMaps(state["platformTargets"]) {
		if boolVal(target["archived"]) {
			continue
		}
		tid := strVal(target["id"])
		if strVal(target["releaseId"]) == relID || slices.Contains(explicitPTs, tid) {
			evalRes := EvaluateTargetReadiness(target)
			if len(evalRes.MissingChecks) > 0 {
				return errBadInput(fmt.Sprintf("Platform compliance checklist incomplete: %s (%s)", strVal(target["name"]), strVal(evalRes.MissingChecks[0]["title"])))
			}
			if len(evalRes.MissingAssets) > 0 {
				return errBadInput(fmt.Sprintf("Storefront assets incomplete: %s (%s)", strVal(target["name"]), strVal(evalRes.MissingAssets[0]["name"])))
			}
			linkedTargetIDs = append(linkedTargetIDs, tid)
		}
	}
	if linkedTargetIDs == nil {
		linkedTargetIDs = []any{}
	}

	rel["publishedAt"] = nowISO()
	rel["publishedBy"] = actor
	rel["buildIds"] = buildIDs
	rel["platformTargetIds"] = linkedTargetIDs
	rel["reviewIds"] = reviewIDs
	return nil
}

func cmdTask(state, input map[string]any) error {
	var existing map[string]any
	if id := strVal(input["id"]); id != "" {
		var err error
		existing, err = findByID(state["tasks"], id)
		if err != nil {
			return err
		}
	}

	assigneeID, err := refID(state["participants"], input["assigneeId"])
	if err != nil {
		return err
	}
	if assigneeID != nil {
		p, err := findByID(state["participants"], strVal(assigneeID))
		if err != nil {
			return err
		}
		if strVal(p["endedAt"]) != "" && (existing == nil || strVal(existing["assigneeId"]) != strVal(assigneeID)) {
			return errBadInput("Participant is inactive")
		}
	}

	deps, err := refIDs(state["tasks"], input["dependencies"])
	if err != nil {
		return err
	}
	taskID := strVal(input["id"])
	if taskID == "" {
		taskID = NewUUID()
	}

	var visit func(dep string, seen map[string]bool) error
	visit = func(dep string, seen map[string]bool) error {
		if dep == taskID {
			return errBadInput("Task dependency cycle")
		}
		if seen[dep] {
			return nil
		}
		seen[dep] = true
		depTask, err := findByID(state["tasks"], dep)
		if err != nil {
			return err
		}
		nextDeps, _ := strSlice(depTask["dependencies"])
		for _, next := range nextDeps {
			if err := visit(next, seen); err != nil {
				return err
			}
		}
		return nil
	}
	for _, dep := range deps {
		if err := visit(strVal(dep), make(map[string]bool)); err != nil {
			return err
		}
	}

	priority := strVal(input["priority"])
	if priority == "" {
		priority = "normal"
	}
	taskType := strVal(input["type"])
	if taskType == "" {
		taskType = "task"
	}
	if !slices.Contains([]string{"low", "normal", "high", "urgent"}, priority) ||
		!slices.Contains([]string{"task", "bug", "feature"}, taskType) {
		return errBadInput("Invalid task priority or type")
	}

	title, err := SafeText(strVal(input["title"]), 500)
	if err != nil {
		return err
	}
	criteria := ""
	if c := strVal(input["criteria"]); c != "" {
		criteria, err = SafeText(c, 10000)
		if err != nil {
			return err
		}
	}
	discipline := ""
	if d := strVal(input["discipline"]); d != "" {
		discipline, err = SafeText(d, 100)
		if err != nil {
			return err
		}
	}

	fbPhase := ""
	fbStage := ""
	if existing != nil {
		fbPhase = strVal(existing["phase"])
		fbStage = strVal(existing["stage"])
	}
	ph, err := validatePhase(strVal(input["phase"]), fbPhase)
	if err != nil {
		return err
	}
	stVal, err := validateStage(strVal(input["stage"]), fbStage)
	if err != nil {
		return err
	}

	var toolURI any
	if rawTU, has := input["toolUri"]; has {
		if s := strVal(rawTU); s != "" {
			toolURI, err = SafeURI(s)
			if err != nil {
				return err
			}
		}
	} else if existing != nil {
		toolURI = existing["toolUri"]
	}

	dueDate, err := parseOptionalDate(input["dueDate"])
	if err != nil {
		return err
	}
	blockedReason := ""
	if br := strVal(input["blockedReason"]); br != "" {
		blockedReason, err = SafeText(br, 2000)
		if err != nil {
			return err
		}
	}
	milestoneID, err := refID(state["milestones"], input["milestoneId"])
	if err != nil {
		return err
	}
	cycleID, err := refID(state["cycles"], input["cycleId"])
	if err != nil {
		return err
	}
	docIDs, err := refIDs(state["documents"], input["documentIds"])
	if err != nil {
		return err
	}

	values := map[string]any{
		"title":         title,
		"criteria":      criteria,
		"discipline":    discipline,
		"phase":         ph,
		"stage":         stVal,
		"toolUri":       toolURI,
		"assigneeId":    assigneeID,
		"priority":      priority,
		"type":          taskType,
		"dueDate":       dueDate,
		"blockedReason": blockedReason,
		"dependencies":  deps,
		"milestoneId":   milestoneID,
		"cycleId":       cycleID,
		"documentIds":   docIDs,
	}
	if existing != nil {
		for k, v := range values {
			existing[k] = v
		}
		return nil
	}
	values["id"] = taskID
	values["status"] = "backlog"
	appendSliceMap(state, "tasks", values)
	return nil
}

func cmdTransition(state, input map[string]any) error {
	status := strVal(input["status"])
	if !slices.Contains(States, status) {
		return errBadInput("Invalid task state")
	}
	task, err := findByID(state["tasks"], strVal(input["id"]))
	if err != nil {
		return err
	}
	if status == "done" {
		if strVal(task["blockedReason"]) != "" {
			return errBadInput("Resolve task blockers first")
		}
		deps, _ := strSlice(task["dependencies"])
		for _, dep := range deps {
			depTask, err := findByID(state["tasks"], dep)
			if err != nil {
				return err
			}
			if strVal(depTask["status"]) != "done" {
				return errBadInput("Resolve task blockers first")
			}
		}
	}
	task["status"] = status
	return nil
}

func buildCreditEntries(state map[string]any, rawIDs any) ([]CanonicalCreditsEntry, error) {
	ids, ok := strSlice(rawIDs)
	if !ok || len(ids) == 0 {
		return nil, errBadInput("Select contributors")
	}
	seen := make(map[string]bool, len(ids))
	var entries []CanonicalCreditsEntry
	for _, pid := range ids {
		if seen[pid] {
			continue
		}
		seen[pid] = true
		person, err := findByID(state["participants"], pid)
		if err != nil {
			return nil, err
		}
		section := strVal(person["team"])
		if tid := strVal(person["teamId"]); tid != "" {
			if t, tErr := findByID(state["teams"], tid); tErr == nil && strVal(t["name"]) != "" {
				section = strVal(t["name"])
			}
		}
		if section == "" {
			section = "Team"
		}
		ord, _ := intVal(person["order"])
		roles, _ := strSlice(person["roles"])
		for _, r := range roles {
			entries = append(entries, CanonicalCreditsEntry{
				ParticipantID: pid,
				Name:          strVal(person["name"]),
				Role:          r,
				Section:       section,
				Order:         ord,
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Order != entries[j].Order {
			return entries[i].Order < entries[j].Order
		}
		if entries[i].Section != entries[j].Section {
			return entries[i].Section < entries[j].Section
		}
		if entries[i].ParticipantID != entries[j].ParticipantID {
			return entries[i].ParticipantID < entries[j].ParticipantID
		}
		return entries[i].Role < entries[j].Role
	})
	return entries, nil
}

func manifestToMap(m CanonicalCreditsManifest) (map[string]any, string, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, "", err
	}
	checksum := SHA256Hex(b)
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, "", err
	}
	return out, checksum, nil
}

func cmdGenerateCredits(state, input map[string]any) error {
	rel, err := findByID(state["releases"], strVal(input["releaseId"]))
	if err != nil {
		return err
	}
	entries, err := buildCreditEntries(state, input["participantIds"])
	if err != nil {
		return err
	}
	relID := strVal(rel["id"])
	manifest := CanonicalCreditsManifest{
		SchemaVersion: 1,
		ProjectID:     strVal(state["projectId"]),
		ReleaseID:     relID,
		Title:         strVal(rel["name"]),
		Entries:       entries,
	}
	mMap, checksum, err := manifestToMap(manifest)
	if err != nil {
		return err
	}
	for _, row := range sliceOfMaps(state["credits"]) {
		if strVal(row["releaseId"]) == relID && strVal(row["checksum"]) == checksum {
			return nil
		}
	}
	appendSliceMap(state, "credits", map[string]any{
		"id":        NewUUID(),
		"releaseId": relID,
		"status":    "draft",
		"checksum":  checksum,
		"manifest":  mMap,
		"createdAt": nowISO(),
	})
	return nil
}

func cmdEditCredits(state, input map[string]any) error {
	snap, err := findByID(state["credits"], strVal(input["id"]))
	if err != nil {
		return err
	}
	if strVal(snap["status"]) != "draft" {
		return errBadInput("Approved credits are immutable")
	}
	rawEntries, ok := input["entries"].([]any)
	if !ok || len(rawEntries) == 0 || len(rawEntries) > 5000 {
		return errBadInput("Invalid credit entries")
	}
	if _, err := SafeText(strVal(input["reason"]), 500); err != nil {
		return err
	}

	entries := make([]CanonicalCreditsEntry, 0, len(rawEntries))
	seenKeys := make(map[string]bool, len(rawEntries))
	for idx, item := range rawEntries {
		em, ok := item.(map[string]any)
		if !ok {
			return errBadInput("Invalid credit entries")
		}
		person, err := findByID(state["participants"], strVal(em["participantId"]))
		if err != nil {
			return err
		}
		pid := strVal(person["id"])
		name, err := SafeText(strVal(em["name"]), 255)
		if err != nil {
			return err
		}
		role, err := SafeText(strVal(em["role"]), 120)
		if err != nil {
			return err
		}
		section, err := SafeText(strVal(em["section"]), 120)
		if err != nil {
			return err
		}
		key := pid + "\x00" + role + "\x00" + section
		if seenKeys[key] {
			return errBadInput("Duplicate credit")
		}
		seenKeys[key] = true
		entries = append(entries, CanonicalCreditsEntry{
			ParticipantID: pid,
			Name:          name,
			Role:          role,
			Section:       section,
			Order:         idx,
		})
	}

	curManifest := decodeCreditsManifest(snap["manifest"].(map[string]any))
	curManifest.Entries = entries
	mMap, checksum, err := manifestToMap(curManifest)
	if err != nil {
		return err
	}
	snap["manifest"] = mMap
	snap["checksum"] = checksum
	return nil
}

func cmdReviseCredits(state, input map[string]any) error {
	snap, err := findByID(state["credits"], strVal(input["id"]))
	if err != nil {
		return err
	}
	if strVal(snap["status"]) != "approved" {
		return errBadInput("Select approved credits")
	}
	appendSliceMap(state, "credits", map[string]any{
		"id":         NewUUID(),
		"releaseId":  snap["releaseId"],
		"status":     "draft",
		"checksum":   snap["checksum"],
		"manifest":   cloneValue(snap["manifest"]),
		"previousId": snap["id"],
		"createdAt":  nowISO(),
	})
	return nil
}

func cmdApproveCredits(state, input map[string]any, actor any) error {
	snap, err := findByID(state["credits"], strVal(input["id"]))
	if err != nil {
		return err
	}
	if strVal(snap["status"]) != "draft" {
		return errBadInput("Credits already approved")
	}
	snap["status"] = "approved"
	snap["approvedBy"] = actor
	snap["approvedAt"] = nowISO()
	return nil
}

func cmdBuild(state, input map[string]any) error {
	rel, err := findByID(state["releases"], strVal(input["releaseId"]))
	if err != nil {
		return err
	}
	if strVal(rel["publishedAt"]) != "" {
		return errBadInput("Published release is immutable")
	}
	relID := strVal(rel["id"])

	snap, err := findByID(state["credits"], strVal(input["snapshotId"]))
	if err != nil {
		return err
	}
	if strVal(snap["status"]) != "approved" || strVal(snap["releaseId"]) != relID {
		return errBadInput("Select approved credits for this release")
	}

	name, err := SafeText(strVal(input["name"]), 500)
	if err != nil {
		return err
	}
	var buildURI any
	if u := strVal(input["uri"]); u != "" {
		c, uErr := SafeURI(u)
		if uErr != nil {
			return errBadInput("Invalid build location")
		}
		buildURI = c
	}

	var artChecksum any
	if cs := strVal(input["checksum"]); cs != "" {
		if !reSHA256Hex.MatchString(cs) {
			return errBadInput("Invalid build checksum")
		}
		artChecksum = strings.ToLower(cs)
	}

	var vcsRevision any
	if vr := strVal(input["vcsRevision"]); vr != "" {
		vcsRevision, err = SafeText(vr, 200)
		if err != nil {
			return err
		}
	}

	snapID := strVal(snap["id"])
	for _, existing := range sliceOfMaps(state["builds"]) {
		if strVal(existing["releaseId"]) == relID && strVal(existing["name"]) == name {
			if strVal(existing["snapshotId"]) != snapID ||
				strVal(existing["uri"]) != strVal(buildURI) ||
				strings.ToLower(strVal(existing["artifactChecksum"])) != strVal(artChecksum) {
				return errBadInput("Build identifier already has different content or credits")
			}
			return nil
		}
	}

	appendSliceMap(state, "builds", map[string]any{
		"id":               NewUUID(),
		"name":             name,
		"releaseId":        relID,
		"snapshotId":       snapID,
		"checksum":         snap["checksum"],
		"artifactChecksum": artChecksum,
		"uri":              buildURI,
		"vcsRevision":      vcsRevision,
	})
	return nil
}

func cmdIntegrationEvent(state, input map[string]any, actor any) error {
	source, err := SafeText(strVal(input["source"]), 50)
	if err != nil || !slices.Contains(Tools, source) {
		return errBadInput("Unsupported source")
	}
	eventID, err := SafeText(strVal(input["eventId"]), 200)
	if err != nil {
		return err
	}

	canonBytes, err := json.Marshal(canonicalValue(map[string]any{
		"type":    input["type"],
		"payload": input["payload"],
	}))
	if err != nil {
		return err
	}
	fingerprint := SHA256Hex(canonBytes)

	for _, existing := range sliceOfMaps(state["integrationEvents"]) {
		if strVal(existing["source"]) == source && strVal(existing["eventId"]) == eventID {
			if strVal(existing["fingerprint"]) != fingerprint {
				return errBadInput("Event ID reused with different content")
			}
			return nil
		}
	}

	evType := strVal(input["type"])
	if !slices.Contains([]string{"artifact", "artifactVersion", "artifactStage", "build", "validation", "vcsStatus"}, evType) {
		return errBadInput("Unsupported event type")
	}

	rawPayload, ok := input["payload"].(map[string]any)
	if !ok {
		return errBadInput("Invalid event payload")
	}
	payload := make(map[string]any, len(rawPayload)+1)
	for k, v := range rawPayload {
		payload[k] = v
	}
	payload["source"] = source

	switch evType {
	case "artifact":
		extID, err := SafeText(strVal(payload["externalId"]), 500)
		if err != nil {
			return err
		}
		for _, r := range sliceOfMaps(state["artifacts"]) {
			if strVal(r["source"]) == source && strVal(r["externalId"]) == extID {
				payload["id"] = r["id"]
				break
			}
		}
		if err := cmdArtifact(state, payload); err != nil {
			return err
		}
		var updated map[string]any
		if pid := strVal(payload["id"]); pid != "" {
			updated, _ = findByID(state["artifacts"], pid)
		} else {
			arts := sliceOfMaps(state["artifacts"])
			if len(arts) > 0 {
				updated = arts[len(arts)-1]
			}
		}
		if updated != nil {
			updated["source"] = source
			updated["externalId"] = extID
		}
	case "artifactVersion":
		if err := cmdArtifactVersion(state, payload); err != nil {
			return err
		}
	case "artifactStage":
		if err := cmdArtifactStage(state, payload); err != nil {
			return err
		}
	case "build":
		if err := cmdBuild(state, payload); err != nil {
			return err
		}
	case "validation":
		if err := cmdValidation(state, payload, actor); err != nil {
			return err
		}
	case "vcsStatus":
		if err := cmdVCSStatus(state, payload, actor); err != nil {
			return err
		}
	}

	appendSliceMap(state, "integrationEvents", map[string]any{
		"id":          NewUUID(),
		"source":      source,
		"eventId":     eventID,
		"fingerprint": fingerprint,
		"type":        evType,
		"at":          nowISO(),
	})
	return nil
}

func cmdAccess(ctx context.Context, st store.Store, state map[string]any, subj SubjectProfile, input map[string]any, actor any) error {
	role := strVal(input["role"])
	if !slices.Contains([]string{"manager", "editor", "reader", "remove"}, role) {
		return errBadInput("Invalid role")
	}

	uid, hasUID := intVal(input["userId"])
	email := strings.TrimSpace(strVal(input["email"]))
	sub := strings.TrimSpace(strVal(input["sub"]))

	if hasUID && uid > 0 {
		if uid == 2 {
			return errBadInput("Invalid member")
		}
		if ownerInt, ok := intVal(state["ownerId"]); ok && ownerInt == uid {
			return errBadInput("Invalid member")
		}
		var userName string
		if st != nil {
			target, err := st.GetWikiUserByID(ctx, uid)
			if err != nil {
				return err
			}
			if target != nil {
				if !target.IsActive || target.ID == 2 {
					return errBadInput("Invalid member")
				}
				userName = target.Name
				if email == "" {
					email = target.Email
				}
				if sub == "" {
					sub = target.ProviderID
				}
			} else if !subj.Delegated {
				return errBadInput("Invalid member")
			}
		}
		if userName == "" {
			userName = strVal(input["userName"])
		}

		filtered := make([]any, 0)
		for _, entry := range sliceOfMaps(state["access"]) {
			if eUID, ok := intVal(entry["userId"]); ok && eUID == uid {
				continue
			}
			filtered = append(filtered, entry)
		}
		if role != "remove" {
			rec := map[string]any{"userId": uid, "role": role}
			if userName != "" {
				rec["name"] = userName
			}
			if email != "" {
				rec["email"] = email
			}
			if sub != "" {
				rec["sub"] = sub
			}
			filtered = append(filtered, rec)
		}
		state["access"] = filtered
		appendSliceMap(state, "history", map[string]any{
			"action": "access",
			"actor":  actor,
			"at":     nowISO(),
			"userId": uid,
			"role":   role,
		})
		return nil
	}

	// Standalone Authentik email / sub access grant when Wiki numeric userId is not used
	if email == "" && sub == "" {
		return errBadInput("Invalid member")
	}
	if (sub != "" && strings.EqualFold(strVal(state["ownerId"]), sub)) ||
		(email != "" && strings.EqualFold(strVal(state["ownerId"]), email)) {
		return errBadInput("Invalid member")
	}
	filtered := make([]any, 0)
	for _, entry := range sliceOfMaps(state["access"]) {
		if (sub != "" && strings.EqualFold(strVal(entry["sub"]), sub)) ||
			(email != "" && strings.EqualFold(strVal(entry["email"]), email)) {
			continue
		}
		filtered = append(filtered, entry)
	}
	if role != "remove" {
		rec := map[string]any{"role": role}
		if email != "" {
			rec["email"] = email
		}
		if sub != "" {
			rec["sub"] = sub
		}
		if n := strVal(input["name"]); n != "" {
			rec["name"] = n
		}
		filtered = append(filtered, rec)
	}
	state["access"] = filtered
	appendSliceMap(state, "history", map[string]any{
		"action": "access",
		"actor":  actor,
		"at":     nowISO(),
		"email":  email,
		"sub":    sub,
		"role":   role,
	})
	return nil
}

func cmdGroupAccess(ctx context.Context, st store.Store, state map[string]any, subj SubjectProfile, input map[string]any, actor any) error {
	role := strVal(input["role"])
	if !slices.Contains([]string{"manager", "editor", "reader", "remove"}, role) {
		return errBadInput("Invalid Authentik group")
	}

	gid, _ := intVal(input["groupId"])
	gName := strings.TrimSpace(strVal(input["groupName"]))

	if gid > 0 && st != nil {
		g, err := st.GetWikiGroupByID(ctx, gid)
		if err != nil {
			return err
		}
		if g != nil {
			gName = g.Name
		} else if !subj.Delegated && gName == "" {
			return errBadInput("Invalid Authentik group")
		}
	} else if gid == 0 && gName != "" && st != nil {
		if g, err := st.GetWikiGroupByName(ctx, gName); err == nil && g != nil {
			gid = g.ID
			gName = g.Name
		}
	}

	if gid <= 0 && gName == "" {
		return errBadInput("Invalid Authentik group")
	}
	if gName != "" {
		var err error
		gName, err = SafeText(gName, 255)
		if err != nil {
			return errBadInput("Invalid Authentik group")
		}
	}

	var loreActs []string
	if role != "remove" {
		rawSlice, hasExplicit := strSlice(input["loreActions"])
		if _, hasKey := input["loreActions"]; hasKey && !hasExplicit {
			return errBadInput("Invalid Lore actions")
		}
		var err error
		loreActs, err = NormalizeLoreActions(rawSlice, hasExplicit, role)
		if err != nil {
			return err
		}
	}
	canCreateRepo := role != "remove" && boolVal(input["loreCanCreateRepo"])

	filtered := make([]any, 0)
	for _, entry := range sliceOfMaps(state["groupAccess"]) {
		eGID, _ := intVal(entry["groupId"])
		eName := strVal(entry["groupName"])
		if (gid > 0 && eGID == gid) || (gName != "" && eName != "" && strings.EqualFold(eName, gName)) {
			continue
		}
		filtered = append(filtered, entry)
	}
	if role != "remove" {
		filtered = append(filtered, map[string]any{
			"id":                NewUUID(),
			"groupId":           gid,
			"groupName":         gName,
			"role":              role,
			"loreActions":       toAnyStrings(loreActs),
			"loreCanCreateRepo": canCreateRepo,
		})
	}
	state["groupAccess"] = filtered

	appendSliceMap(state, "history", map[string]any{
		"id":                NewUUID(),
		"action":            "groupAccess",
		"actor":             actor,
		"at":                nowISO(),
		"groupId":           gid,
		"groupName":         gName,
		"role":              role,
		"loreActions":       toAnyStrings(loreActs),
		"loreCanCreateRepo": canCreateRepo,
	})
	return nil
}

func cmdIntegrationAccess(ctx context.Context, st store.Store, state map[string]any, subj SubjectProfile, input map[string]any, actor any) error {
	role := strVal(input["role"])
	if !slices.Contains([]string{"reader", "editor", "remove"}, role) {
		return errBadInput("Invalid integration group")
	}
	gid, _ := intVal(input["groupId"])
	gName := strings.TrimSpace(strVal(input["groupName"]))

	if gid > 0 && st != nil {
		g, err := st.GetWikiGroupByID(ctx, gid)
		if err != nil {
			return err
		}
		if g != nil {
			gName = g.Name
		} else if !subj.Delegated && gName == "" {
			return errBadInput("Invalid integration group")
		}
	}
	if gid <= 0 && gName == "" {
		return errBadInput("Invalid integration group")
	}

	filtered := make([]any, 0)
	for _, entry := range sliceOfMaps(state["integrationAccess"]) {
		eGID, _ := intVal(entry["groupId"])
		eName := strVal(entry["groupName"])
		if (gid > 0 && eGID == gid) || (gName != "" && eName != "" && strings.EqualFold(eName, gName)) {
			continue
		}
		filtered = append(filtered, entry)
	}
	if role != "remove" {
		rec := map[string]any{
			"id":      NewUUID(),
			"groupId": gid,
			"role":    role,
		}
		if gName != "" {
			rec["groupName"] = gName
		}
		filtered = append(filtered, rec)
	}
	state["integrationAccess"] = filtered
	appendSliceMap(state, "history", map[string]any{
		"action":  "integrationAccess",
		"actor":   actor,
		"at":      nowISO(),
		"groupId": gid,
		"role":    role,
	})
	return nil
}

func cmdLinkDocument(state map[string]any, command string, input map[string]any, actor any) error {
	pageID, ok := intVal(input["pageId"])
	if !ok || pageID <= 0 {
		return errBadInput("Page unavailable")
	}
	revision := strVal(input["revision"])
	alreadyLinked := false
	for _, link := range sliceOfMaps(state["documents"]) {
		if lp, ok := intVal(link["pageId"]); ok && lp == pageID {
			alreadyLinked = true
			break
		}
	}
	if !alreadyLinked {
		doc := map[string]any{
			"id":       NewUUID(),
			"pageId":   pageID,
			"revision": revision,
		}
		if tpl := strVal(input["template"]); tpl != "" {
			doc["template"] = tpl
			tv, ok := intVal(input["templateVersion"])
			if !ok || tv <= 0 {
				tv = 1
			}
			doc["templateVersion"] = tv
		}
		appendSliceMap(state, "documents", doc)
	}
	histAction := "document"
	if act := strVal(input["historyAction"]); act != "" {
		histAction = act
	} else if command == "createDocument" || strVal(input["template"]) != "" {
		histAction = "createDocument"
	}
	hist := map[string]any{
		"action": histAction,
		"actor":  actor,
		"at":     nowISO(),
	}
	if tpl := strVal(input["template"]); tpl != "" {
		hist["template"] = tpl
	}
	appendSliceMap(state, "history", hist)
	return nil
}

func cmdRefreshDocument(state, input map[string]any, actor any) error {
	link, err := findByID(state["documents"], strVal(input["id"]))
	if err != nil {
		return err
	}
	newRev := strVal(input["revision"])
	if newRev == "" {
		return errBadInput("Page unavailable")
	}
	prevRev := link["revision"]
	link["revision"] = newRev
	appendSliceMap(state, "history", map[string]any{
		"action":           "refreshDocument",
		"actor":            actor,
		"at":               nowISO(),
		"documentId":       link["id"],
		"previousRevision": prevRev,
		"revision":         newRev,
	})
	return nil
}
