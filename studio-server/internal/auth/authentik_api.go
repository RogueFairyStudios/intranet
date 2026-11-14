package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// AuthentikUser represents a user record fetched from Tellurian Authentik Core API (/api/v3/core/users/).
type AuthentikUser struct {
	PK       int      `json:"pk"`
	UUID     string   `json:"uuid"`
	Username string   `json:"username"`
	Name     string   `json:"name"`
	Email    string   `json:"email"`
	IsActive bool     `json:"is_active"`
	Groups   []string `json:"groups_names"`
}

// DirectoryClient resolves users and groups from Tellurian Authentik's Core API when configured.
type DirectoryClient interface {
	LookupUser(ctx context.Context, identifier string) (*AuthentikUser, error)
}

// AuthentikAPIClient queries Tellurian Authentik /api/v3/core/users/ with a short TTL cache.
type AuthentikAPIClient struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client

	mu    sync.RWMutex
	cache map[string]cachedAuthentikUser
}

type cachedAuthentikUser struct {
	user      *AuthentikUser
	expiresAt time.Time
}

const directoryCacheTTL = 60 * time.Second

// NewAuthentikAPIClient creates an optional Core API client. Returns nil if baseURL or apiToken is empty.
func NewAuthentikAPIClient(baseURL, apiToken string, hc *http.Client) *AuthentikAPIClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiToken = strings.TrimSpace(apiToken)
	if baseURL == "" || apiToken == "" {
		return nil
	}
	if hc == nil {
		hc = &http.Client{Timeout: 8 * time.Second}
	}
	return &AuthentikAPIClient{
		baseURL:    baseURL,
		apiToken:   apiToken,
		httpClient: hc,
		cache:      make(map[string]cachedAuthentikUser),
	}
}

type authentikUsersResponse struct {
	Results []struct {
		PK        int    `json:"pk"`
		UUID      string `json:"uuid"`
		Username  string `json:"username"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		IsActive  bool   `json:"is_active"`
		GroupsObj []struct {
			Name string `json:"name"`
		} `json:"groups_obj"`
	} `json:"results"`
}

// LookupUser finds an active Tellurian Authentik user by username, email, or UUID/sub.
func (c *AuthentikAPIClient) LookupUser(ctx context.Context, identifier string) (*AuthentikUser, error) {
	if c == nil {
		return nil, nil
	}
	key := strings.ToLower(strings.TrimSpace(identifier))
	if key == "" {
		return nil, nil
	}

	c.mu.RLock()
	if entry, ok := c.cache[key]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.RUnlock()
		return entry.user, nil
	}
	c.mu.RUnlock()

	u, err := url.Parse(c.baseURL + "/api/v3/core/users/")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("search", strings.TrimSpace(identifier))
	q.Set("include_groups", "true")
	q.Set("page_size", "20")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("authentik api: %w", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("authentik api status %d", resp.StatusCode)
	}
	if readErr != nil {
		return nil, readErr
	}

	var parsed authentikUsersResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}

	var matched *AuthentikUser
	for _, item := range parsed.Results {
		if !item.IsActive {
			continue
		}
		exact := strings.EqualFold(item.Username, identifier) ||
			strings.EqualFold(item.Email, identifier) ||
			strings.EqualFold(item.UUID, identifier) ||
			fmt.Sprintf("%d", item.PK) == strings.TrimSpace(identifier)
		if !exact {
			continue
		}
		groups := make([]string, 0, len(item.GroupsObj))
		for _, g := range item.GroupsObj {
			if strings.TrimSpace(g.Name) != "" {
				groups = append(groups, strings.TrimSpace(g.Name))
			}
		}
		matched = &AuthentikUser{
			PK:       item.PK,
			UUID:     item.UUID,
			Username: item.Username,
			Name:     item.Name,
			Email:    item.Email,
			IsActive: item.IsActive,
			Groups:   groups,
		}
		break
	}

	c.mu.Lock()
	c.cache[key] = cachedAuthentikUser{
		user:      matched,
		expiresAt: time.Now().Add(directoryCacheTTL),
	}
	c.mu.Unlock()
	return matched, nil
}
