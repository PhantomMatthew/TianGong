package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/PhantomMatthew/TianGong/internal/storage/sqlc"
)

// PostgresStore is a PostgreSQL implementation of SessionStore.
// It uses sqlc-generated queries for type-safe database access.
type PostgresStore struct {
	queries *sqlc.Queries
}

// NewPostgresStore creates a new PostgreSQL-backed session store.
// The queries parameter should be initialized with a connection pool.
func NewPostgresStore(queries *sqlc.Queries) *PostgresStore {
	return &PostgresStore{
		queries: queries,
	}
}

// CreateSession creates a new session with the given title.
// Generates a unique session ID using crypto/rand and stores it in PostgreSQL.
func (p *PostgresStore) CreateSession(ctx context.Context, title string) (*Session, error) {
	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	params := sqlc.CreateSessionParams{
		ID:    id,
		Title: title,
	}

	sqlcSession, err := p.queries.CreateSession(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return sqlcSessionToDomain(&sqlcSession)
}

// GetSession retrieves a session by ID.
// Returns ErrSessionNotFound if the session does not exist.
func (p *PostgresStore) GetSession(ctx context.Context, id string) (*Session, error) {
	sqlcSession, err := p.queries.GetSession(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return sqlcSessionToDomain(&sqlcSession)
}

// ListSessions returns all sessions ordered by UpdatedAt in descending order (most recent first).
func (p *PostgresStore) ListSessions(ctx context.Context) ([]*Session, error) {
	sqlcSessions, err := p.queries.ListSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	sessions := make([]*Session, 0, len(sqlcSessions))
	for i := range sqlcSessions {
		session, err := sqlcSessionToDomain(&sqlcSessions[i])
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

// AddMessage adds a message to a session.
// Generates a message ID if not provided and stores it in PostgreSQL.
func (p *PostgresStore) AddMessage(ctx context.Context, sessionID string, msg *Message) error {
	if msg.ID == "" {
		id, err := generateID()
		if err != nil {
			return fmt.Errorf("failed to generate message ID: %w", err)
		}
		msg.ID = id
	}

	// Marshal tool_calls to JSON
	var toolCallsJSON []byte
	if len(msg.ToolCalls) > 0 {
		var err error
		toolCallsJSON, err = json.Marshal(msg.ToolCalls)
		if err != nil {
			return fmt.Errorf("failed to marshal tool calls: %w", err)
		}
	}

	// Handle optional tool_call_id
	var toolCallID pgtype.Text
	if msg.ToolCallID != "" {
		toolCallID = pgtype.Text{String: msg.ToolCallID, Valid: true}
	}

	params := sqlc.AddMessageParams{
		ID:         msg.ID,
		SessionID:  sessionID,
		Role:       string(msg.Role),
		Content:    msg.Content,
		ToolCallID: toolCallID,
		ToolCalls:  toolCallsJSON,
	}

	sqlcMessage, err := p.queries.AddMessage(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to add message: %w", err)
	}

	// Update the input message with database values
	domainMessage, err := sqlcMessageToDomain(&sqlcMessage)
	if err != nil {
		return err
	}

	msg.SessionID = domainMessage.SessionID
	msg.CreatedAt = domainMessage.CreatedAt

	return nil
}

// GetMessages returns all messages for a session in chronological order.
// Returns ErrSessionNotFound if the session does not exist.
func (p *PostgresStore) GetMessages(ctx context.Context, sessionID string) ([]*Message, error) {
	// Check if session exists first
	_, err := p.queries.GetSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to check session existence: %w", err)
	}

	sqlcMessages, err := p.queries.GetMessagesBySession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	messages := make([]*Message, 0, len(sqlcMessages))
	for i := range sqlcMessages {
		message, err := sqlcMessageToDomain(&sqlcMessages[i])
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}

	return messages, nil
}

// sqlcSessionToDomain converts a sqlc Session to a domain Session.
func sqlcSessionToDomain(s *sqlc.Session) (*Session, error) {
	// Parse metadata from JSONB
	metadata := make(map[string]string)
	if len(s.Metadata) > 0 && string(s.Metadata) != "{}" {
		if err := json.Unmarshal(s.Metadata, &metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal session metadata: %w", err)
		}
	}

	return &Session{
		ID:        s.ID,
		Title:     s.Title,
		CreatedAt: s.CreatedAt.Time,
		UpdatedAt: s.UpdatedAt.Time,
		Metadata:  metadata,
	}, nil
}

// sqlcMessageToDomain converts a sqlc Message to a domain Message.
func sqlcMessageToDomain(m *sqlc.Message) (*Message, error) {
	// Parse tool_calls from JSON
	var toolCalls []ToolCall
	if len(m.ToolCalls) > 0 {
		if err := json.Unmarshal(m.ToolCalls, &toolCalls); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tool calls: %w", err)
		}
	}

	// Extract optional tool_call_id
	var toolCallID string
	if m.ToolCallID.Valid {
		toolCallID = m.ToolCallID.String
	}

	return &Message{
		ID:         m.ID,
		SessionID:  m.SessionID,
		Role:       MessageRole(m.Role),
		Content:    m.Content,
		ToolCallID: toolCallID,
		ToolCalls:  toolCalls,
		CreatedAt:  m.CreatedAt.Time,
	}, nil
}

