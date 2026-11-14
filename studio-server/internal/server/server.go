// Package server exposes the HTTP/JSON & multipart REST API of the Rogue Fairy Studio Server.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/roguefairystudios/intranet/studio-server/internal/auth"
	"github.com/roguefairystudios/intranet/studio-server/internal/config"
	"github.com/roguefairystudios/intranet/studio-server/internal/domain"
	"github.com/roguefairystudios/intranet/studio-server/internal/store"
)

const maxJSONBodyBytes = 512 << 10 // 512 KiB

// Options configures a Studio HTTP Server instance.
type Options struct {
	Config        config.Config
	Store         store.Store
	Authenticator *auth.Authenticator
	Directory     auth.DirectoryClient
	Log           *slog.Logger
}

// Server wires routing, authentication, CORS, persistence, and domain logic.
type Server struct {
	cfg    config.Config
	store  store.Store
	auth   *auth.Authenticator
	dir    auth.DirectoryClient
	policy domain.PolicyConfig
	log    *slog.Logger
	mux    *http.ServeMux
}

// New constructs a ready-to-serve Studio Server.
func New(opts Options) *Server {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	s := &Server{
		cfg:   opts.Config,
		store: opts.Store,
		auth:  opts.Authenticator,
		dir:   opts.Directory,
		policy: domain.PolicyConfig{
			AdminGroups:    opts.Config.AdminGroups,
			EmployeeGroups: opts.Config.EmployeeGroups,
			PartnerGroups:  opts.Config.PartnerGroups,
		},
		log: log,
		mux: http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// Handler returns the root http.Handler with CORS and security headers applied.
func (s *Server) Handler() http.Handler {
	return s.withMiddleware(s.mux)
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /v1/health", s.handleHealth)

	s.mux.HandleFunc("GET /v1/auth/me", s.requireAuth(s.handleAuthMe))
	s.mux.HandleFunc("GET /v1/configs", s.requireAuth(s.handleListStudioConfigs))
	s.mux.HandleFunc("GET /v1/configs/{tool}", s.requireAuth(s.handleGetStudioConfig))
	s.mux.HandleFunc("GET /v1/me/projects", s.requireAuth(s.handleMeProjects))
	s.mux.HandleFunc("GET /v1/me/workspaces", s.requireAuth(s.handleMeWorkspaces))
	s.mux.HandleFunc("GET /v1/users/{identifier}/projects", s.requireAuth(s.handleUserProjects))
	s.mux.HandleFunc("GET /v1/users/{identifier}/workspaces", s.requireAuth(s.handleUserWorkspaces))

	s.mux.HandleFunc("GET /v1/projects", s.requireAuth(s.handleListProjects))
	s.mux.HandleFunc("POST /v1/projects", s.requireAuth(s.handleCreateProject))
	s.mux.HandleFunc("GET /v1/projects/{projectId}", s.requireAuth(s.handleGetProject))
	s.mux.HandleFunc("GET /v1/projects/{projectId}/configs", s.requireAuth(s.handleListProjectConfigs))
	s.mux.HandleFunc("GET /v1/projects/{projectId}/configs/{tool}", s.requireAuth(s.handleGetProjectConfig))
	s.mux.HandleFunc("POST /v1/projects/{projectId}/commands", s.requireAuth(s.handleProjectCommand))
	s.mux.HandleFunc("POST /v1/projects/{projectId}/events", s.requireAuth(s.handleProjectEvent))
	s.mux.HandleFunc("GET /v1/projects/{projectId}/credits/{snapshotId}", s.requireAuth(s.handleExportCredits))

	s.mux.HandleFunc("POST /v1/projects/{projectId}/files", s.requireAuth(s.handleUploadFile))
	s.mux.HandleFunc("GET /v1/projects/{projectId}/files/{fileId}", s.requireAuth(s.handleDownloadFile))

	s.mux.HandleFunc("GET /v1/directory/users", s.requireAuth(s.handleDirectoryUsers))
	s.mux.HandleFunc("GET /v1/directory/groups", s.requireAuth(s.handleDirectoryGroups))

	s.mux.HandleFunc("GET /v1/service-keys", s.requireAuth(s.handleListServiceKeys))
	s.mux.HandleFunc("POST /v1/service-keys", s.requireAuth(s.handleCreateServiceKey))
	s.mux.HandleFunc("DELETE /v1/service-keys/{id}", s.requireAuth(s.handleDeleteServiceKey))
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, no-store")

		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" && s.isAllowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, X-Studio-Key, X-Studio-Client")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) isAllowedOrigin(origin string) bool {
	for _, allowed := range s.cfg.CORSOrigins {
		if allowed == "*" || strings.EqualFold(strings.TrimRight(allowed, "/"), strings.TrimRight(origin, "/")) {
			return true
		}
	}
	return false
}

type authedHandler func(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile)

func (s *Server) requireAuth(fn authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			writeError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}
		principal, err := s.auth.Authenticate(r)
		if err != nil {
			if errors.Is(err, auth.ErrIdentityProviderUnavailable) {
				writeError(w, http.StatusServiceUnavailable, "Identity provider unavailable")
				return
			}
			writeError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}
		subj, err := domain.ResolvePrincipalProfile(r.Context(), s.store, s.policy, principal)
		if err != nil {
			s.log.Error("failed to resolve principal profile", "error", err)
			writeError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		fn(w, r, subj)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if s.store != nil {
		if err := s.store.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"ok":    false,
				"error": "database unavailable",
			})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "studio-server",
	})
}

func (s *Server) handleAuthMe(w http.ResponseWriter, _ *http.Request, subj domain.SubjectProfile) {
	writeJSON(w, http.StatusOK, subj)
}

func (s *Server) handleMeProjects(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	q := r.URL.Query()
	clientApp := auth.NormalizeClientApp(q.Get("client"), subj.ClientApp)
	resp, err := domain.QueryUserProjects(r.Context(), s.store, s.policy, subj, nil, domain.ProjectQueryOptions{
		ClientApp:       clientApp,
		IntranetURL:     s.cfg.IntranetURL,
		IncludeArchived: queryBool(q.Get("includeArchived")),
		IncludeAllKinds: queryBool(q.Get("includeAllKinds")),
	})
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleMeWorkspaces(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	q := r.URL.Query()
	clientApp := auth.NormalizeClientApp(q.Get("client"), subj.ClientApp)
	resp, err := domain.QueryUserWorkspaces(r.Context(), s.store, s.policy, subj, nil, domain.WorkspaceQueryOptions{
		ClientApp:         clientApp,
		IntranetURL:       s.cfg.IntranetURL,
		VCS:               q.Get("vcs"),
		Track:             q.Get("track"),
		Tool:              q.Get("tool"),
		OnlyWritable:      queryBool(q.Get("onlyWritable")),
		OnlyActionable:    queryBool(q.Get("onlyActionable")),
		IncludeEmptyRepos: queryBool(q.Get("includeEmptyRepos")),
		IncludeArchived:   queryBool(q.Get("includeArchived")),
	})
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) resolveTargetOrSelf(ctx context.Context, identifier string, caller domain.SubjectProfile) (target domain.SubjectProfile, viewer *domain.SubjectProfile, err error) {
	idTrim := strings.TrimSpace(identifier)
	if idTrim == "" || strings.EqualFold(idTrim, "me") || isSameSubject(idTrim, caller) {
		return caller, nil, nil
	}
	resolved, err := domain.ResolveTargetUser(ctx, s.store, s.dir, s.policy, idTrim)
	if err != nil {
		return domain.SubjectProfile{}, nil, err
	}
	return resolved, &caller, nil
}

func isSameSubject(id string, s domain.SubjectProfile) bool {
	if s.Subject != "" && strings.EqualFold(id, s.Subject) {
		return true
	}
	if s.Email != "" && strings.EqualFold(id, s.Email) {
		return true
	}
	if s.Username != "" && strings.EqualFold(id, s.Username) {
		return true
	}
	if s.WikiUserID > 0 && id == fmt.Sprintf("%d", s.WikiUserID) {
		return true
	}
	return false
}

func (s *Server) handleUserProjects(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	target, viewer, err := s.resolveTargetOrSelf(r.Context(), r.PathValue("identifier"), subj)
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	q := r.URL.Query()
	clientApp := auth.NormalizeClientApp(q.Get("client"), subj.ClientApp)
	resp, err := domain.QueryUserProjects(r.Context(), s.store, s.policy, target, viewer, domain.ProjectQueryOptions{
		ClientApp:       clientApp,
		IntranetURL:     s.cfg.IntranetURL,
		IncludeArchived: queryBool(q.Get("includeArchived")),
		IncludeAllKinds: queryBool(q.Get("includeAllKinds")),
	})
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleUserWorkspaces(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	target, viewer, err := s.resolveTargetOrSelf(r.Context(), r.PathValue("identifier"), subj)
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	q := r.URL.Query()
	clientApp := auth.NormalizeClientApp(q.Get("client"), subj.ClientApp)
	resp, err := domain.QueryUserWorkspaces(r.Context(), s.store, s.policy, target, viewer, domain.WorkspaceQueryOptions{
		ClientApp:         clientApp,
		IntranetURL:       s.cfg.IntranetURL,
		VCS:               q.Get("vcs"),
		Track:             q.Get("track"),
		Tool:              q.Get("tool"),
		OnlyWritable:      queryBool(q.Get("onlyWritable")),
		OnlyActionable:    queryBool(q.Get("onlyActionable")),
		IncludeEmptyRepos: queryBool(q.Get("includeEmptyRepos")),
		IncludeArchived:   queryBool(q.Get("includeArchived")),
	})
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	rows, err := s.store.ListProjects(r.Context())
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	out := make([]domain.PresentedProject, 0, len(rows))
	for _, row := range rows {
		presented, pErr := domain.PresentProject(r.Context(), s.store, row, subj, s.policy)
		if pErr == nil {
			out = append(out, presented)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": out})
}

type createProjectRequest struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Engine string `json:"engine,omitempty"`
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	if !subj.IsAdmin {
		writeError(w, http.StatusForbidden, "Only administrators can create projects")
		return
	}
	var req createProjectRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	state, err := domain.CreateState(req.Name, req.Kind, subj.ActorID())
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	if eng := strings.TrimSpace(req.Engine); eng != "" {
		if !slices.Contains(domain.Engines, eng) {
			writeError(w, http.StatusBadRequest, "Invalid project engine")
			return
		}
		state["engine"] = eng
	}
	encoded, err := domain.MarshalState(state)
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	pid, _ := state["projectId"].(string)
	row := store.ProjectRow{
		ID:       pid,
		Revision: 1,
		State:    encoded,
	}
	if err := s.store.CreateProject(r.Context(), row); err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	presented, err := domain.PresentProject(r.Context(), s.store, row, subj, s.policy)
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, presented)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	row, _, _, err := domain.ResolveVisibleProject(r.Context(), s.store, s.policy, r.PathValue("projectId"), subj, nil)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Project unavailable")
			return
		}
		s.writeDomainOrStoreError(w, err)
		return
	}
	presented, err := domain.PresentProject(r.Context(), s.store, *row, subj, s.policy)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Project unavailable")
			return
		}
		s.writeDomainOrStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, presented)
}

func (s *Server) configEnvForRequest(r *http.Request) domain.ConfigEnvOptions {
	apiURL := strings.TrimSpace(s.cfg.PublicURL)
	if apiURL == "" && r != nil && isSafeHostHeader(r.Host) {
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
			scheme = "https"
		}
		apiURL = scheme + "://" + strings.TrimSpace(r.Host)
	}
	maxMB := int(s.cfg.MaxUploadBytes >> 20)
	if maxMB <= 0 {
		maxMB = 50
	}
	return domain.ConfigEnvOptions{
		StudioAPIURL:   apiURL,
		IntranetURL:    s.cfg.IntranetURL,
		OIDCBaseURL:    s.cfg.OIDCBaseURL,
		LoreRemoteBase: s.cfg.LoreRemoteBase,
		LoreBridgeURL:  s.cfg.LoreBridgeURL,
		LoreAuthURL:    s.cfg.LoreAuthURL,
		OIDCClientIDs:  s.cfg.OIDCClientIDs,
		MaxUploadMB:    maxMB,
	}
}

func isSafeHostHeader(h string) bool {
	h = strings.TrimSpace(h)
	if h == "" || len(h) > 255 {
		return false
	}
	for _, r := range h {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') &&
			r != '.' && r != '-' && r != ':' && r != '[' && r != ']' {
			return false
		}
	}
	return true
}

func writeFormattedConfig(w http.ResponseWriter, r *http.Request, payload any, extFormat, filenameBase string, yamlHeaders ...string) bool {
	format, err := domain.ParseConfigFormat(r.URL.Query().Get("format"), extFormat, r.Header.Get("Accept"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	encoded, contentType, err := domain.MarshalConfig(payload, format, yamlHeaders...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to encode configuration")
		return false
	}
	if queryBool(r.URL.Query().Get("download")) && filenameBase != "" {
		ext := "json"
		if format == "yaml" {
			ext = "yaml"
		}
		cd := mime.FormatMediaType("attachment", map[string]string{"filename": filenameBase + "." + ext})
		if cd != "" {
			w.Header().Set("Content-Disposition", cd)
		}
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
	return true
}

func (s *Server) handleListStudioConfigs(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	catalog := domain.BuildStudioConfigCatalog(subj, s.configEnvForRequest(r))
	writeFormattedConfig(w, r, catalog, "", "roguefairy-configs-catalog",
		"Rogue Fairy Studios — Studio Configuration Catalog",
	)
}

func (s *Server) handleGetStudioConfig(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	rawTool := r.PathValue("tool")
	normTool, extFormat, err := domain.NormalizeConfigTool(rawTool)
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	target, _, err := s.resolveTargetOrSelf(r.Context(), r.URL.Query().Get("user"), subj)
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	doc, filenameBase, headers, err := domain.BuildStudioToolConfig(normTool, target, s.configEnvForRequest(r))
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	writeFormattedConfig(w, r, doc, extFormat, filenameBase, headers...)
}

func (s *Server) handleListProjectConfigs(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	target, viewer, err := s.resolveTargetOrSelf(r.Context(), r.URL.Query().Get("user"), subj)
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	row, state, _, err := domain.ResolveVisibleProject(r.Context(), s.store, s.policy, r.PathValue("projectId"), target, viewer)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Project unavailable")
			return
		}
		s.writeDomainOrStoreError(w, err)
		return
	}
	catalog := domain.BuildProjectConfigCatalog(r.Context(), s.store, s.policy, *row, state, target, s.configEnvForRequest(r))
	writeFormattedConfig(w, r, catalog, "", catalog.Project.Slug+"-configs-catalog",
		fmt.Sprintf("Rogue Fairy Studios — Project Configuration Catalog: %s", catalog.Project.Name),
	)
}

func (s *Server) handleGetProjectConfig(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	rawTool := r.PathValue("tool")
	normTool, extFormat, err := domain.NormalizeConfigTool(rawTool)
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	target, viewer, err := s.resolveTargetOrSelf(r.Context(), r.URL.Query().Get("user"), subj)
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	row, state, _, err := domain.ResolveVisibleProject(r.Context(), s.store, s.policy, r.PathValue("projectId"), target, viewer)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Project unavailable")
			return
		}
		s.writeDomainOrStoreError(w, err)
		return
	}
	doc, filenameBase, headers, err := domain.BuildProjectToolConfig(
		r.Context(), s.store, s.policy, *row, state, target, viewer, normTool, s.configEnvForRequest(r),
	)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Project unavailable")
			return
		}
		s.writeDomainOrStoreError(w, err)
		return
	}
	writeFormattedConfig(w, r, doc, extFormat, filenameBase, headers...)
}

type commandRequest struct {
	Revision int            `json:"revision"`
	Command  string         `json:"command"`
	Input    map[string]any `json:"input"`
}

func (s *Server) handleProjectCommand(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	projectID := r.PathValue("projectId")
	var req commandRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Input == nil {
		writeError(w, http.StatusBadRequest, "Invalid input")
		return
	}

	maxAttempts := 1
	if req.Revision <= 0 {
		maxAttempts = 4
	}
	for attempt := 0; attempt < maxAttempts; attempt++ {
		row, err := s.store.GetProject(r.Context(), projectID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "Project unavailable")
				return
			}
			s.writeDomainOrStoreError(w, err)
			return
		}
		state, err := domain.ParseState(row.State)
		if err != nil {
			s.writeDomainOrStoreError(w, err)
			return
		}
		role := domain.RoleFor(state, subj, s.policy)
		if role == "" {
			writeError(w, http.StatusNotFound, "Project unavailable")
			return
		}
		if req.Revision > 0 && row.Revision != req.Revision {
			writeError(w, http.StatusConflict, "Project changed. Reload and retry your change.")
			return
		}

		if err := domain.ApplyCommand(r.Context(), s.store, state, subj, role, req.Command, req.Input); err != nil {
			s.writeDomainOrStoreError(w, err)
			return
		}
		encoded, err := domain.MarshalState(state)
		if err != nil {
			s.writeDomainOrStoreError(w, err)
			return
		}
		if err := s.store.UpdateProject(r.Context(), projectID, row.Revision, encoded); err != nil {
			if errors.Is(err, store.ErrConflict) {
				if req.Revision <= 0 && attempt+1 < maxAttempts {
					continue
				}
				writeError(w, http.StatusConflict, "Project changed. Reload and retry your change.")
				return
			}
			s.writeDomainOrStoreError(w, err)
			return
		}
		updatedRow := store.ProjectRow{
			ID:       projectID,
			Revision: row.Revision + 1,
			State:    encoded,
		}
		presented, err := domain.PresentProject(r.Context(), s.store, updatedRow, subj, s.policy)
		if err != nil {
			s.writeDomainOrStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, presented)
		return
	}
}

func (s *Server) handleProjectEvent(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	projectID := r.PathValue("projectId")
	var eventInput map[string]any
	if err := decodeJSONBody(w, r, &eventInput); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, hasSrc := eventInput["source"]; !hasSrc && subj.ClientApp != "" && subj.ClientApp != "generic" && subj.ClientApp != "cli" {
		eventInput["source"] = subj.ClientApp
	}

	const maxCASRetries = 5
	for attempt := 0; attempt < maxCASRetries; attempt++ {
		row, err := s.store.GetProject(r.Context(), projectID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "Project unavailable")
				return
			}
			s.writeDomainOrStoreError(w, err)
			return
		}
		state, err := domain.ParseState(row.State)
		if err != nil {
			s.writeDomainOrStoreError(w, err)
			return
		}
		role := domain.RoleFor(state, subj, s.policy)
		if role == "" {
			writeError(w, http.StatusNotFound, "Project unavailable")
			return
		}
		if err := domain.ApplyCommand(r.Context(), s.store, state, subj, role, "integrationEvent", eventInput); err != nil {
			s.writeDomainOrStoreError(w, err)
			return
		}
		encoded, err := domain.MarshalState(state)
		if err != nil {
			s.writeDomainOrStoreError(w, err)
			return
		}
		if err := s.store.UpdateProject(r.Context(), projectID, row.Revision, encoded); err != nil {
			if errors.Is(err, store.ErrConflict) && attempt+1 < maxCASRetries {
				continue
			}
			if errors.Is(err, store.ErrConflict) {
				writeError(w, http.StatusConflict, "Project changed. Reload and retry your change.")
				return
			}
			s.writeDomainOrStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        true,
			"projectId": projectID,
			"revision":  row.Revision + 1,
		})
		return
	}
}

func (s *Server) handleExportCredits(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	projectID := r.PathValue("projectId")
	snapshotID := r.PathValue("snapshotId")
	format := strings.TrimSpace(r.URL.Query().Get("format"))
	if format == "" {
		format = "json"
	}

	row, err := s.store.GetProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Project unavailable")
			return
		}
		s.writeDomainOrStoreError(w, err)
		return
	}
	state, err := domain.ParseState(row.State)
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	if domain.RoleFor(state, subj, s.policy) == "" {
		writeError(w, http.StatusNotFound, "Project unavailable")
		return
	}
	content, err := domain.ExportCredits(state, snapshotID, format)
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	if queryBool(r.URL.Query().Get("raw")) {
		ct := "application/json; charset=utf-8"
		if format == "markdown" {
			ct = "text/markdown; charset=utf-8"
		}
		w.Header().Set("Content-Type", ct)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"format":  format,
		"content": content,
	})
}

func (s *Server) handleUploadFile(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	projectID := r.PathValue("projectId")
	row, err := s.store.GetProject(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusForbidden, "Forbidden")
		return
	}
	state, err := domain.ParseState(row.State)
	if err != nil {
		writeError(w, http.StatusForbidden, "Forbidden")
		return
	}
	role := domain.RoleFor(state, subj, s.policy)
	archived, _ := state["archived"].(bool)
	if (role != "manager" && role != "editor") || archived {
		writeError(w, http.StatusForbidden, "Forbidden")
		return
	}

	maxBytes := s.cfg.MaxUploadBytes
	if maxBytes <= 0 {
		maxBytes = 50 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(64<<10))
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "File exceeds configured limits or upload is invalid")
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "File required")
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(data)) > maxBytes {
		writeError(w, http.StatusBadRequest, "File exceeds configured limits or upload is invalid")
		return
	}

	baseName := filepath.Base(strings.ReplaceAll(hdr.Filename, "\\", "/"))
	if baseName == "" || baseName == "." || baseName == "/" {
		baseName = "artifact.bin"
	}
	if len(baseName) > 255 {
		baseName = baseName[:255]
	}

	checksum := domain.SHA256Hex(data)
	saved, err := s.store.SaveFile(r.Context(), store.FileRow{
		ID:        domain.NewUUID(),
		ProjectID: projectID,
		Checksum:  checksum,
		Name:      baseName,
		CreatedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Data:      data,
	})
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":       saved.ID,
		"checksum": checksum,
		"path":     fmt.Sprintf("/p/studio/files/%s/%s", projectID, saved.ID),
		"apiPath":  fmt.Sprintf("/v1/projects/%s/files/%s", projectID, saved.ID),
	})
}

func (s *Server) handleDownloadFile(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	projectID := r.PathValue("projectId")
	fileID := r.PathValue("fileId")

	row, err := s.store.GetProject(r.Context(), projectID)
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	state, err := domain.ParseState(row.State)
	if err != nil || domain.RoleFor(state, subj, s.policy) == "" {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	f, err := s.store.GetFile(r.Context(), projectID, fileID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		s.writeDomainOrStoreError(w, err)
		return
	}

	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	cd := mime.FormatMediaType("attachment", map[string]string{"filename": f.Name})
	if cd == "" {
		cd = "attachment"
	}
	w.Header().Set("Content-Disposition", cd)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(f.Data)
}

func (s *Server) handleDirectoryUsers(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	projectID := strings.TrimSpace(r.URL.Query().Get("projectId"))
	if projectID != "" {
		row, err := s.store.GetProject(r.Context(), projectID)
		if err != nil {
			writeError(w, http.StatusForbidden, "Forbidden")
			return
		}
		state, err := domain.ParseState(row.State)
		if err != nil {
			writeError(w, http.StatusForbidden, "Forbidden")
			return
		}
		role := domain.RoleFor(state, subj, s.policy)
		if role != "manager" && role != "editor" {
			writeError(w, http.StatusForbidden, "Forbidden")
			return
		}
	} else if !subj.IsAdmin {
		writeError(w, http.StatusForbidden, "Forbidden")
		return
	}

	users, err := s.store.SearchWikiUsers(r.Context(), r.URL.Query().Get("search"), 30)
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	if users == nil {
		users = []store.WikiUser{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (s *Server) handleDirectoryGroups(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	projectID := strings.TrimSpace(r.URL.Query().Get("projectId"))
	if projectID != "" {
		row, err := s.store.GetProject(r.Context(), projectID)
		if err != nil {
			writeError(w, http.StatusForbidden, "Manager required")
			return
		}
		state, err := domain.ParseState(row.State)
		if err != nil {
			writeError(w, http.StatusForbidden, "Manager required")
			return
		}
		if domain.RoleFor(state, subj, s.policy) != "manager" {
			writeError(w, http.StatusForbidden, "Manager required")
			return
		}
	} else if !subj.IsAdmin {
		writeError(w, http.StatusForbidden, "Manager required")
		return
	}

	groups, err := s.store.SearchWikiGroups(r.Context(), r.URL.Query().Get("search"), 30)
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	if groups == nil {
		groups = []store.WikiGroup{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
}

type createServiceKeyRequest struct {
	Name   string   `json:"name"`
	Role   string   `json:"role"`
	Groups []string `json:"groups"`
}

func (s *Server) handleListServiceKeys(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	if !subj.IsAdmin {
		writeError(w, http.StatusForbidden, "Administrator required")
		return
	}
	keys, err := s.store.ListServiceKeys(r.Context())
	if err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	if keys == nil {
		keys = []store.ServiceKeyRecord{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

func (s *Server) handleCreateServiceKey(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	if !subj.IsAdmin {
		writeError(w, http.StatusForbidden, "Administrator required")
		return
	}
	var req createServiceKeyRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name, err := domain.SafeText(req.Name, 120)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid key name")
		return
	}
	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role == "" {
		role = "editor"
	}
	if !slices.Contains([]string{"reader", "editor", "admin"}, role) {
		writeError(w, http.StatusBadRequest, "Invalid service key role")
		return
	}
	groups := make([]string, 0, len(req.Groups))
	for _, g := range req.Groups {
		if cg, gErr := domain.SafeText(g, 255); gErr == nil {
			groups = append(groups, cg)
		}
	}

	plain, hash, err := auth.GenerateServiceKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate service key")
		return
	}
	rec := store.ServiceKeyRecord{
		ID:        domain.NewUUID(),
		Name:      name,
		Role:      role,
		Groups:    groups,
		CreatedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		CreatedBy: fmt.Sprintf("%v", subj.ActorID()),
	}
	if err := s.store.CreateServiceKey(r.Context(), rec, hash); err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"key":    rec,
		"secret": plain,
	})
}

func (s *Server) handleDeleteServiceKey(w http.ResponseWriter, r *http.Request, subj domain.SubjectProfile) {
	if !subj.IsAdmin {
		writeError(w, http.StatusForbidden, "Administrator required")
		return
	}
	if err := s.store.DeleteServiceKey(r.Context(), r.PathValue("id")); err != nil {
		s.writeDomainOrStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) writeDomainOrStoreError(w http.ResponseWriter, err error) {
	if ve, ok := domain.IsValidationError(err); ok {
		if ve.Forbidden {
			writeError(w, http.StatusForbidden, ve.Message)
			return
		}
		writeError(w, http.StatusBadRequest, ve.Message)
		return
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "Record not found")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "Project changed. Reload and retry your change.")
	default:
		s.log.Error("unhandled server error", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal server error")
	}
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return errors.New("Input too large")
		}
		return errors.New("Invalid input")
	}
	return nil
}

func queryBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
