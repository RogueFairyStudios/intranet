// Package store abstracts Studio persistence across PostgreSQL (shared with Wiki.js) and in-memory dev/test stores.
package store

import (
	"context"
	"errors"

	"github.com/roguefairystudios/intranet/studio-server/internal/auth"
)

var (
	// ErrNotFound indicates a requested record does not exist.
	ErrNotFound = errors.New("store: record not found")
	// ErrConflict indicates an optimistic concurrency revision mismatch or duplicate key.
	ErrConflict = errors.New("store: revision conflict")
)

// ProjectRow represents a row in the "studioProjects" table.
type ProjectRow struct {
	ID       string
	Revision int
	State    string
}

// FileRow represents a row in the "studioFiles" table.
type FileRow struct {
	ID        string
	ProjectID string
	Checksum  string
	Name      string
	CreatedAt string
	Data      []byte
}

// WikiGroup represents a group from Wiki.js's "groups" table.
type WikiGroup struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions,omitempty"`
}

// WikiUser represents an account from Wiki.js's "users" table along with its linked groups.
type WikiUser struct {
	ID         int         `json:"id"`
	Email      string      `json:"email"`
	Name       string      `json:"name"`
	ProviderID string      `json:"providerId,omitempty"`
	IsActive   bool        `json:"isActive"`
	Groups     []WikiGroup `json:"groups,omitempty"`
}

// ServiceKeyRecord represents metadata for an issued X-Studio-Key (secret hash never serialized).
type ServiceKeyRecord struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Role      string   `json:"role"`
	Groups    []string `json:"groups"`
	CreatedAt string   `json:"createdAt"`
	CreatedBy string   `json:"createdBy"`
}

// Store defines all persistence operations required by the Studio server.
type Store interface {
	auth.ServiceKeyLookup

	Ping(ctx context.Context) error
	Close()

	// Studio Projects
	ListProjects(ctx context.Context) ([]ProjectRow, error)
	GetProject(ctx context.Context, id string) (*ProjectRow, error)
	CreateProject(ctx context.Context, row ProjectRow) error
	UpdateProject(ctx context.Context, id string, expectedRevision int, newState string) error

	// Studio Files (private SHA-256 deduplicated binary storage)
	SaveFile(ctx context.Context, file FileRow) (FileRow, error)
	GetFile(ctx context.Context, projectID, fileID string) (*FileRow, error)

	// Wiki.js User & Group Directory (gracefully empty if Wiki tables are not present)
	GetWikiUserByID(ctx context.Context, id int) (*WikiUser, error)
	ResolveWikiUser(ctx context.Context, subject, email, username string, id int) (*WikiUser, error)
	SearchWikiUsers(ctx context.Context, search string, limit int) ([]WikiUser, error)
	GetWikiGroupByID(ctx context.Context, id int) (*WikiGroup, error)
	GetWikiGroupByName(ctx context.Context, name string) (*WikiGroup, error)
	SearchWikiGroups(ctx context.Context, search string, limit int) ([]WikiGroup, error)

	// Service Keys
	CreateServiceKey(ctx context.Context, rec ServiceKeyRecord, keyHash string) error
	ListServiceKeys(ctx context.Context) ([]ServiceKeyRecord, error)
	DeleteServiceKey(ctx context.Context, id string) error
}
