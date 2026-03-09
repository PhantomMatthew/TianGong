//go:build integration

package session

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PhantomMatthew/TianGong/internal/storage/sqlc"
)

func setupTestDB(t *testing.T) (*sqlc.Queries, func()) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set, skipping integration tests")
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
	require.NoError(t, err, "failed to connect to database")

	queries := sqlc.New(conn)

	cleanup := func() {
		// Clean up test data
		_, _ = conn.Exec(ctx, "DELETE FROM messages")
		_, _ = conn.Exec(ctx, "DELETE FROM sessions")
		conn.Close(context.Background())
	}

	return queries, cleanup
}

func TestPostgresStoreCreateSession(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewPostgresStore(queries)
	ctx := context.Background()

	session, err := store.CreateSession(ctx, "Test Session")
	require.NoError(t, err)

	assert.NotEmpty(t, session.ID)
	assert.Equal(t, "Test Session", session.Title)
	assert.False(t, session.CreatedAt.IsZero())
	assert.False(t, session.UpdatedAt.IsZero())
	assert.NotNil(t, session.Metadata)
}

func TestPostgresStoreGetSession(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewPostgresStore(queries)
	ctx := context.Background()

	// Create a session
	created, err := store.CreateSession(ctx, "Test Session")
	require.NoError(t, err)

	// Retrieve the session
	retrieved, err := store.GetSession(ctx, created.ID)
	require.NoError(t, err)

	assert.Equal(t, created.ID, retrieved.ID)
	assert.Equal(t, created.Title, retrieved.Title)
	assert.Equal(t, created.CreatedAt.Unix(), retrieved.CreatedAt.Unix())
	assert.Equal(t, created.UpdatedAt.Unix(), retrieved.UpdatedAt.Unix())
}

func TestPostgresStoreGetSessionNotFound(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewPostgresStore(queries)
	ctx := context.Background()

	_, err := store.GetSession(ctx, "nonexistent-id")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestPostgresStoreListSessions(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewPostgresStore(queries)
	ctx := context.Background()

	// Create multiple sessions
	session1, err := store.CreateSession(ctx, "Session 1")
	require.NoError(t, err)

	session2, err := store.CreateSession(ctx, "Session 2")
	require.NoError(t, err)

	session3, err := store.CreateSession(ctx, "Session 3")
	require.NoError(t, err)

	// List sessions
	sessions, err := store.ListSessions(ctx)
	require.NoError(t, err)
	require.Len(t, sessions, 3)

	// Should be ordered by UpdatedAt DESC (most recent first)
	// Since session3 was created last, it should be first
	assert.Equal(t, session3.ID, sessions[0].ID)
	assert.Equal(t, session2.ID, sessions[1].ID)
	assert.Equal(t, session1.ID, sessions[2].ID)
}

func TestPostgresStoreAddMessage(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewPostgresStore(queries)
	ctx := context.Background()

	// Create a session
	session, err := store.CreateSession(ctx, "Test Session")
	require.NoError(t, err)

	// Add a message
	msg := &Message{
		Role:    RoleUser,
		Content: "Hello, world!",
	}

	err = store.AddMessage(ctx, session.ID, msg)
	require.NoError(t, err)

	// Message should have been updated with ID and SessionID
	assert.NotEmpty(t, msg.ID)
	assert.Equal(t, session.ID, msg.SessionID)
	assert.False(t, msg.CreatedAt.IsZero())
}

func TestPostgresStoreAddMessageWithToolCalls(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewPostgresStore(queries)
	ctx := context.Background()

	// Create a session
	session, err := store.CreateSession(ctx, "Test Session")
	require.NoError(t, err)

	// Add a message with tool calls
	msg := &Message{
		Role:    RoleAssistant,
		Content: "Let me search that for you.",
		ToolCalls: []ToolCall{
			{ID: "call_1", Name: "search", Arguments: `{"query":"test"}`},
			{ID: "call_2", Name: "fetch", Arguments: `{"url":"example.com"}`},
		},
	}

	err = store.AddMessage(ctx, session.ID, msg)
	require.NoError(t, err)

	// Retrieve messages and verify tool calls
	messages, err := store.GetMessages(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, messages, 1)

	assert.Equal(t, RoleAssistant, messages[0].Role)
	assert.Len(t, messages[0].ToolCalls, 2)
	assert.Equal(t, "call_1", messages[0].ToolCalls[0].ID)
	assert.Equal(t, "search", messages[0].ToolCalls[0].Name)
}

func TestPostgresStoreAddMessageWithToolCallID(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewPostgresStore(queries)
	ctx := context.Background()

	// Create a session
	session, err := store.CreateSession(ctx, "Test Session")
	require.NoError(t, err)

	// Add a tool result message
	msg := &Message{
		Role:       RoleTool,
		Content:    `{"result":"success"}`,
		ToolCallID: "call_123",
	}

	err = store.AddMessage(ctx, session.ID, msg)
	require.NoError(t, err)

	// Retrieve and verify
	messages, err := store.GetMessages(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, messages, 1)

	assert.Equal(t, RoleTool, messages[0].Role)
	assert.Equal(t, "call_123", messages[0].ToolCallID)
}

func TestPostgresStoreAddMessageToNonexistentSession(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewPostgresStore(queries)
	ctx := context.Background()

	msg := &Message{
		Role:    RoleUser,
		Content: "Hello",
	}

	err := store.AddMessage(ctx, "nonexistent-id", msg)
	assert.Error(t, err)
}

func TestPostgresStoreGetMessages(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewPostgresStore(queries)
	ctx := context.Background()

	// Create a session
	session, err := store.CreateSession(ctx, "Test Session")
	require.NoError(t, err)

	// Add multiple messages
	msg1 := &Message{Role: RoleUser, Content: "Message 1"}
	msg2 := &Message{Role: RoleAssistant, Content: "Message 2"}
	msg3 := &Message{Role: RoleUser, Content: "Message 3"}

	require.NoError(t, store.AddMessage(ctx, session.ID, msg1))
	require.NoError(t, store.AddMessage(ctx, session.ID, msg2))
	require.NoError(t, store.AddMessage(ctx, session.ID, msg3))

	// Retrieve messages
	messages, err := store.GetMessages(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, messages, 3)

	// Should be in chronological order
	assert.Equal(t, "Message 1", messages[0].Content)
	assert.Equal(t, "Message 2", messages[1].Content)
	assert.Equal(t, "Message 3", messages[2].Content)
}

func TestPostgresStoreGetMessagesFromNonexistentSession(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewPostgresStore(queries)
	ctx := context.Background()

	_, err := store.GetMessages(ctx, "nonexistent-id")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestPostgresStoreGetMessagesFromEmptySession(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewPostgresStore(queries)
	ctx := context.Background()

	// Create a session but don't add any messages
	session, err := store.CreateSession(ctx, "Empty Session")
	require.NoError(t, err)

	// Should return empty slice, not error
	messages, err := store.GetMessages(ctx, session.ID)
	require.NoError(t, err)
	assert.Empty(t, messages)
}
