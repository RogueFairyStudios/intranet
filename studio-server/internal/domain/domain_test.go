package domain_test

import (
	"context"
	"strings"
	"testing"

	"github.com/roguefairystudios/intranet/studio-server/internal/domain"
	"github.com/roguefairystudios/intranet/studio-server/internal/store"
)

func TestStorefrontReleaseGatingCreditsAndBlueprint(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemory()
	mem.SeedWikiGroup(store.WikiGroup{ID: 7, Name: "Rogue Fairy Studios - Nyx - Art"})

	admin := domain.SubjectProfile{
		Subject:    "sub-admin",
		WikiUserID: 1,
		Name:       "Admin",
		Email:      "admin@roguefairy.net",
		GroupNames: []string{domain.GroupCLevel},
		IsAdmin:    true,
	}

	state, err := domain.CreateState("Echoes of Nyx", "digital", 1)
	if err != nil {
		t.Fatalf("create state: %v", err)
	}

	exec := func(cmd string, input map[string]any) error {
		return domain.ApplyCommand(ctx, mem, state, admin, "manager", cmd, input)
	}

	if err := exec("groupAccess", map[string]any{
		"groupId":           7,
		"role":              "editor",
		"loreActions":       []any{"write", "read", "presign"},
		"loreCanCreateRepo": true,
	}); err != nil {
		t.Fatalf("groupAccess: %v", err)
	}
	bp := domain.ExportAuthentikBlueprint(state)
	if !strings.Contains(bp, "Rogue Fairy Studios - Nyx - Art") || !strings.Contains(bp, "lore_can_create_repo: true") {
		t.Fatalf("unexpected blueprint:\n%s", bp)
	}

	// Setup participant, release, credits, build, validation, and Steam platform target
	if err := exec("participant", map[string]any{
		"name":  "Bianca",
		"roles": []any{"Art Director"},
	}); err != nil {
		t.Fatalf("participant: %v", err)
	}
	pID := state["participants"].([]any)[0].(map[string]any)["id"].(string)

	if err := exec("release", map[string]any{
		"name":     "v1.0.0",
		"criteria": "Gold Master",
	}); err != nil {
		t.Fatalf("release: %v", err)
	}
	relID := state["releases"].([]any)[0].(map[string]any)["id"].(string)

	if err := exec("generateCredits", map[string]any{
		"releaseId":      relID,
		"participantIds": []any{pID},
	}); err != nil {
		t.Fatalf("generateCredits: %v", err)
	}
	snapID := state["credits"].([]any)[0].(map[string]any)["id"].(string)

	if err := exec("approveCredits", map[string]any{"id": snapID}); err != nil {
		t.Fatalf("approveCredits: %v", err)
	}
	md, err := domain.ExportCredits(state, snapID, "markdown")
	if err != nil || !strings.Contains(md, "Bianca") {
		t.Fatalf("exportCredits markdown: err=%v md=%q", err, md)
	}

	if err := exec("build", map[string]any{
		"releaseId":  relID,
		"snapshotId": snapID,
		"name":       "win64-1.0.0",
		"checksum":   strings.Repeat("a", 64),
	}); err != nil {
		t.Fatalf("build: %v", err)
	}

	if err := exec("validation", map[string]any{
		"name":      "QA Smoke",
		"kind":      "test",
		"result":    "passed",
		"releaseId": relID,
	}); err != nil {
		t.Fatalf("validation: %v", err)
	}
	valID := state["validations"].([]any)[0].(map[string]any)["id"].(string)

	if err := exec("platformTarget", map[string]any{
		"storefront": "steam",
		"releaseId":  relID,
		"locale":     "en",
	}); err != nil {
		t.Fatalf("platformTarget: %v", err)
	}
	target := state["platformTargets"].([]any)[0].(map[string]any)
	targetID := target["id"].(string)

	if err := exec("release", map[string]any{
		"id":                relID,
		"name":              "v1.0.0",
		"criteria":          "Gold Master",
		"validationIds":     []any{valID},
		"platformTargetIds": []any{targetID},
	}); err != nil {
		t.Fatalf("update release: %v", err)
	}

	// Publish must fail while Steam checklist items are pending
	if err := exec("publishRelease", map[string]any{"id": relID}); err == nil || !strings.Contains(err.Error(), "Platform compliance checklist incomplete") {
		t.Fatalf("expected checklist gating failure, got %v", err)
	}

	// Mark all checklist items done
	for _, rawItem := range target["checklist"].([]any) {
		item := rawItem.(map[string]any)
		if err := exec("platformCheckItem", map[string]any{
			"targetId": targetID,
			"id":       item["id"],
			"status":   "done",
		}); err != nil {
			t.Fatalf("check item done: %v", err)
		}
	}

	// Publish must still fail while Steam graphical assets are pending
	if err := exec("publishRelease", map[string]any{"id": relID}); err == nil || !strings.Contains(err.Error(), "Storefront assets incomplete") {
		t.Fatalf("expected storefront asset gating failure, got %v", err)
	}

	// Complete all Steam asset slots with file URIs
	for _, rawSlot := range target["assets"].([]any) {
		slot := rawSlot.(map[string]any)
		if err := exec("platformAssetSlot", map[string]any{
			"targetId": targetID,
			"id":       slot["id"],
			"status":   "done",
			"uri":      "https://docs.roguefairy.net/p/studio/files/p1/f1",
		}); err != nil {
			t.Fatalf("asset slot done: %v", err)
		}
	}

	// Now publishRelease succeeds!
	if err := exec("publishRelease", map[string]any{"id": relID}); err != nil {
		t.Fatalf("publishRelease after storefront readiness failed: %v", err)
	}
}
