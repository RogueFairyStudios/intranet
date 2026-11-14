package server_test

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/roguefairystudios/intranet/studio-server/internal/auth"
	"github.com/roguefairystudios/intranet/studio-server/internal/config"
	"github.com/roguefairystudios/intranet/studio-server/internal/domain"
	"github.com/roguefairystudios/intranet/studio-server/internal/server"
	"github.com/roguefairystudios/intranet/studio-server/internal/store"
)

type testEnv struct {
	api    *httptest.Server
	idp    *httptest.Server
	rsaKey *rsa.PrivateKey
	mem    *store.Memory
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa key: %v", err)
	}

	env := &testEnv{
		rsaKey: rsaKey,
		mem:    store.NewMemory(),
	}

	idpMux := http.NewServeMux()
	idpMux.HandleFunc("/application/o/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/.well-known/openid-configuration"):
			slugPath := strings.TrimSuffix(r.URL.Path, "/.well-known/openid-configuration")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":   env.idp.URL + slugPath + "/",
				"jwks_uri": env.idp.URL + slugPath + "/jwks/",
			})
		case strings.HasSuffix(r.URL.Path, "/jwks/"):
			eBytes := big.NewInt(int64(rsaKey.PublicKey.E)).Bytes()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"keys": []map[string]any{
					{
						"kty": "RSA",
						"use": "sig",
						"kid": "k1",
						"n":   base64.RawURLEncoding.EncodeToString(rsaKey.PublicKey.N.Bytes()),
						"e":   base64.RawURLEncoding.EncodeToString(eBytes),
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	})
	idpMux.HandleFunc("/api/v3/core/users/", func(w http.ResponseWriter, r *http.Request) {
		q := strings.ToLower(r.URL.Query().Get("search"))
		switch q {
		case "bianca", "bianca@roguefairy.net":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{
						"pk":        20,
						"uuid":      "sub-bianca",
						"username":  "bianca",
						"name":      "Bianca Art",
						"email":     "bianca@roguefairy.net",
						"is_active": true,
						"groups_obj": []map[string]any{
							{"name": "Rogue Fairy Studios"},
						},
					},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
		}
	})
	env.idp = httptest.NewServer(idpMux)
	t.Cleanup(env.idp.Close)

	// Seed Wiki users & groups so shared-DB lookups also work end-to-end
	gEmployees := store.WikiGroup{ID: 3, Name: "Rogue Fairy Studios"}
	gSecretArt := store.WikiGroup{ID: 10, Name: "Rogue Fairy Studios - Secret - Art"}
	env.mem.SeedWikiGroup(gEmployees)
	env.mem.SeedWikiGroup(gSecretArt)
	env.mem.SeedWikiUser(store.WikiUser{
		ID:         11,
		Email:      "lucas@roguefairy.net",
		Name:       "Lucas Engine",
		ProviderID: "sub-lucas",
		IsActive:   true,
		Groups:     []store.WikiGroup{gEmployees},
	})
	env.mem.SeedWikiUser(store.WikiUser{
		ID:         12,
		Email:      "bianca@roguefairy.net",
		Name:       "Bianca Art",
		ProviderID: "sub-bianca",
		IsActive:   true,
		Groups:     []store.WikiGroup{gEmployees},
	})

	cfg := config.Config{
		OIDCBaseURL:    env.idp.URL,
		OIDCAllowedSlugs: config.DefaultAllowedSlugs,
		InternalKey:    "wiki-internal-secret",
		IntranetURL:    "https://docs.roguefairy.net",
		MaxUploadBytes: 10 << 20,
		CORSOrigins:    []string{"https://docs.roguefairy.net"},
		AdminGroups:    []string{"Rogue Fairy Studios - C-Level", "authentik Admins"},
		EmployeeGroups: []string{"Rogue Fairy Studios"},
		PartnerGroups:  []string{"Rogue Fairy Studios - Partners"},
	}

	verifier := auth.NewMultiAppVerifier(auth.MultiAppVerifierOptions{
		BaseURL:      cfg.OIDCBaseURL,
		AllowedSlugs: cfg.OIDCAllowedSlugs,
	})
	dir := auth.NewAuthentikAPIClient(env.idp.URL, "dir-token", nil)
	authn := &auth.Authenticator{
		Verifier:    verifier,
		InternalKey: cfg.InternalKey,
		Keys:        env.mem,
		AdminGroups: cfg.AdminGroups,
	}

	srv := server.New(server.Options{
		Config:        cfg,
		Store:         env.mem,
		Authenticator: authn,
		Directory:     dir,
	})
	env.api = httptest.NewServer(srv.Handler())
	t.Cleanup(env.api.Close)
	return env
}

func (e *testEnv) token(t *testing.T, slug string, claims map[string]any) string {
	t.Helper()
	claims["iss"] = e.idp.URL + "/application/o/" + slug + "/"
	claims["exp"] = time.Now().Add(15 * time.Minute).Unix()
	hdr, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1", "typ": "JWT"})
	pay, _ := json.Marshal(claims)
	input := base64.RawURLEncoding.EncodeToString(hdr) + "." + base64.RawURLEncoding.EncodeToString(pay)
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, e.rsaKey, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (e *testEnv) doJSON(t *testing.T, method, path, bearer string, body any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.api.URL+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func TestEndToEndCrossToolQueriesAndIsolation(t *testing.T) {
	env := setupTestEnv(t)

	adminTok := env.token(t, "rfs-intranet", map[string]any{
		"sub":                "sub-ceo",
		"preferred_username": "ceo",
		"name":               "CEO",
		"email":              "ceo@roguefairy.net",
		"groups":             []string{"Rogue Fairy Studios - C-Level"},
	})

	// 1. Create 3 projects:
	// - Echoes of Nyx (digital, internal, employeeRole=editor, Git + Lore art repo)
	// - Grimoire Tabletop (analogue, internal, employeeRole=reader, Git only)
	// - Project Veil (digital, restricted, partner group only)
	stCode, echoes := env.doJSON(t, http.MethodPost, "/v1/projects", adminTok, map[string]any{
		"name": "Echoes of Nyx",
		"kind": "digital",
	})
	if stCode != http.StatusCreated {
		t.Fatalf("create Echoes failed: %d %+v", stCode, echoes)
	}
	echoesID := echoes["id"].(string)
	rev := int(echoes["revision"].(float64))

	runCmd := func(pid string, r int, cmd string, input map[string]any) int {
		code, res := env.doJSON(t, http.MethodPost, "/v1/projects/"+pid+"/commands", adminTok, map[string]any{
			"revision": r,
			"command":  cmd,
			"input":    input,
		})
		if code != http.StatusOK {
			t.Fatalf("command %s failed: %d %+v", cmd, code, res)
		}
		return int(res["revision"].(float64))
	}

	rev = runCmd(echoesID, rev, "loreConfig", map[string]any{
		"projectSlug":  "echoes-of-nyx",
		"visibility":   "internal",
		"employeeRole": "editor",
	})
	rev = runCmd(echoesID, rev, "repository", map[string]any{
		"name":          "Echoes Engine (Aetherion)",
		"slug":          "echoes-engine",
		"system":        "git",
		"remoteUri":     "https://github.com/RogueFairyStudios/echoes-of-nyx.git",
		"workspacePath": "game/project.godot",
	})
	rev = runCmd(echoesID, rev, "repository", map[string]any{
		"name":     "Echoes 2D Art (Lore)",
		"system":   "lore",
		"loreRepo": "art-2d",
	})
	rev = runCmd(echoesID, rev, "vcsStatus", map[string]any{
		"slug":     "echoes-of-nyx-art-2d",
		"branch":   "main",
		"revision": "9f82a10",
		"locks": []map[string]any{
			{
				"path":   "characters/nyx.kra",
				"owner":  "bianca",
				"branch": "main",
			},
		},
	})
	rev = runCmd(echoesID, rev, "participant", map[string]any{
		"name":   "Lucas Engine",
		"roles":  []string{"Lead Programmer"},
		"userId": 11,
	})
	rev = runCmd(echoesID, rev, "participant", map[string]any{
		"name":   "Bianca Art",
		"roles":  []string{"2D Concept & Key Art"},
		"userId": 12,
	})

	// Read state to get participant & repo IDs
	_, projSnap := env.doJSON(t, http.MethodGet, "/v1/projects/"+echoesID, adminTok, nil)
	pMap := projSnap["project"].(map[string]any)
	parts := pMap["participants"].([]any)
	lucasPartID := parts[0].(map[string]any)["id"].(string)
	biancaPartID := parts[1].(map[string]any)["id"].(string)
	repos := pMap["repositories"].([]any)
	loreRepoID := repos[1].(map[string]any)["id"].(string)

	rev = runCmd(echoesID, rev, "task", map[string]any{
		"title":      "Implement combat state machine",
		"discipline": "Programming",
		"assigneeId": lucasPartID,
		"toolUri":    "godot://project/echoes-of-nyx",
	})
	rev = runCmd(echoesID, rev, "task", map[string]any{
		"title":      "Paint Nyx portrait & Steam Header Capsule",
		"discipline": "2D Art",
		"assigneeId": biancaPartID,
		"toolUri":    "geppetto://echoes-of-nyx/characters/nyx.kra",
	})
	rev = runCmd(echoesID, rev, "artifact", map[string]any{
		"name":      "Nyx Portrait Master",
		"kind":      "Character Art",
		"track":     "art_2d",
		"stage":     "authoring",
		"tool":      "geppetto",
		"toolUri":   "geppetto://echoes-of-nyx/characters/nyx.kra",
		"repoId":    loreRepoID,
		"assetPath": "characters/nyx.kra",
	})
	_ = runCmd(echoesID, rev, "platformTarget", map[string]any{
		"storefront": "steam",
		"locale":     "pt",
	})

	// Create analogue project
	_, grimoire := env.doJSON(t, http.MethodPost, "/v1/projects", adminTok, map[string]any{
		"name": "Grimoire Tabletop",
		"kind": "analogue",
	})
	grimoireID := grimoire["id"].(string)
	_ = runCmd(grimoireID, 1, "loreConfig", map[string]any{
		"projectSlug":  "grimoire-tabletop",
		"visibility":   "internal",
		"employeeRole": "reader",
	})

	// Create restricted partner project
	_, veil := env.doJSON(t, http.MethodPost, "/v1/projects", adminTok, map[string]any{
		"name": "Project Veil",
		"kind": "digital",
	})
	veilID := veil["id"].(string)
	vRev := runCmd(veilID, 1, "loreConfig", map[string]any{
		"projectSlug": "project-veil",
		"visibility":  "restricted",
	})
	_ = runCmd(veilID, vRev, "groupAccess", map[string]any{
		"groupId":   10,
		"role":      "editor",
		"loreActions": []string{"read", "write"},
	})

	// 2. Query 1: "Usuário X está em quais projetos? Se é o Aetherion que está perguntando ele lista os projetos no project manager."
	lucasAetherionTok := env.token(t, "aetherion", map[string]any{
		"sub":                "sub-lucas",
		"preferred_username": "lucas",
		"email":              "lucas@roguefairy.net",
		"groups":             []string{"Rogue Fairy Studios"},
	})
	code, aethRes := env.doJSON(t, http.MethodGet, "/v1/me/projects", lucasAetherionTok, nil)
	if code != http.StatusOK {
		t.Fatalf("aetherion /v1/me/projects failed: %d %+v", code, aethRes)
	}
	if aethRes["clientApp"] != "aetherion" {
		t.Fatalf("expected inferred clientApp=aetherion, got %v", aethRes["clientApp"])
	}
	aethProjects := aethRes["projects"].([]any)
	if len(aethProjects) != 1 {
		t.Fatalf("expected 1 digital project for Lucas in Aetherion (excluding analogue & restricted), got %d: %+v", len(aethProjects), aethProjects)
	}
	ep := aethProjects[0].(map[string]any)
	if ep["name"] != "Echoes of Nyx" || ep["primaryWorkspacePath"] != "game/project.godot" || int(ep["assignedOpenTaskCount"].(float64)) != 1 {
		t.Fatalf("unexpected Aetherion project summary: %+v", ep)
	}

	// With includeAllKinds=1, Lucas also sees Grimoire Tabletop, still never Project Veil
	_, allKindsRes := env.doJSON(t, http.MethodGet, "/v1/me/projects?includeAllKinds=1", lucasAetherionTok, nil)
	if len(allKindsRes["projects"].([]any)) != 2 {
		t.Fatalf("expected 2 internal projects with includeAllKinds=1, got %d", len(allKindsRes["projects"].([]any)))
	}

	// 3. Query 2: "Usuário Y está em qual projeto na qual ele tem de trabalhar nos repositórios do Lore para adicionar a arte e etc?"
	// Asked both by Admin querying `/v1/users/bianca/workspaces?vcs=lore&client=geppetto&onlyWritable=1` AND by Bianca directly from Geppetto!
	code, biancaWork := env.doJSON(t, http.MethodGet, "/v1/users/bianca/workspaces?vcs=lore&client=geppetto&onlyWritable=1", adminTok, nil)
	if code != http.StatusOK {
		t.Fatalf("admin query for bianca workspaces failed: %d %+v", code, biancaWork)
	}
	wsList := biancaWork["workspaces"].([]any)
	if len(wsList) != 1 {
		t.Fatalf("expected 1 writable Lore art workspace for Bianca, got %d: %+v", len(wsList), wsList)
	}
	ws := wsList[0].(map[string]any)
	if ws["projectName"] != "Echoes of Nyx" || !ws["hasAssignedWork"].(bool) {
		t.Fatalf("unexpected workspace for Bianca: %+v", ws)
	}
	wsRepos := ws["repositories"].([]any)
	if len(wsRepos) != 1 {
		t.Fatalf("expected 1 Lore repository in workspace, got %d", len(wsRepos))
	}
	loreRepo := wsRepos[0].(map[string]any)
	if loreRepo["remoteUri"] != "lores://lore.roguefairy.net/echoes-of-nyx/art-2d" || loreRepo["loreFullName"] != "echoes-of-nyx/art-2d" || !loreRepo["canWrite"].(bool) {
		t.Fatalf("unexpected Lore repo details: %+v", loreRepo)
	}
	wsArts := ws["artifacts"].([]any)
	if len(wsArts) != 1 || wsArts[0].(map[string]any)["lockedBy"] != "bianca" {
		t.Fatalf("expected Nyx artifact with lockedBy=bianca, got %+v", wsArts)
	}
	sfSlots := ws["storefrontAssets"].([]any)
	if len(sfSlots) == 0 {
		t.Fatal("expected pending Steam graphical capsule slots in Geppetto workspace")
	}
	for _, s := range sfSlots {
		fmtStr := s.(map[string]any)["format"].(string)
		if strings.Contains(fmtStr, "MP4") {
			t.Fatalf("Geppetto 2D workspace should filter out MP4 video trailer slots, got %s", fmtStr)
		}
	}

	// 4. Fail-closed Partner Isolation:
	// Even if a Partner is accidentally placed in "Rogue Fairy Studios", they NEVER inherit internal employee projects!
	misconfiguredPartnerTok := env.token(t, "geppetto", map[string]any{
		"sub":    "sub-ext-partner",
		"email":  "partner@external.studio",
		"groups": []string{"Rogue Fairy Studios", "Rogue Fairy Studios - Partners"},
	})
	_, partnerUnassigned := env.doJSON(t, http.MethodGet, "/v1/me/projects?includeAllKinds=1", misconfiguredPartnerTok, nil)
	if len(partnerUnassigned["projects"].([]any)) != 0 {
		t.Fatalf("fail-closed violation: partner without explicit project grant saw projects: %+v", partnerUnassigned)
	}

	// Partner explicitly in "Rogue Fairy Studios - Secret - Art" sees ONLY Project Veil!
	veilPartnerTok := env.token(t, "geppetto", map[string]any{
		"sub":    "sub-veil-partner",
		"email":  "veil@external.studio",
		"groups": []string{"Rogue Fairy Studios", "Rogue Fairy Studios - Partners", "Rogue Fairy Studios - Secret - Art"},
	})
	_, veilPartnerRes := env.doJSON(t, http.MethodGet, "/v1/me/projects?includeAllKinds=1", veilPartnerTok, nil)
	vpList := veilPartnerRes["projects"].([]any)
	if len(vpList) != 1 || vpList[0].(map[string]any)["id"] != veilID {
		t.Fatalf("expected partner to see ONLY Project Veil, got %+v", vpList)
	}
}

func TestConcurrencyFilesCreditsAndServiceKeys(t *testing.T) {
	env := setupTestEnv(t)
	adminTok := env.token(t, "rfs-intranet", map[string]any{
		"sub":    "sub-admin",
		"email":  "admin@roguefairy.net",
		"groups": []string{"authentik Admins"},
	})

	_, created := env.doJSON(t, http.MethodPost, "/v1/projects", adminTok, map[string]any{
		"name": "Chronos",
		"kind": "digital",
	})
	pid := created["id"].(string)

	// Stale revision must return 409 Conflict
	code, _ := env.doJSON(t, http.MethodPost, "/v1/projects/"+pid+"/commands", adminTok, map[string]any{
		"revision": 99,
		"command":  "team",
		"input":    map[string]any{"name": "Core"},
	})
	if code != http.StatusConflict {
		t.Fatalf("expected 409 on stale revision, got %d", code)
	}

	// Mint a dynamic service key via POST /v1/service-keys
	skCode, skRes := env.doJSON(t, http.MethodPost, "/v1/service-keys", adminTok, map[string]any{
		"name": "geppetto-exporter",
		"role": "editor",
	})
	if skCode != http.StatusCreated {
		t.Fatalf("create service key failed: %d %+v", skCode, skRes)
	}
	secret := skRes["secret"].(string)
	keyID := skRes["key"].(map[string]any)["id"].(string)
	if !strings.HasPrefix(secret, "stk_") {
		t.Fatalf("expected stk_ prefix, got %q", secret)
	}

	// Upload binary file using X-Studio-Key and verify SHA-256 deduplication + CSP headers on download
	payload := []byte("MOCK_PNG_CAPSULE_DATA_12345")
	uploadOnce := func() map[string]any {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "../unsafe/header_capsule.png")
		_, _ = fw.Write(payload)
		_ = mw.Close()

		req, _ := http.NewRequest(http.MethodPost, env.api.URL+"/v1/projects/"+pid+"/files", &buf)
		req.Header.Set("X-Studio-Key", secret)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("upload failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 upload, got %d", resp.StatusCode)
		}
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out
	}
	u1 := uploadOnce()
	u2 := uploadOnce()
	if u1["id"] != u2["id"] || u1["checksum"] != domain.SHA256Hex(payload) {
		t.Fatalf("expected deduplicated file id & sha256 match, got u1=%+v u2=%+v", u1, u2)
	}

	dlReq, _ := http.NewRequest(http.MethodGet, env.api.URL+u1["apiPath"].(string), nil)
	dlReq.Header.Set("X-Studio-Key", secret)
	dlResp, err := http.DefaultClient.Do(dlReq)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	dlBytes, _ := io.ReadAll(dlResp.Body)
	_ = dlResp.Body.Close()
	if dlResp.StatusCode != http.StatusOK || !bytes.Equal(dlBytes, payload) {
		t.Fatalf("unexpected download response: status=%d bytes=%q", dlResp.StatusCode, string(dlBytes))
	}
	if !strings.Contains(dlResp.Header.Get("Content-Disposition"), "header_capsule.png") ||
		dlResp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing security headers on download: %+v", dlResp.Header)
	}

	// Push idempotent integrationEvent via /v1/projects/{id}/events
	evBody, _ := json.Marshal(map[string]any{
		"source":  "geppetto",
		"eventId": "ev-001",
		"type":    "artifact",
		"payload": map[string]any{
			"externalId": "kra-hero",
			"name":       "Chronos Hero",
			"kind":       "Illustration",
			"track":      "art_2d",
		},
	})
	evReq, _ := http.NewRequest(http.MethodPost, env.api.URL+"/v1/projects/"+pid+"/events", bytes.NewReader(evBody))
	evReq.Header.Set("X-Studio-Key", secret)
	evReq.Header.Set("Content-Type", "application/json")
	evResp, err := http.DefaultClient.Do(evReq)
	if err != nil || evResp.StatusCode != http.StatusOK {
		t.Fatalf("event ingestion failed: resp=%+v err=%v", evResp, err)
	}
	_ = evResp.Body.Close()

	// Delete service key and confirm immediate revocation
	delCode, _ := env.doJSON(t, http.MethodDelete, "/v1/service-keys/"+keyID, adminTok, nil)
	if delCode != http.StatusOK {
		t.Fatalf("expected 200 deleting service key, got %d", delCode)
	}
	revokedReq, _ := http.NewRequest(http.MethodGet, env.api.URL+"/v1/me/projects", nil)
	revokedReq.Header.Set("X-Studio-Key", secret)
	revokedResp, _ := http.DefaultClient.Do(revokedReq)
	_ = revokedResp.Body.Close()
	if revokedResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 after revoking service key, got %d", revokedResp.StatusCode)
	}
}

func (e *testEnv) doRaw(t *testing.T, path, bearer, accept string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, e.api.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(raw)
}

func TestGeneralistAndProjectToolConfigEndpoints(t *testing.T) {
	env := setupTestEnv(t)

	adminTok := env.token(t, "rfs-intranet", map[string]any{
		"sub":                "sub-ceo",
		"preferred_username": "ceo",
		"name":               "CEO",
		"email":              "ceo@roguefairy.net",
		"groups":             []string{"Rogue Fairy Studios - C-Level"},
	})
	lucasTok := env.token(t, "aetherion", map[string]any{
		"sub":                "sub-lucas",
		"preferred_username": "lucas",
		"email":              "lucas@roguefairy.net",
		"groups":             []string{"Rogue Fairy Studios"},
	})
	biancaTok := env.token(t, "geppetto", map[string]any{
		"sub":                "sub-bianca",
		"preferred_username": "bianca",
		"email":              "bianca@roguefairy.net",
		"groups":             []string{"Rogue Fairy Studios"},
	})
	partnerTok := env.token(t, "geppetto", map[string]any{
		"sub":    "sub-partner",
		"email":  "ext@partner.io",
		"groups": []string{"Rogue Fairy Studios - Partners"},
	})

	// 1. Generalist catalog & tool configs (Aetherion, Geppetto, Authentik, All) in JSON and YAML
	code, catRes := env.doJSON(t, http.MethodGet, "/v1/configs", lucasTok, nil)
	if code != http.StatusOK || catRes["scope"] != "studio" {
		t.Fatalf("expected studio config catalog, got %d %+v", code, catRes)
	}
	if len(catRes["profiles"].([]any)) < 8 {
		t.Fatalf("expected full generalist profile catalog, got %+v", catRes["profiles"])
	}

	code, aethGen := env.doJSON(t, http.MethodGet, "/v1/configs/aetherion", lucasTok, nil)
	if code != http.StatusOK || aethGen["tool"] != "aetherion" || aethGen["scope"] != "studio" {
		t.Fatalf("generalist aetherion JSON failed: %d %+v", code, aethGen)
	}
	edSettings := aethGen["editorSettings"].(map[string]any)
	if edSettings["network/authentik/client_id"] != "aetherion" || !strings.Contains(edSettings["network/authentik/issuer_url"].(string), "/application/o/aetherion") {
		t.Fatalf("unexpected Aetherion editorSettings: %+v", edSettings)
	}

	yCode, yHdr, gepGenYAML := env.doRaw(t, "/v1/configs/geppetto?format=yaml&download=1", biancaTok, "")
	if yCode != http.StatusOK || !strings.Contains(yHdr.Get("Content-Type"), "application/yaml") {
		t.Fatalf("generalist geppetto YAML failed: %d ct=%s", yCode, yHdr.Get("Content-Type"))
	}
	if !strings.Contains(yHdr.Get("Content-Disposition"), "roguefairy-geppetto.yaml") {
		t.Fatalf("expected attachment filename roguefairy-geppetto.yaml, got %q", yHdr.Get("Content-Disposition"))
	}
	if !strings.Contains(gepGenYAML, "tool: geppetto") ||
		!strings.Contains(gepGenYAML, "lores://lore.roguefairy.net") ||
		!strings.Contains(gepGenYAML, "projectsRoot: \"~/Documents/Geppetto\"") {
		t.Fatalf("unexpected generalist Geppetto YAML:\n%s", gepGenYAML)
	}

	// 2. Create project "Klaus Syndicate" using Aetherion engine
	code, klausCreated := env.doJSON(t, http.MethodPost, "/v1/projects", adminTok, map[string]any{
		"name":   "Klaus Syndicate",
		"kind":   "digital",
		"engine": "aetherion",
	})
	if code != http.StatusCreated {
		t.Fatalf("create Klaus Syndicate failed: %d %+v", code, klausCreated)
	}
	klausID := klausCreated["id"].(string)
	rev := int(klausCreated["revision"].(float64))

	exec := func(cmd string, input map[string]any) {
		c, r := env.doJSON(t, http.MethodPost, "/v1/projects/"+klausID+"/commands", adminTok, map[string]any{
			"revision": rev,
			"command":  cmd,
			"input":    input,
		})
		if c != http.StatusOK {
			t.Fatalf("cmd %s failed: %d %+v", cmd, c, r)
		}
		rev = int(r["revision"].(float64))
	}

	exec("loreConfig", map[string]any{
		"projectSlug":  "klaus-syndicate",
		"visibility":   "internal",
		"employeeRole": "editor",
	})
	exec("repository", map[string]any{
		"name":          "Klaus Syndicate — Aetherion Game",
		"slug":          "klaus-game",
		"system":        "git",
		"remoteUri":     "git@github.com:RogueFairyStudios/klaus-syndicate.git",
		"workspacePath": "client",
	})
	exec("repository", map[string]any{
		"name":     "Klaus Syndicate — 2D Art",
		"system":   "lore",
		"loreRepo": "art",
	})
	exec("repository", map[string]any{
		"name":     "Klaus Syndicate — Animations",
		"system":   "lore",
		"loreRepo": "animations",
	})
	exec("vcsStatus", map[string]any{
		"slug":     "klaus-syndicate-animations",
		"branch":   "main",
		"revision": "42c0ffee",
		"locks": []map[string]any{
			{
				"path":   "characters/klaus_walk.kra",
				"owner":  "bianca",
				"branch": "main",
			},
		},
	})
	exec("platformTarget", map[string]any{
		"storefront": "steam",
		"locale":     "pt",
	})

	// 3. Developer queries "Klaus Syndicate" Aetherion config by project SLUG (`klaus-syndicate`)
	code, klausAeth := env.doJSON(t, http.MethodGet, "/v1/projects/klaus-syndicate/configs/aetherion", lucasTok, nil)
	if code != http.StatusOK {
		t.Fatalf("Klaus Syndicate aetherion config failed: %d %+v", code, klausAeth)
	}
	projMeta := klausAeth["project"].(map[string]any)
	if projMeta["name"] != "Klaus Syndicate" || projMeta["engine"] != "aetherion" || !projMeta["usesAetherion"].(bool) {
		t.Fatalf("unexpected Klaus Syndicate project meta: %+v", projMeta)
	}
	projSettings := klausAeth["projectSettings"].(map[string]any)
	if projSettings["application/config/name"] != "Klaus Syndicate" {
		t.Fatalf("expected project.godot name Klaus Syndicate, got %+v", projSettings)
	}
	wsBlock := klausAeth["workspace"].(map[string]any)
	if wsBlock["primaryWorkspacePath"] != "client" || len(wsBlock["bootstrapCommands"].([]any)) < 3 {
		t.Fatalf("unexpected developer workspace block: %+v", wsBlock)
	}

	// Also verify .yaml extension routing on Aetherion project config
	yCode, yHdr, klausAethYAML := env.doRaw(t, "/v1/projects/klaus-syndicate/configs/aetherion.yaml", lucasTok, "")
	if yCode != http.StatusOK || !strings.Contains(yHdr.Get("Content-Type"), "application/yaml") {
		t.Fatalf("expected YAML for .yaml route, got %d %s", yCode, yHdr.Get("Content-Type"))
	}
	if !strings.Contains(klausAethYAML, "\"network/authentik/studio_api_url\":") ||
		!strings.Contains(klausAethYAML, "Klaus Syndicate") {
		t.Fatalf("unexpected Klaus Aetherion YAML:\n%s", klausAethYAML)
	}

	// 4. Artist / Animator queries "Klaus Syndicate" Geppetto config (JSON + YAML)
	code, klausGep := env.doJSON(t, http.MethodGet, "/v1/projects/klaus-syndicate/configs/geppetto", biancaTok, nil)
	if code != http.StatusOK {
		t.Fatalf("Klaus Syndicate geppetto config failed: %d %+v", code, klausGep)
	}
	accessBlk := klausGep["access"].(map[string]any)
	loreGrant := accessBlk["lore"].(map[string]any)
	if accessBlk["studioRole"] != "editor" || !loreGrant["canWrite"].(bool) {
		t.Fatalf("expected writable Lore & editor Studio access for Bianca, got %+v", accessBlk)
	}
	kvs := klausGep["kritaVersioningSettings"].(map[string]any)
	pEntry := kvs["projectEntry"].(map[string]any)
	if pEntry["projectSlug"] != "klaus-syndicate" || pEntry["accessLevel"] != "read-write" || !pEntry["writable"].(bool) {
		t.Fatalf("unexpected kritaVersioningSettings.projectEntry: %+v", pEntry)
	}
	gepRepos := klausGep["repositories"].([]any)
	if len(gepRepos) != 3 {
		t.Fatalf("expected 3 readable repos (Lore art, Lore animations, Git game), got %d", len(gepRepos))
	}
	// Lore repos come first for Geppetto
	firstRepo := gepRepos[0].(map[string]any)
	if firstRepo["system"] != "lore" || firstRepo["accessLevel"] != "read-write" {
		t.Fatalf("expected Lore repo first with read-write access, got %+v", firstRepo)
	}

	yCode, yHdr, klausGepYAML := env.doRaw(t, "/v1/projects/klaus-syndicate/configs/geppetto.yaml?download=1", biancaTok, "")
	if yCode != http.StatusOK || !strings.Contains(yHdr.Get("Content-Disposition"), "klaus-syndicate-geppetto.yaml") {
		t.Fatalf("expected klaus-syndicate-geppetto.yaml download, got %d %q", yCode, yHdr.Get("Content-Disposition"))
	}
	if !strings.Contains(klausGepYAML, "lores://lore.roguefairy.net/klaus-syndicate/animations") ||
		!strings.Contains(klausGepYAML, "klaus_walk.kra") {
		t.Fatalf("expected Lore animation repo & active .kra lock in Geppetto YAML:\n%s", klausGepYAML)
	}

	// 5. Goethe .gproj config via Accept: application/yaml
	yCode, _, goetheYAML := env.doRaw(t, "/v1/projects/klaus-syndicate/configs/goethe", lucasTok, "application/yaml")
	if yCode != http.StatusOK || !strings.Contains(goetheYAML, "klaus-syndicate.gproj.yaml") {
		t.Fatalf("unexpected Goethe YAML:\n%s", goetheYAML)
	}

	// 6. Fail-closed Partner isolation on project config endpoints (both by UUID and by slug!)
	pCodeSlug, _ := env.doJSON(t, http.MethodGet, "/v1/projects/klaus-syndicate/configs/geppetto", partnerTok, nil)
	pCodeID, _ := env.doJSON(t, http.MethodGet, "/v1/projects/"+klausID+"/configs/geppetto", partnerTok, nil)
	if pCodeSlug != http.StatusNotFound || pCodeID != http.StatusNotFound {
		t.Fatalf("expected 404 for unassigned partner on Klaus Syndicate configs, got slug=%d id=%d", pCodeSlug, pCodeID)
	}
}
