package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ErrSessionNotFound is returned when a session does not exist.
var ErrSessionNotFound = errors.New("session not found")

// MemoryStore is an in-memory implementation of SessionStore.
// It is safe for concurrent access via sync.RWMutex.
type MemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	messages map[string][]*Message
}

// NewMemoryStore creates a new in-memory session store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions: make(map[string]*Session),
		messages: make(map[string][]*Message),
	}
}

// CreateSession creates a new session with the given title.
// Generates a unique session ID using crypto/rand.
func (m *MemoryStore) CreateSession(ctx context.Context, title string) (*Session, error) {
	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	now := time.Now()
	session := &Session{
		ID:        id,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  make(map[string]string),
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[id] = session
	m.messages[id] = make([]*Message, 0)

	return session, nil
}

// GetSession retrieves a session by ID.
// Returns ErrSessionNotFound if the session does not exist.
func (m *MemoryStore) GetSession(ctx context.Context, id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[id]
	if !exists {
		return nil, ErrSessionNotFound
	}

	return session, nil
}

// ListSessions returns all sessions ordered by UpdatedAt in descending order (most recent first).
func (m *MemoryStore) ListSessions(ctx context.Context) ([]*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}

	// Sort by UpdatedAt descending (most recent first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

// AddMessage adds a message to a session.
// Returns ErrSessionNotFound if the session does not exist.
func (m *MemoryStore) AddMessage(ctx context.Context, sessionID string, msg *Message) error {
	if msg.ID == "" {
		id, err := generateID()
		if err != nil {
			return fmt.Errorf("failed to generate message ID: %w", err)
		}
		msg.ID = id
	}

	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}

	msg.SessionID = sessionID

	m.mu.Lock()
	defer m.mu.Unlock()

	_, sessionExists := m.sessions[sessionID]
	if !sessionExists {
		return ErrSessionNotFound
	}

	// Update session's UpdatedAt timestamp
	m.sessions[sessionID].UpdatedAt = time.Now()

	// Append message to session's message list
	m.messages[sessionID] = append(m.messages[sessionID], msg)

	return nil
}

// GetMessages returns all messages for a session in chronological order.
// Returns ErrSessionNotFound if the session does not exist.
func (m *MemoryStore) GetMessages(ctx context.Context, sessionID string) ([]*Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, sessionExists := m.sessions[sessionID]
	if !sessionExists {
		return nil, ErrSessionNotFound
	}

	messages := m.messages[sessionID]
	// Return a copy to prevent external modification
	result := make([]*Message, len(messages))
	copy(result, messages)

	return result, nil
}

// generateID generates a random hex string of 16 bytes (32 hex characters).
func generateID() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
