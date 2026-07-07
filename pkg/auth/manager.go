package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Token represents a session JWT.
type Token struct {
	SessionID string
	AgentRole string
	IssuedAt  time.Time
	ExpiresAt time.Time
	Revoked   bool
	Metadata  map[string]string
}

// Manager handles token lifecycle.
type Manager struct {
	mu      sync.RWMutex
	tokens  map[string]*Token
	ttl     time.Duration
	jitter  time.Duration
}

// NewManager creates a token manager with the given TTL.
func NewManager(ttl time.Duration) *Manager {
	return &Manager{
		tokens: make(map[string]*Token),
		ttl:    ttl,
		jitter: 30 * time.Second,
	}
}

// Issue creates a new token for a session.
func (m *Manager) Issue(sessionID, agentRole string, metadata map[string]string) (*Token, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tokens[sessionID]; exists {
		return nil, fmt.Errorf("token already exists for session %s", sessionID)
	}

	now := time.Now()
	token := &Token{
		SessionID: sessionID,
		AgentRole: agentRole,
		IssuedAt:  now,
		ExpiresAt: now.Add(m.ttl),
		Revoked:   false,
		Metadata:  metadata,
	}

	m.tokens[sessionID] = token
	return token, nil
}

// Validate checks if a token is valid.
func (m *Manager) Validate(sessionID string) (*Token, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	token, ok := m.tokens[sessionID]
	if !ok {
		return nil, fmt.Errorf("no token for session %s", sessionID)
	}

	if token.Revoked {
		return nil, fmt.Errorf("token for session %s is revoked", sessionID)
	}

	if time.Now().After(token.ExpiresAt) {
		return nil, fmt.Errorf("token for session %s expired", sessionID)
	}

	return token, nil
}

// Revoke invalidates a token.
func (m *Manager) Revoke(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[sessionID]
	if !ok {
		return fmt.Errorf("no token for session %s", sessionID)
	}

	token.Revoked = true
	return nil
}

// Cleanup removes expired tokens.
func (m *Manager) Cleanup() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	now := time.Now()
	for id, token := range m.tokens {
		if token.Revoked || now.After(token.ExpiresAt.Add(m.jitter)) {
			delete(m.tokens, id)
			count++
		}
	}
	return count
}

// ActiveCount returns the number of active tokens.
func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, token := range m.tokens {
		if !token.Revoked && time.Now().Before(token.ExpiresAt) {
			count++
		}
	}
	return count
}

// GenerateSessionID creates a cryptographically random session ID.
func GenerateSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// SPIREClient is a placeholder for SPIRE SVID retrieval.
type SPIREClient struct {
	spiffeID string
}

// NewSPIREClient creates a new SPIRE client.
func NewSPIREClient(spiffeID string) *SPIREClient {
	return &SPIREClient{spiffeID: spiffeID}
}

// GetSVID returns the current SVID for the workload.
func (c *SPIREClient) GetSVID() (string, error) {
	// In production, this calls the SPIRE Agent API
	return c.spiffeID, nil
}

// ValidateSVID checks if a SVID is valid.
func (c *SPIREClient) ValidateSVID(svid string) bool {
	// In production, this validates the X.509 certificate chain
	return svid != "" && svid == c.spiffeID
}
