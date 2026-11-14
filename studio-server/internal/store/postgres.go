package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/roguefairystudios/intranet/studio-server/internal/auth"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// studioMigrationLockID serialises migration runs across replicas without colliding with Wiki.js or Fontpit.
const studioMigrationLockID int64 = 0x72667373747564 // "rfsstud"

// Postgres implements Store against the shared roguefairy PostgreSQL database.
type Postgres struct {
	pool *pgxpool.Pool
}

var _ Store = (*Postgres)(nil)

// OpenPostgres connects to url and ensures the Studio tables exist.
func OpenPostgres(ctx context.Context, url string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("store: open postgres: %w", err)
	}
	if err := migratePostgres(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }
func (p *Postgres) Close()                         { p.pool.Close() }

func migratePostgres(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("store: acquire migration conn: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", studioMigrationLockID); err != nil {
		return fmt.Errorf("store: advisory lock: %w", err)
	}
	defer func() {
		uctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(uctx, "SELECT pg_advisory_unlock($1)", studioMigrationLockID); err != nil {
			_ = conn.Conn().Close(uctx)
		}
	}()

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS "studioServiceMigrations" ("name" text PRIMARY KEY, "appliedAt" timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("store: create migrations table: %w", err)
	}

	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("store: read migrations: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var applied bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "studioServiceMigrations" WHERE "name" = $1)`, name).Scan(&applied); err != nil {
			return fmt.Errorf("store: check migration %s: %w", name, err)
		}
		if applied {
			continue
		}
		sqlBytes, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("store: read migration %s: %w", name, err)
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("store: begin migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("store: apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO "studioServiceMigrations" ("name") VALUES ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("store: record migration %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("store: commit migration %s: %w", name, err)
		}
	}
	return nil
}

func (p *Postgres) ListProjects(ctx context.Context) ([]ProjectRow, error) {
	rows, err := p.pool.Query(ctx, `SELECT "id", "revision", "state" FROM "studioProjects" ORDER BY "id" ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProjectRow
	for rows.Next() {
		var r ProjectRow
		if err := rows.Scan(&r.ID, &r.Revision, &r.State); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *Postgres) GetProject(ctx context.Context, id string) (*ProjectRow, error) {
	var r ProjectRow
	err := p.pool.QueryRow(ctx, `SELECT "id", "revision", "state" FROM "studioProjects" WHERE "id" = $1`, id).Scan(&r.ID, &r.Revision, &r.State)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (p *Postgres) CreateProject(ctx context.Context, row ProjectRow) error {
	if row.Revision <= 0 {
		row.Revision = 1
	}
	_, err := p.pool.Exec(ctx, `INSERT INTO "studioProjects" ("id", "revision", "state") VALUES ($1, $2, $3)`, row.ID, row.Revision, row.State)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (p *Postgres) UpdateProject(ctx context.Context, id string, expectedRevision int, newState string) error {
	tag, err := p.pool.Exec(ctx, `UPDATE "studioProjects" SET "state" = $1, "revision" = $2 WHERE "id" = $3 AND "revision" = $4`, newState, expectedRevision+1, id, expectedRevision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var exists bool
	if err := p.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "studioProjects" WHERE "id" = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return ErrConflict
}

func (p *Postgres) SaveFile(ctx context.Context, file FileRow) (FileRow, error) {
	var existingID, existingName, existingCreatedAt string
	err := p.pool.QueryRow(ctx, `SELECT "id", "name", "createdAt" FROM "studioFiles" WHERE "projectId" = $1 AND "checksum" = $2`, file.ProjectID, file.Checksum).Scan(&existingID, &existingName, &existingCreatedAt)
	if err == nil {
		return FileRow{
			ID:        existingID,
			ProjectID: file.ProjectID,
			Checksum:  file.Checksum,
			Name:      existingName,
			CreatedAt: existingCreatedAt,
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return FileRow{}, err
	}

	_, err = p.pool.Exec(ctx, `INSERT INTO "studioFiles" ("id", "projectId", "checksum", "name", "createdAt", "data") VALUES ($1, $2, $3, $4, $5, $6)`,
		file.ID, file.ProjectID, file.Checksum, file.Name, file.CreatedAt, file.Data)
	if err != nil {
		if isUniqueViolation(err) {
			if qErr := p.pool.QueryRow(ctx, `SELECT "id", "name", "createdAt" FROM "studioFiles" WHERE "projectId" = $1 AND "checksum" = $2`, file.ProjectID, file.Checksum).Scan(&existingID, &existingName, &existingCreatedAt); qErr == nil {
				return FileRow{
					ID:        existingID,
					ProjectID: file.ProjectID,
					Checksum:  file.Checksum,
					Name:      existingName,
					CreatedAt: existingCreatedAt,
				}, nil
			}
		}
		if isForeignKeyViolation(err) {
			return FileRow{}, ErrNotFound
		}
		return FileRow{}, err
	}
	return FileRow{
		ID:        file.ID,
		ProjectID: file.ProjectID,
		Checksum:  file.Checksum,
		Name:      file.Name,
		CreatedAt: file.CreatedAt,
	}, nil
}

func (p *Postgres) GetFile(ctx context.Context, projectID, fileID string) (*FileRow, error) {
	var f FileRow
	err := p.pool.QueryRow(ctx, `SELECT "id", "projectId", "checksum", "name", "createdAt", "data" FROM "studioFiles" WHERE "id" = $1 AND "projectId" = $2`, fileID, projectID).
		Scan(&f.ID, &f.ProjectID, &f.Checksum, &f.Name, &f.CreatedAt, &f.Data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (p *Postgres) hasWikiTables(ctx context.Context) bool {
	var has bool
	err := p.pool.QueryRow(ctx, `SELECT to_regclass('public."users"') IS NOT NULL AND to_regclass('public."groups"') IS NOT NULL`).Scan(&has)
	return err == nil && has
}

func (p *Postgres) loadUserGroups(ctx context.Context, userID int) ([]WikiGroup, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT g."id", g."name", COALESCE(g."permissions"::text, '[]')
		FROM "groups" g
		INNER JOIN "userGroups" ug ON ug."groupId" = g."id"
		WHERE ug."userId" = $1
		ORDER BY g."id" ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []WikiGroup
	for rows.Next() {
		var g WikiGroup
		var rawPerms string
		if err := rows.Scan(&g.ID, &g.Name, &rawPerms); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(rawPerms), &g.Permissions)
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (p *Postgres) GetWikiUserByID(ctx context.Context, id int) (*WikiUser, error) {
	if id <= 0 || !p.hasWikiTables(ctx) {
		return nil, nil
	}
	var u WikiUser
	err := p.pool.QueryRow(ctx, `SELECT "id", COALESCE("email", ''), COALESCE("name", ''), COALESCE("providerId", ''), COALESCE("isActive", false) FROM "users" WHERE "id" = $1`, id).
		Scan(&u.ID, &u.Email, &u.Name, &u.ProviderID, &u.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	groups, err := p.loadUserGroups(ctx, u.ID)
	if err == nil {
		u.Groups = groups
	}
	return &u, nil
}

func (p *Postgres) ResolveWikiUser(ctx context.Context, subject, email, username string, id int) (*WikiUser, error) {
	if !p.hasWikiTables(ctx) {
		return nil, nil
	}
	if id > 0 {
		u, err := p.GetWikiUserByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if u != nil && u.IsActive && u.ID != 2 {
			return u, nil
		}
	}

	subject = strings.TrimSpace(subject)
	email = strings.TrimSpace(email)
	username = strings.TrimSpace(username)

	var matchedID int
	if subject != "" {
		_ = p.pool.QueryRow(ctx, `SELECT "id" FROM "users" WHERE "isActive" = true AND "id" <> 2 AND LOWER("providerId") = LOWER($1) LIMIT 1`, subject).Scan(&matchedID)
	}
	if matchedID == 0 && email != "" {
		_ = p.pool.QueryRow(ctx, `SELECT "id" FROM "users" WHERE "isActive" = true AND "id" <> 2 AND LOWER("email") = LOWER($1) LIMIT 1`, email).Scan(&matchedID)
	}
	if matchedID == 0 && username != "" {
		_ = p.pool.QueryRow(ctx, `SELECT "id" FROM "users" WHERE "isActive" = true AND "id" <> 2 AND (LOWER("name") = LOWER($1) OR LOWER(SPLIT_PART("email", '@', 1)) = LOWER($1)) LIMIT 1`, username).Scan(&matchedID)
	}
	if matchedID == 0 {
		return nil, nil
	}
	return p.GetWikiUserByID(ctx, matchedID)
}

func (p *Postgres) SearchWikiUsers(ctx context.Context, search string, limit int) ([]WikiUser, error) {
	if !p.hasWikiTables(ctx) {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	pattern := searchLikePattern(search)
	rows, err := p.pool.Query(ctx, `
		SELECT "id", COALESCE("email", ''), COALESCE("name", ''), COALESCE("providerId", ''), COALESCE("isActive", false)
		FROM "users"
		WHERE "isActive" = true AND "id" <> 2 AND (LOWER("name") LIKE $1 OR LOWER("email") LIKE $1)
		ORDER BY "name" ASC
		LIMIT $2`, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WikiUser
	for rows.Next() {
		var u WikiUser
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.ProviderID, &u.IsActive); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (p *Postgres) GetWikiGroupByID(ctx context.Context, id int) (*WikiGroup, error) {
	if id <= 0 || !p.hasWikiTables(ctx) {
		return nil, nil
	}
	var g WikiGroup
	var rawPerms string
	err := p.pool.QueryRow(ctx, `SELECT "id", "name", COALESCE("permissions"::text, '[]') FROM "groups" WHERE "id" = $1`, id).
		Scan(&g.ID, &g.Name, &rawPerms)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(rawPerms), &g.Permissions)
	return &g, nil
}

func (p *Postgres) GetWikiGroupByName(ctx context.Context, name string) (*WikiGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" || !p.hasWikiTables(ctx) {
		return nil, nil
	}
	var g WikiGroup
	var rawPerms string
	err := p.pool.QueryRow(ctx, `SELECT "id", "name", COALESCE("permissions"::text, '[]') FROM "groups" WHERE LOWER("name") = LOWER($1) LIMIT 1`, name).
		Scan(&g.ID, &g.Name, &rawPerms)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(rawPerms), &g.Permissions)
	return &g, nil
}

func (p *Postgres) SearchWikiGroups(ctx context.Context, search string, limit int) ([]WikiGroup, error) {
	if !p.hasWikiTables(ctx) {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	pattern := searchLikePattern(search)
	rows, err := p.pool.Query(ctx, `SELECT "id", "name" FROM "groups" WHERE LOWER("name") LIKE $1 ORDER BY "name" ASC LIMIT $2`, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WikiGroup
	for rows.Next() {
		var g WikiGroup
		if err := rows.Scan(&g.ID, &g.Name); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (p *Postgres) CreateServiceKey(ctx context.Context, rec ServiceKeyRecord, keyHash string) error {
	groupsJSON, err := json.Marshal(rec.Groups)
	if err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx, `INSERT INTO "studioServiceKeys" ("id", "name", "role", "keyHash", "groupsJson", "createdAt", "createdBy") VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		rec.ID, rec.Name, rec.Role, keyHash, string(groupsJSON), rec.CreatedAt, rec.CreatedBy)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (p *Postgres) ListServiceKeys(ctx context.Context) ([]ServiceKeyRecord, error) {
	rows, err := p.pool.Query(ctx, `SELECT "id", "name", "role", "groupsJson", "createdAt", "createdBy" FROM "studioServiceKeys" ORDER BY "createdAt" DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ServiceKeyRecord
	for rows.Next() {
		var rec ServiceKeyRecord
		var rawGroups string
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.Role, &rawGroups, &rec.CreatedAt, &rec.CreatedBy); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(rawGroups), &rec.Groups)
		if rec.Groups == nil {
			rec.Groups = []string{}
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (p *Postgres) DeleteServiceKey(ctx context.Context, id string) error {
	tag, err := p.pool.Exec(ctx, `DELETE FROM "studioServiceKeys" WHERE "id" = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) GetServiceKeyByHash(ctx context.Context, hash string) (*auth.StoredServiceKey, error) {
	var sk auth.StoredServiceKey
	var rawGroups string
	err := p.pool.QueryRow(ctx, `SELECT "id", "name", "role", "groupsJson" FROM "studioServiceKeys" WHERE "keyHash" = $1`, hash).
		Scan(&sk.ID, &sk.Name, &sk.Role, &rawGroups)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(rawGroups), &sk.Groups)
	return &sk, nil
}

func searchLikePattern(raw string) string {
	clean := strings.ToLower(strings.TrimSpace(raw))
	if len(clean) > 100 {
		clean = clean[:100]
	}
	clean = strings.ReplaceAll(clean, "%", "")
	clean = strings.ReplaceAll(clean, "_", "")
	return "%" + clean + "%"
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
