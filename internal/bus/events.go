package bus

// EventType represents the type of event.
type EventType string

const (
	// EventMessageReceived is emitted when a new message is received.
	EventMessageReceived EventType = "message.received"

	// EventMessageSent is emitted when a message is sent.
	EventMessageSent EventType = "message.sent"

	// EventSessionCreated is emitted when a new session is created.
	EventSessionCreated EventType = "session.created"

	// EventAgentToolCall is emitted when an agent makes a tool call.
	EventAgentToolCall EventType = "agent.tool_call"

	// EventChannelConnected is emitted when a channel connects.
	EventChannelConnected EventType = "channel.connected"

	// EventTypingStarted is emitted when a typing indicator is sent.
	EventTypingStarted EventType = "typing.started"
)
