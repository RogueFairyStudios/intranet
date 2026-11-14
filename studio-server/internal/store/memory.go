package store

import (
	"context"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/roguefairystudios/intranet/studio-server/internal/auth"
)

type memoryKeyEntry struct {
	rec  ServiceKeyRecord
	hash string
}

// Memory is a thread-safe in-memory Store for unit tests and local development without PostgreSQL.
type Memory struct {
	mu       sync.RWMutex
	projects map[string]ProjectRow
	files    map[string]FileRow
	users    map[int]WikiUser
	groups   map[int]WikiGroup
	keys     map[string]memoryKeyEntry
}

var _ Store = (*Memory)(nil)

// NewMemory creates an empty in-memory Store.
func NewMemory() *Memory {
	return &Memory{
		projects: make(map[string]ProjectRow),
		files:    make(map[string]FileRow),
		users:    make(map[int]WikiUser),
		groups:   make(map[int]WikiGroup),
		keys:     make(map[string]memoryKeyEntry),
	}
}

func (m *Memory) Ping(_ context.Context) error { return nil }
func (m *Memory) Close()                       {}

// SeedWikiUser registers a WikiUser in the memory store for testing.
func (m *Memory) SeedWikiUser(u WikiUser) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[u.ID] = u
}

// SeedWikiGroup registers a WikiGroup in the memory store for testing.
func (m *Memory) SeedWikiGroup(g WikiGroup) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.groups[g.ID] = g
}

func (m *Memory) ListProjects(_ context.Context) ([]ProjectRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ProjectRow, 0, len(m.projects))
	for _, p := range m.projects {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *Memory) GetProject(_ context.Context, id string) (*ProjectRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.projects[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := p
	return &cp, nil
}

func (m *Memory) CreateProject(_ context.Context, row ProjectRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.projects[row.ID]; exists {
		return ErrConflict
	}
	if row.Revision <= 0 {
		row.Revision = 1
	}
	m.projects[row.ID] = row
	return nil
}

func (m *Memory) UpdateProject(_ context.Context, id string, expectedRevision int, newState string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.projects[id]
	if !ok {
		return ErrNotFound
	}
	if cur.Revision != expectedRevision {
		return ErrConflict
	}
	cur.Revision = expectedRevision + 1
	cur.State = newState
	m.projects[id] = cur
	return nil
}

func (m *Memory) SaveFile(_ context.Context, file FileRow) (FileRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.projects[file.ProjectID]; !ok {
		return FileRow{}, ErrNotFound
	}
	for _, existing := range m.files {
		if existing.ProjectID == file.ProjectID && existing.Checksum == file.Checksum {
			return existing, nil
		}
	}
	cp := file
	cp.Data = slices.Clone(file.Data)
	m.files[cp.ID] = cp
	return cp, nil
}

func (m *Memory) GetFile(_ context.Context, projectID, fileID string) (*FileRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.files[fileID]
	if !ok || f.ProjectID != projectID {
		return nil, ErrNotFound
	}
	cp := f
	cp.Data = slices.Clone(f.Data)
	return &cp, nil
}

func (m *Memory) GetWikiUserByID(_ context.Context, id int) (*WikiUser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[id]
	if !ok {
		return nil, nil
	}
	cp := u
	return &cp, nil
}

func (m *Memory) ResolveWikiUser(_ context.Context, subject, email, username string, id int) (*WikiUser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if id > 0 {
		if u, ok := m.users[id]; ok && u.IsActive && u.ID != 2 {
			cp := u
			return &cp, nil
		}
	}
	for _, u := range m.users {
		if !u.IsActive || u.ID == 2 {
			continue
		}
		if subject != "" && u.ProviderID != "" && strings.EqualFold(u.ProviderID, subject) {
			cp := u
			return &cp, nil
		}
		if email != "" && u.Email != "" && strings.EqualFold(u.Email, email) {
			cp := u
			return &cp, nil
		}
		if username != "" && (strings.EqualFold(u.Name, username) || strings.EqualFold(strings.Split(u.Email, "@")[0], username)) {
			cp := u
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *Memory) SearchWikiUsers(_ context.Context, search string, limit int) ([]WikiUser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 {
		limit = 30
	}
	needle := strings.ToLower(strings.TrimSpace(search))
	var out []WikiUser
	for _, u := range m.users {
		if !u.IsActive || u.ID == 2 {
			continue
		}
		if needle == "" || strings.Contains(strings.ToLower(u.Name), needle) || strings.Contains(strings.ToLower(u.Email), needle) {
			out = append(out, u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Memory) GetWikiGroupByID(_ context.Context, id int) (*WikiGroup, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	g, ok := m.groups[id]
	if !ok {
		return nil, nil
	}
	cp := g
	return &cp, nil
}

func (m *Memory) GetWikiGroupByName(_ context.Context, name string) (*WikiGroup, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, g := range m.groups {
		if strings.EqualFold(strings.TrimSpace(g.Name), strings.TrimSpace(name)) {
			cp := g
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *Memory) SearchWikiGroups(_ context.Context, search string, limit int) ([]WikiGroup, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 {
		limit = 30
	}
	needle := strings.ToLower(strings.TrimSpace(search))
	var out []WikiGroup
	for _, g := range m.groups {
		if needle == "" || strings.Contains(strings.ToLower(g.Name), needle) {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Memory) CreateServiceKey(_ context.Context, rec ServiceKeyRecord, keyHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.keys {
		if existing.hash == keyHash || existing.rec.ID == rec.ID {
			return ErrConflict
		}
	}
	m.keys[rec.ID] = memoryKeyEntry{rec: rec, hash: keyHash}
	return nil
}

func (m *Memory) ListServiceKeys(_ context.Context) ([]ServiceKeyRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ServiceKeyRecord, 0, len(m.keys))
	for _, entry := range m.keys {
		out = append(out, entry.rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

func (m *Memory) DeleteServiceKey(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.keys[id]; !ok {
		return ErrNotFound
	}
	delete(m.keys, id)
	return nil
}

func (m *Memory) GetServiceKeyByHash(_ context.Context, hash string) (*auth.StoredServiceKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, entry := range m.keys {
		if entry.hash == hash {
			return &auth.StoredServiceKey{
				ID:     entry.rec.ID,
				Name:   entry.rec.Name,
				Role:   entry.rec.Role,
				Groups: slices.Clone(entry.rec.Groups),
			}, nil
		}
	}
	return nil, nil
}
