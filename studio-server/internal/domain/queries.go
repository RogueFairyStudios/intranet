package domain

import (
	"context"
	"slices"
	"sort"
	"strings"

	"github.com/roguefairystudios/intranet/studio-server/internal/store"
)

// SubjectIdentity is the public summary of a resolved user in query responses.
type SubjectIdentity struct {
	Subject    string   `json:"sub"`
	WikiUserID int      `json:"wikiUserId,omitempty"`
	Username   string   `json:"username,omitempty"`
	Name       string   `json:"name,omitempty"`
	Email      string   `json:"email,omitempty"`
	Groups     []string `json:"groups,omitempty"`
	IsAdmin    bool     `json:"isAdmin"`
}

func toSubjectIdentity(s SubjectProfile) SubjectIdentity {
	groups := slices.Clone(s.GroupNames)
	if groups == nil {
		groups = []string{}
	}
	return SubjectIdentity{
		Subject:    s.Subject,
		WikiUserID: s.WikiUserID,
		Username:   s.Username,
		Name:       s.Name,
		Email:      s.Email,
		Groups:     groups,
		IsAdmin:    s.IsAdmin,
	}
}

// RepositorySummary describes a project VCS repository along with effective access and clone command.
type RepositorySummary struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Slug           string            `json:"slug"`
	System         string            `json:"system"`
	RemoteURI      string            `json:"remoteUri"`
	LoreProject    string            `json:"loreProject,omitempty"`
	LoreRepo       string            `json:"loreRepo,omitempty"`
	LoreFullName   string            `json:"loreFullName,omitempty"`
	DefaultBranch  string            `json:"defaultBranch"`
	WorkspacePath  string            `json:"workspacePath"`
	AccessLevel    string            `json:"accessLevel"`
	CanRead        bool              `json:"canRead"`
	CanWrite       bool              `json:"canWrite"`
	CloneCommand   []string          `json:"cloneCommand"`
	Branch         string            `json:"branch,omitempty"`
	LatestRevision string            `json:"latestRevision,omitempty"`
	Clean          bool              `json:"clean"`
	LockCount      int               `json:"lockCount"`
	Locks          []RepoLockSummary `json:"locks"`
}

// RepoLockSummary represents an active binary file lock (e.g. Lore lock on a .kra or .tscn file).
type RepoLockSummary struct {
	Path        string `json:"path"`
	Owner       string `json:"owner"`
	Branch      string `json:"branch,omitempty"`
	LockedAt    string `json:"lockedAt,omitempty"`
	OwnedByUser bool   `json:"ownedByUser"`
}

// MilestoneSummary represents the active/next milestone on a project.
type MilestoneSummary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Phase   string `json:"phase"`
	DueDate string `json:"dueDate,omitempty"`
}

// CycleSummary represents the active iteration/sprint cycle on a project.
type CycleSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	StartDate string `json:"startDate,omitempty"`
	EndDate   string `json:"endDate,omitempty"`
}

// UserProjectSummary summarizes a Studio project for Project Manager views (Aetherion, Intranet, CLI).
type UserProjectSummary struct {
	ID                    string              `json:"id"`
	Revision              int                 `json:"revision"`
	Slug                  string              `json:"slug"`
	Name                  string              `json:"name"`
	Kind                  string              `json:"kind"`
	Engine                string              `json:"engine"`
	Phase                 string              `json:"phase"`
	Archived              bool                `json:"archived"`
	Role                  string              `json:"role"`
	CanManage             bool                `json:"canManage"`
	CanEdit               bool                `json:"canEdit"`
	ParticipantIDs        []string            `json:"participantIds"`
	ParticipantRoles      []string            `json:"participantRoles"`
	IntranetURL           string              `json:"intranetUrl"`
	GodotURI              string              `json:"godotUri,omitempty"`
	PrimaryWorkspacePath  string              `json:"primaryWorkspacePath"`
	ActiveMilestone       *MilestoneSummary   `json:"activeMilestone,omitempty"`
	ActiveCycle           *CycleSummary       `json:"activeCycle,omitempty"`
	OpenTaskCount         int                 `json:"openTaskCount"`
	AssignedOpenTaskCount int                 `json:"assignedOpenTaskCount"`
	Lore                  EffectiveLoreGrant  `json:"lore"`
	Repositories          []RepositorySummary `json:"repositories"`
}

// UserProjectsResponse is returned by GET /v1/me/projects and GET /v1/users/{id}/projects.
type UserProjectsResponse struct {
	User      SubjectIdentity      `json:"user"`
	ClientApp string               `json:"clientApp"`
	Projects  []UserProjectSummary `json:"projects"`
}

// ProjectQueryOptions customizes project listing per tool (e.g. Aetherion Project Manager).
type ProjectQueryOptions struct {
	ClientApp       string
	IntranetURL     string
	IncludeArchived bool
	IncludeAllKinds bool
}

// QueryUserProjects lists all Studio projects accessible to target, tailored to opts.ClientApp.
// When viewer is non-nil and not an administrator, results are intersected with viewer's visible projects.
func QueryUserProjects(
	ctx context.Context,
	st store.Store,
	policy PolicyConfig,
	target SubjectProfile,
	viewer *SubjectProfile,
	opts ProjectQueryOptions,
) (UserProjectsResponse, error) {
	rows, err := st.ListProjects(ctx)
	if err != nil {
		return UserProjectsResponse{}, err
	}

	clientApp := strings.ToLower(strings.TrimSpace(opts.ClientApp))
	if clientApp == "" {
		clientApp = target.ClientApp
	}
	if clientApp == "" {
		clientApp = "generic"
	}
	intranetBase := strings.TrimRight(opts.IntranetURL, "/")
	if intranetBase == "" {
		intranetBase = "https://docs.roguefairy.net"
	}

	projects := make([]UserProjectSummary, 0)
	for _, row := range rows {
		state, err := ParseState(row.State)
		if err != nil {
			continue
		}
		if viewer != nil && !viewer.IsAdmin && RoleFor(state, *viewer, policy) == "" {
			continue
		}
		role := RoleFor(state, target, policy)
		if role == "" {
			continue
		}
		archived := boolVal(state["archived"])
		if archived && !opts.IncludeArchived {
			continue
		}
		kind := strVal(state["kind"])
		if clientApp == "aetherion" && !opts.IncludeAllKinds && kind != "digital" && kind != "hybrid" {
			continue
		}

		pIDs, pRoles := matchParticipantRecords(ctx, st, state, target)
		loreGrant := EffectiveLoreGrantFor(state, target, policy)
		repos := summarizeRepositories(state, role, loreGrant, target, "")

		openTasks, assignedOpen := countTasks(state, pIDs)
		primaryWS := inferPrimaryWorkspacePath(repos)
		godotURI := ""
		if kind == "digital" || kind == "hybrid" {
			godotURI = "godot://project/" + loreGrant.ProjectSlug
		}

		projects = append(projects, UserProjectSummary{
			ID:                    row.ID,
			Revision:              row.Revision,
			Slug:                  loreGrant.ProjectSlug,
			Name:                  strVal(state["name"]),
			Kind:                  kind,
			Engine:                strVal(state["engine"]),
			Phase:                 strVal(state["phase"]),
			Archived:              archived,
			Role:                  role,
			CanManage:             role == "manager",
			CanEdit:               role == "manager" || role == "editor",
			ParticipantIDs:        pIDs,
			ParticipantRoles:      pRoles,
			IntranetURL:           intranetBase + "/p/studio?project=" + row.ID,
			GodotURI:              godotURI,
			PrimaryWorkspacePath:  primaryWS,
			ActiveMilestone:       pickActiveMilestone(state),
			ActiveCycle:           pickActiveCycle(state),
			OpenTaskCount:         openTasks,
			AssignedOpenTaskCount: assignedOpen,
			Lore:                  loreGrant,
			Repositories:          repos,
		})
	}

	sort.Slice(projects, func(i, j int) bool {
		if (projects[i].AssignedOpenTaskCount > 0) != (projects[j].AssignedOpenTaskCount > 0) {
			return projects[i].AssignedOpenTaskCount > 0
		}
		if (len(projects[i].ParticipantIDs) > 0) != (len(projects[j].ParticipantIDs) > 0) {
			return len(projects[i].ParticipantIDs) > 0
		}
		return strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name)
	})

	return UserProjectsResponse{
		User:      toSubjectIdentity(target),
		ClientApp: clientApp,
		Projects:  projects,
	}, nil
}

// WorkspaceTask describes an actionable Studio task relevant to a user's workspace.
type WorkspaceTask struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Discipline     string `json:"discipline,omitempty"`
	Phase          string `json:"phase,omitempty"`
	Stage          string `json:"stage,omitempty"`
	Status         string `json:"status"`
	Priority       string `json:"priority"`
	Type           string `json:"type"`
	DueDate        string `json:"dueDate,omitempty"`
	ToolURI        string `json:"toolUri,omitempty"`
	AssigneeID     string `json:"assigneeId,omitempty"`
	AssignedToUser bool   `json:"assignedToUser"`
}

// WorkspaceArtifact describes a pipeline artifact that a user works on in tools like Geppetto, Goethe, or Aetherion.
type WorkspaceArtifact struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Track           string `json:"track"`
	Stage           string `json:"stage"`
	Tool            string `json:"tool"`
	ToolURI         string `json:"toolUri,omitempty"`
	RepoID          string `json:"repoId,omitempty"`
	RepoSlug        string `json:"repoSlug,omitempty"`
	LoreFullName    string `json:"loreFullName,omitempty"`
	AssetPath       string `json:"assetPath,omitempty"`
	TaskID          string `json:"taskId,omitempty"`
	LastVCSRevision string `json:"lastVcsRevision,omitempty"`
	LockedBy        string `json:"lockedBy,omitempty"`
}

// WorkspaceStorefrontAsset describes a pending storefront graphical slot (e.g. Steam Capsule, App Icon).
type WorkspaceStorefrontAsset struct {
	TargetID   string `json:"targetId"`
	TargetName string `json:"targetName"`
	Storefront string `json:"storefront"`
	SlotID     string `json:"slotId"`
	Name       string `json:"name"`
	Dimensions string `json:"dimensions"`
	Format     string `json:"format"`
	Spec       string `json:"spec,omitempty"`
	Status     string `json:"status"`
	Required   bool   `json:"required"`
	ToolURI    string `json:"toolUri,omitempty"`
}

// ProjectWorkspace bundles the VCS repositories, tasks, artifacts, and storefront slots a user works on in a project.
type ProjectWorkspace struct {
	ProjectID        string                     `json:"projectId"`
	ProjectSlug      string                     `json:"projectSlug"`
	ProjectName      string                     `json:"projectName"`
	Kind             string                     `json:"kind"`
	Engine           string                     `json:"engine"`
	Phase            string                     `json:"phase"`
	Role             string                     `json:"role"`
	CanWrite         bool                       `json:"canWrite"`
	ParticipantIDs   []string                   `json:"participantIds"`
	ParticipantRoles []string                   `json:"participantRoles"`
	IntranetURL      string                     `json:"intranetUrl"`
	Lore             EffectiveLoreGrant         `json:"lore"`
	Repositories     []RepositorySummary        `json:"repositories"`
	Tasks            []WorkspaceTask            `json:"tasks"`
	Artifacts        []WorkspaceArtifact        `json:"artifacts"`
	StorefrontAssets []WorkspaceStorefrontAsset `json:"storefrontAssets"`
	HasAssignedWork  bool                       `json:"hasAssignedWork"`
	relevance        int
}

// UserWorkspacesResponse answers cross-tool workspace & Lore repository queries (e.g. Geppetto, Lore CLI, Aetherion).
type UserWorkspacesResponse struct {
	User       SubjectIdentity    `json:"user"`
	ClientApp  string             `json:"clientApp"`
	VCSFilter  string             `json:"vcsFilter,omitempty"`
	Track      string             `json:"track,omitempty"`
	Tool       string             `json:"tool,omitempty"`
	Workspaces []ProjectWorkspace `json:"workspaces"`
}

// WorkspaceQueryOptions filters workspaces by tool, VCS system (e.g. "lore"), pipeline track (e.g. "art_2d"), and write permission.
type WorkspaceQueryOptions struct {
	ClientApp         string
	IntranetURL       string
	VCS               string // "lore", "git", "urc", or ""
	Track             string // e.g. "art_2d", "narrative"
	Tool              string // e.g. "geppetto", "goethe", "godot"
	OnlyWritable      bool
	OnlyActionable    bool
	IncludeEmptyRepos bool
	IncludeArchived   bool
}

// QueryUserWorkspaces resolves which projects, Lore/Git repositories, pipeline artifacts, tasks, and storefront slots
// target should work on (answering queries such as "Which Lore repositories should User Y push art to?").
func QueryUserWorkspaces(
	ctx context.Context,
	st store.Store,
	policy PolicyConfig,
	target SubjectProfile,
	viewer *SubjectProfile,
	opts WorkspaceQueryOptions,
) (UserWorkspacesResponse, error) {
	rows, err := st.ListProjects(ctx)
	if err != nil {
		return UserWorkspacesResponse{}, err
	}

	clientApp := strings.ToLower(strings.TrimSpace(opts.ClientApp))
	if clientApp == "" {
		clientApp = target.ClientApp
	}
	if clientApp == "" {
		clientApp = "generic"
	}
	vcsFilter := strings.ToLower(strings.TrimSpace(opts.VCS))
	effTrack, effTool := resolveToolTrackDefaults(clientApp, opts.Track, opts.Tool)

	intranetBase := strings.TrimRight(opts.IntranetURL, "/")
	if intranetBase == "" {
		intranetBase = "https://docs.roguefairy.net"
	}

	workspaces := make([]ProjectWorkspace, 0)
	for _, row := range rows {
		state, err := ParseState(row.State)
		if err != nil {
			continue
		}
		if viewer != nil && !viewer.IsAdmin && RoleFor(state, *viewer, policy) == "" {
			continue
		}
		role := RoleFor(state, target, policy)
		if role == "" {
			continue
		}
		if boolVal(state["archived"]) && !opts.IncludeArchived {
			continue
		}

		loreGrant := EffectiveLoreGrantFor(state, target, policy)
		canEditStudio := role == "manager" || role == "editor"
		canWriteEffective := canEditStudio
		if vcsFilter == "lore" {
			canWriteEffective = loreGrant.CanWrite
		}
		if opts.OnlyWritable && !canWriteEffective {
			continue
		}

		repos := summarizeRepositories(state, role, loreGrant, target, vcsFilter)
		if opts.OnlyWritable {
			writableRepos := make([]RepositorySummary, 0, len(repos))
			for _, r := range repos {
				if r.CanWrite {
					writableRepos = append(writableRepos, r)
				}
			}
			repos = writableRepos
		}
		if vcsFilter != "" && len(repos) == 0 && !opts.IncludeEmptyRepos {
			continue
		}

		pIDs, pRoles := matchParticipantRecords(ctx, st, state, target)
		tasks := collectWorkspaceTasks(state, pIDs, effTrack, effTool)
		artifacts := collectWorkspaceArtifacts(state, repos, effTrack, effTool)
		sfAssets := collectStorefrontArtSlots(state, clientApp, effTrack)

		assignedCount := 0
		for _, t := range tasks {
			if t.AssignedToUser {
				assignedCount++
			}
		}
		hasAssigned := assignedCount > 0 || len(pIDs) > 0
		if opts.OnlyActionable && assignedCount == 0 && len(artifacts) == 0 && len(sfAssets) == 0 {
			continue
		}

		rel := 0
		if assignedCount > 0 {
			rel += 100 + assignedCount*10
		}
		if len(pIDs) > 0 {
			rel += 40
		}
		if len(artifacts) > 0 {
			rel += 20 + len(artifacts)
		}
		if len(repos) > 0 {
			rel += 10
		}

		workspaces = append(workspaces, ProjectWorkspace{
			ProjectID:        row.ID,
			ProjectSlug:      loreGrant.ProjectSlug,
			ProjectName:      strVal(state["name"]),
			Kind:             strVal(state["kind"]),
			Engine:           strVal(state["engine"]),
			Phase:            strVal(state["phase"]),
			Role:             role,
			CanWrite:         canWriteEffective,
			ParticipantIDs:   pIDs,
			ParticipantRoles: pRoles,
			IntranetURL:      intranetBase + "/p/studio?project=" + row.ID,
			Lore:             loreGrant,
			Repositories:     repos,
			Tasks:            tasks,
			Artifacts:        artifacts,
			StorefrontAssets: sfAssets,
			HasAssignedWork:  hasAssigned,
			relevance:        rel,
		})
	}

	sort.Slice(workspaces, func(i, j int) bool {
		if workspaces[i].relevance != workspaces[j].relevance {
			return workspaces[i].relevance > workspaces[j].relevance
		}
		return strings.ToLower(workspaces[i].ProjectName) < strings.ToLower(workspaces[j].ProjectName)
	})

	return UserWorkspacesResponse{
		User:       toSubjectIdentity(target),
		ClientApp:  clientApp,
		VCSFilter:  vcsFilter,
		Track:      effTrack,
		Tool:       effTool,
		Workspaces: workspaces,
	}, nil
}

func resolveToolTrackDefaults(clientApp, explicitTrack, explicitTool string) (track, tool string) {
	track = strings.ToLower(strings.TrimSpace(explicitTrack))
	tool = strings.ToLower(strings.TrimSpace(explicitTool))
	if track != "" || tool != "" {
		return track, tool
	}
	switch clientApp {
	case "geppetto":
		return "art_2d", "geppetto"
	case "goethe":
		return "narrative", "goethe"
	case "fontpit":
		return "typography", "fontpit"
	case "animafaber":
		return "", "animafaber"
	case "carnifex":
		return "", "carnifex"
	case "aetherion":
		return "", "godot"
	default:
		return "", ""
	}
}

func matchParticipantRecords(ctx context.Context, st store.Store, state map[string]any, target SubjectProfile) (ids []string, roles []string) {
	ids = []string{}
	roles = []string{}
	for _, p := range sliceOfMaps(state["participants"]) {
		if strVal(p["endedAt"]) != "" {
			continue
		}
		matched := false
		if uid, ok := intVal(p["userId"]); ok && uid > 0 {
			if target.WikiUserID == uid {
				matched = true
			} else if st != nil && target.Email != "" {
				if wu, err := st.GetWikiUserByID(ctx, uid); err == nil && wu != nil && strings.EqualFold(wu.Email, target.Email) {
					matched = true
				}
			}
		}
		if !matched && target.Email != "" && strVal(p["email"]) != "" && strings.EqualFold(strVal(p["email"]), target.Email) {
			matched = true
		}
		if !matched && target.Name != "" && strings.EqualFold(strings.TrimSpace(strVal(p["name"])), strings.TrimSpace(target.Name)) {
			matched = true
		}
		if !matched && target.Username != "" && strings.EqualFold(strings.TrimSpace(strVal(p["name"])), strings.TrimSpace(target.Username)) {
			matched = true
		}
		if matched {
			pid := strVal(p["id"])
			if pid != "" && !slices.Contains(ids, pid) {
				ids = append(ids, pid)
			}
			if rList, ok := strSlice(p["roles"]); ok {
				for _, r := range rList {
					addUniqueString(&roles, r)
				}
			}
		}
	}
	return ids, roles
}

func summarizeRepositories(state map[string]any, role string, loreGrant EffectiveLoreGrant, target SubjectProfile, vcsFilter string) []RepositorySummary {
	out := make([]RepositorySummary, 0)
	canEditStudio := role == "manager" || role == "editor"

	for _, r := range sliceOfMaps(state["repositories"]) {
		sys := strVal(r["system"])
		if vcsFilter != "" && sys != vcsFilter {
			continue
		}
		canRead := true
		canWrite := canEditStudio
		if sys == "lore" {
			canRead = loreGrant.CanRead
			canWrite = loreGrant.CanWrite
		}
		if !canRead {
			continue
		}

		remote := strVal(r["remoteUri"])
		var cloneCmd []string
		switch sys {
		case "lore":
			if remote != "" {
				cloneCmd = []string{"lore", "clone", remote}
			}
		case "git":
			if remote != "" {
				cloneCmd = []string{"git", "clone", remote}
			}
		default:
			cloneCmd = []string{}
		}
		if cloneCmd == nil {
			cloneCmd = []string{}
		}

		var branch, rev string
		clean := true
		locks := []RepoLockSummary{}
		if stMap, ok := r["status"].(map[string]any); ok && stMap != nil {
			branch = strVal(stMap["branch"])
			rev = strVal(stMap["revision"])
			if c, ok := stMap["clean"].(bool); ok {
				clean = c
			}
			for _, lk := range sliceOfMaps(stMap["locks"]) {
				owner := strVal(lk["owner"])
				locks = append(locks, RepoLockSummary{
					Path:        strVal(lk["path"]),
					Owner:       owner,
					Branch:      strVal(lk["branch"]),
					LockedAt:    strVal(lk["lockedAt"]),
					OwnedByUser: isSameOwner(owner, target),
				})
			}
		}

		accessLevel := "read-only"
		if sys == "lore" {
			if slices.Contains(loreGrant.Actions, "admin") || slices.Contains(loreGrant.Actions, "owner") {
				accessLevel = "admin"
			} else if canWrite {
				accessLevel = "read-write"
			}
		} else {
			if role == "manager" {
				accessLevel = "admin"
			} else if canWrite {
				accessLevel = "read-write"
			}
		}

		out = append(out, RepositorySummary{
			ID:             strVal(r["id"]),
			Name:           strVal(r["name"]),
			Slug:           strVal(r["slug"]),
			System:         sys,
			RemoteURI:      remote,
			LoreProject:    strVal(r["loreProject"]),
			LoreRepo:       strVal(r["loreRepo"]),
			LoreFullName:   strVal(r["loreFullName"]),
			DefaultBranch:  strVal(r["defaultBranch"]),
			WorkspacePath:  strVal(r["workspacePath"]),
			AccessLevel:    accessLevel,
			CanRead:        canRead,
			CanWrite:       canWrite,
			CloneCommand:   cloneCmd,
			Branch:         branch,
			LatestRevision: rev,
			Clean:          clean,
			LockCount:      len(locks),
			Locks:          locks,
		})
	}
	return out
}

func isSameOwner(lockOwner string, target SubjectProfile) bool {
	lockOwner = strings.TrimSpace(lockOwner)
	if lockOwner == "" {
		return false
	}
	for _, cand := range []string{target.Username, target.Email, target.Name, target.Subject} {
		if cand != "" && strings.EqualFold(lockOwner, strings.TrimSpace(cand)) {
			return true
		}
	}
	return false
}

func countTasks(state map[string]any, participantIDs []string) (openCount int, assignedOpen int) {
	for _, t := range sliceOfMaps(state["tasks"]) {
		if strVal(t["status"]) == "done" {
			continue
		}
		openCount++
		if aid := strVal(t["assigneeId"]); aid != "" && slices.Contains(participantIDs, aid) {
			assignedOpen++
		}
	}
	return openCount, assignedOpen
}

func inferPrimaryWorkspacePath(repos []RepositorySummary) string {
	for _, r := range repos {
		if r.System == "git" && r.WorkspacePath != "" {
			return r.WorkspacePath
		}
	}
	for _, r := range repos {
		if r.WorkspacePath != "" {
			return r.WorkspacePath
		}
	}
	return "."
}

func pickActiveMilestone(state map[string]any) *MilestoneSummary {
	curPhase := strVal(state["phase"])
	var fallback *MilestoneSummary
	for _, m := range sliceOfMaps(state["milestones"]) {
		ms := &MilestoneSummary{
			ID:      strVal(m["id"]),
			Name:    strVal(m["name"]),
			Phase:   strVal(m["phase"]),
			DueDate: strVal(m["dueDate"]),
		}
		if ms.Phase == curPhase {
			return ms
		}
		if fallback == nil {
			fallback = ms
		}
	}
	return fallback
}

func pickActiveCycle(state map[string]any) *CycleSummary {
	cycles := sliceOfMaps(state["cycles"])
	if len(cycles) == 0 {
		return nil
	}
	now := nowISO()
	for _, c := range cycles {
		sd, ed := strVal(c["startDate"]), strVal(c["endDate"])
		if (sd == "" || sd <= now) && (ed == "" || ed >= now) {
			return &CycleSummary{
				ID:        strVal(c["id"]),
				Name:      strVal(c["name"]),
				StartDate: sd,
				EndDate:   ed,
			}
		}
	}
	last := cycles[len(cycles)-1]
	return &CycleSummary{
		ID:        strVal(last["id"]),
		Name:      strVal(last["name"]),
		StartDate: strVal(last["startDate"]),
		EndDate:   strVal(last["endDate"]),
	}
}

func collectWorkspaceTasks(state map[string]any, participantIDs []string, effTrack, effTool string) []WorkspaceTask {
	out := make([]WorkspaceTask, 0)
	for _, t := range sliceOfMaps(state["tasks"]) {
		status := strVal(t["status"])
		if status == "done" {
			continue
		}
		aid := strVal(t["assigneeId"])
		assigned := aid != "" && slices.Contains(participantIDs, aid)
		disc := strings.ToLower(strVal(t["discipline"]))
		tURI := strings.ToLower(strVal(t["toolUri"]))

		matchesToolOrTrack := false
		if effTool != "" && (strings.HasPrefix(tURI, effTool+":") || strings.Contains(disc, effTool)) {
			matchesToolOrTrack = true
		}
		if effTrack == "art_2d" && (strings.Contains(disc, "art") || strings.Contains(disc, "2d") || strings.Contains(disc, "ilustra") || strings.Contains(disc, "concept")) {
			matchesToolOrTrack = true
		} else if effTrack != "" && strings.Contains(disc, strings.ReplaceAll(effTrack, "_", " ")) {
			matchesToolOrTrack = true
		}

		if !assigned && !matchesToolOrTrack && (effTrack != "" || effTool != "") {
			continue
		}
		if !assigned && effTrack == "" && effTool == "" && len(participantIDs) > 0 {
			continue
		}

		out = append(out, WorkspaceTask{
			ID:             strVal(t["id"]),
			Title:          strVal(t["title"]),
			Discipline:     strVal(t["discipline"]),
			Phase:          strVal(t["phase"]),
			Stage:          strVal(t["stage"]),
			Status:         status,
			Priority:       strVal(t["priority"]),
			Type:           strVal(t["type"]),
			DueDate:        strVal(t["dueDate"]),
			ToolURI:        strVal(t["toolUri"]),
			AssigneeID:     aid,
			AssignedToUser: assigned,
		})
	}
	return out
}

func collectWorkspaceArtifacts(state map[string]any, repos []RepositorySummary, effTrack, effTool string) []WorkspaceArtifact {
	repoByID := make(map[string]RepositorySummary, len(repos))
	lockByPath := make(map[string]string)
	for _, r := range repos {
		repoByID[r.ID] = r
		for _, lk := range r.Locks {
			if lk.Path != "" {
				lockByPath[lk.Path] = lk.Owner
			}
		}
	}

	out := make([]WorkspaceArtifact, 0)
	for _, a := range sliceOfMaps(state["artifacts"]) {
		aTrack := strVal(a["track"])
		aTool := strVal(a["tool"])
		aPath := strVal(a["assetPath"])
		if effTrack != "" || effTool != "" {
			matchTrack := effTrack != "" && aTrack == effTrack
			matchTool := effTool != "" && aTool == effTool
			matchKra := effTool == "geppetto" && strings.HasSuffix(strings.ToLower(aPath), ".kra")
			matchGodot := (effTool == "godot" || effTool == "aetherion") && (aTrack == "scene_level" || aTrack == "code_system" || aTool == "aetherion" || aTool == "godot" || strings.HasPrefix(strVal(a["toolUri"]), "res:") || strings.HasPrefix(strVal(a["toolUri"]), "godot:") || strings.HasPrefix(strVal(a["toolUri"]), "aetherion:"))
			if !matchTrack && !matchTool && !matchKra && !matchGodot {
				// If both track and tool were explicitly non-matching, skip
				continue
			}
		}

		rid := strVal(a["repoId"])
		var rSlug, rLore string
		if rInfo, ok := repoByID[rid]; ok {
			rSlug = rInfo.Slug
			rLore = rInfo.LoreFullName
		} else if rid != "" {
			// Also look up unfiltered state repositories if repo wasn't in filtered list
			if rawRepo, err := findByID(state["repositories"], rid); err == nil {
				rSlug = strVal(rawRepo["slug"])
				rLore = strVal(rawRepo["loreFullName"])
			}
		}

		out = append(out, WorkspaceArtifact{
			ID:              strVal(a["id"]),
			Name:            strVal(a["name"]),
			Kind:            strVal(a["kind"]),
			Track:           aTrack,
			Stage:           strVal(a["stage"]),
			Tool:            aTool,
			ToolURI:         strVal(a["toolUri"]),
			RepoID:          rid,
			RepoSlug:        rSlug,
			LoreFullName:    rLore,
			AssetPath:       aPath,
			TaskID:          strVal(a["taskId"]),
			LastVCSRevision: strVal(a["lastVcsRevision"]),
			LockedBy:        lockByPath[aPath],
		})
	}
	return out
}

func collectStorefrontArtSlots(state map[string]any, clientApp, effTrack string) []WorkspaceStorefrontAsset {
	if effTrack != "" && effTrack != "art_2d" && effTrack != "tabletop_print" && clientApp != "geppetto" {
		return []WorkspaceStorefrontAsset{}
	}
	out := make([]WorkspaceStorefrontAsset, 0)
	for _, target := range sliceOfMaps(state["platformTargets"]) {
		if boolVal(target["archived"]) {
			continue
		}
		tid := strVal(target["id"])
		tName := strVal(target["name"])
		sf := strVal(target["storefront"])
		for _, slot := range sliceOfMaps(target["assets"]) {
			st := strVal(slot["status"])
			if st == "done" || st == "na" {
				continue
			}
			fmtStr := strings.ToUpper(strVal(slot["format"]))
			if clientApp == "geppetto" && strings.Contains(fmtStr, "MP4") && !strings.Contains(fmtStr, "PNG") && !strings.Contains(fmtStr, "JPG") {
				continue
			}
			req := true
			if b, ok := slot["required"].(bool); ok {
				req = b
			}
			out = append(out, WorkspaceStorefrontAsset{
				TargetID:   tid,
				TargetName: tName,
				Storefront: sf,
				SlotID:     strVal(slot["id"]),
				Name:       strVal(slot["name"]),
				Dimensions: strVal(slot["dimensions"]),
				Format:     strVal(slot["format"]),
				Spec:       strVal(slot["spec"]),
				Status:     st,
				Required:   req,
				ToolURI:    strVal(slot["toolUri"]),
			})
		}
	}
	return out
}
