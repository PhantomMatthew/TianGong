package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCreateSession(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	session, err := store.CreateSession(ctx, "Test Session")

	assert.NoError(t, err)
	assert.NotNil(t, session)
	assert.NotEmpty(t, session.ID)
	assert.Equal(t, "Test Session", session.Title)
	assert.False(t, session.CreatedAt.IsZero())
	assert.False(t, session.UpdatedAt.IsZero())
	assert.Equal(t, session.CreatedAt, session.UpdatedAt)
	assert.NotNil(t, session.Metadata)
}

func TestGetSession(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Create a session
	created, err := store.CreateSession(ctx, "Get Test")
	assert.NoError(t, err)

	// Get the session
	retrieved, err := store.GetSession(ctx, created.ID)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, created.ID, retrieved.ID)
	assert.Equal(t, created.Title, retrieved.Title)

	// Try to get non-existent session
	_, err = store.GetSession(ctx, "nonexistent-id")
	assert.Error(t, err)
	assert.Equal(t, ErrSessionNotFound, err)
}

func TestAddAndGetMessages(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Create a session
	session, err := store.CreateSession(ctx, "Message Test")
	assert.NoError(t, err)

	// Add messages
	msg1 := &Message{
		Role:    RoleUser,
		Content: "Hello",
	}
	err = store.AddMessage(ctx, session.ID, msg1)
	assert.NoError(t, err)
	assert.NotEmpty(t, msg1.ID)
	assert.False(t, msg1.CreatedAt.IsZero())
	assert.Equal(t, session.ID, msg1.SessionID)

	time.Sleep(10 * time.Millisecond)

	msg2 := &Message{
		Role:    RoleAssistant,
		Content: "Hi there",
	}
	err = store.AddMessage(ctx, session.ID, msg2)
	assert.NoError(t, err)

	// Retrieve messages
	messages, err := store.GetMessages(ctx, session.ID)
	assert.NoError(t, err)
	assert.Len(t, messages, 2)
	assert.Equal(t, "Hello", messages[0].Content)
	assert.Equal(t, "Hi there", messages[1].Content)

	// Try to add message to non-existent session
	msg3 := &Message{
		Role:    RoleUser,
		Content: "Should fail",
	}
	err = store.AddMessage(ctx, "nonexistent-id", msg3)
	assert.Error(t, err)
	assert.Equal(t, ErrSessionNotFound, err)

	// Try to get messages from non-existent session
	_, err = store.GetMessages(ctx, "nonexistent-id")
	assert.Error(t, err)
	assert.Equal(t, ErrSessionNotFound, err)
}

func TestListSessions(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Create multiple sessions with delays to ensure different UpdatedAt times
	session1, err := store.CreateSession(ctx, "Session 1")
	assert.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	session2, err := store.CreateSession(ctx, "Session 2")
	assert.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	session3, err := store.CreateSession(ctx, "Session 3")
	assert.NoError(t, err)

	// Add a message to session1 to update its UpdatedAt
	msg := &Message{
		Role:    RoleUser,
		Content: "Update timestamp",
	}
	time.Sleep(10 * time.Millisecond)
	err = store.AddMessage(ctx, session1.ID, msg)
	assert.NoError(t, err)

	// List sessions
	sessions, err := store.ListSessions(ctx)
	assert.NoError(t, err)
	assert.Len(t, sessions, 3)

	// Verify sorted by UpdatedAt descending (most recent first)
	assert.Equal(t, session1.ID, sessions[0].ID)
	assert.Equal(t, session3.ID, sessions[1].ID)
	assert.Equal(t, session2.ID, sessions[2].ID)

	// Verify all sessions are present
	ids := make(map[string]bool)
	for _, s := range sessions {
		ids[s.ID] = true
	}
	assert.True(t, ids[session1.ID])
	assert.True(t, ids[session2.ID])
	assert.True(t, ids[session3.ID])
}

func TestListSessionsEmpty(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	sessions, err := store.ListSessions(ctx)
	assert.NoError(t, err)
	assert.Len(t, sessions, 0)
}

func TestSessionIDUniqueness(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	session1, err := store.CreateSession(ctx, "Session 1")
	assert.NoError(t, err)

	session2, err := store.CreateSession(ctx, "Session 2")
	assert.NoError(t, err)

	assert.NotEqual(t, session1.ID, session2.ID)
}

func TestMessagePreservesOrder(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	session, err := store.CreateSession(ctx, "Order Test")
	assert.NoError(t, err)

	// Add 5 messages in sequence
	for i := 1; i <= 5; i++ {
		msg := &Message{
			Role:    RoleUser,
			Content: string(rune('0' + i)),
		}
		err = store.AddMessage(ctx, session.ID, msg)
		assert.NoError(t, err)
	}

	// Verify messages are in chronological order
	messages, err := store.GetMessages(ctx, session.ID)
	assert.NoError(t, err)
	assert.Len(t, messages, 5)

	for i := 0; i < 5; i++ {
		assert.Equal(t, string(rune('1'+rune(i))), messages[i].Content)
	}
}

func TestMessageIDGeneration(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	session, err := store.CreateSession(ctx, "ID Gen Test")
	assert.NoError(t, err)

	msg := &Message{
		Role:    RoleUser,
		Content: "Test",
	}

	err = store.AddMessage(ctx, session.ID, msg)
	assert.NoError(t, err)

	assert.NotEmpty(t, msg.ID)
	assert.Len(t, msg.ID, 32) // 16 bytes in hex = 32 chars
}

func TestConcurrentAccess(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Create initial session
	session, err := store.CreateSession(ctx, "Concurrent Test")
	assert.NoError(t, err)

	// Concurrent message additions
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(index int) {
			msg := &Message{
				Role:    RoleUser,
				Content: string(rune('A' + rune(index))),
			}
			_ = store.AddMessage(ctx, session.ID, msg)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all messages were added
	messages, err := store.GetMessages(ctx, session.ID)
	assert.NoError(t, err)
	assert.Len(t, messages, 10)
}
