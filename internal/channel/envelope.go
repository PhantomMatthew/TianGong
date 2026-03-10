package channel

import (
	"time"
)

// InboundMessage represents a message received from a channel.
type InboundMessage struct {
	// ID is the platform-specific message identifier.
	ID string `json:"id"`
	// ChannelType identifies which platform sent this message.
	ChannelType ChannelType `json:"channel_type"`
	// ChannelName is the name of the channel instance.
	ChannelName string `json:"channel_name"`
	// SenderID is the platform-specific user identifier.
	SenderID string `json:"sender_id"`
	// SenderName is the display name of the sender.
	SenderName string `json:"sender_name"`
	// Content is the text content of the message.
	Content string `json:"content"`
	// ThreadID is the thread/conversation identifier (if applicable).
	ThreadID string `json:"thread_id,omitempty"`
	// ReplyToID is the ID of the message being replied to (if applicable).
	ReplyToID string `json:"reply_to_id,omitempty"`
	// Attachments contains any media attachments.
	Attachments []Attachment `json:"attachments,omitempty"`
	// Timestamp is when the message was sent.
	Timestamp time.Time `json:"timestamp"`
	// Metadata holds platform-specific extra data.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// OutboundMessage represents a message to be sent to a channel.
type OutboundMessage struct {
	// Content is the text content of the message.
	Content string `json:"content"`
	// ChannelType identifies which platform to send to.
	ChannelType ChannelType `json:"channel_type"`
	// ChannelName is the name of the channel instance.
	ChannelName string `json:"channel_name"`
	// RecipientID is the platform-specific recipient identifier (user/chat/room).
	RecipientID string `json:"recipient_id"`
	// ThreadID is the thread to reply in (if applicable).
	ThreadID string `json:"thread_id,omitempty"`
	// ReplyToID is the message to reply to (if applicable).
	ReplyToID string `json:"reply_to_id,omitempty"`
	// Attachments contains any media attachments.
	Attachments []Attachment `json:"attachments,omitempty"`
	// Metadata holds platform-specific extra data.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// AttachmentType identifies the kind of attachment.
type AttachmentType string

const (
	// AttachmentImage represents an image file.
	AttachmentImage AttachmentType = "image"
	// AttachmentAudio represents an audio file.
	AttachmentAudio AttachmentType = "audio"
	// AttachmentVideo represents a video file.
	AttachmentVideo AttachmentType = "video"
	// AttachmentDocument represents a document/file.
	AttachmentDocument AttachmentType = "document"
	// AttachmentVoice represents a voice message.
	AttachmentVoice AttachmentType = "voice"
)

// Attachment represents a media attachment on a message.
type Attachment struct {
	// Type is the kind of attachment.
	Type AttachmentType `json:"type"`
	// URL is the download URL for the attachment.
	URL string `json:"url,omitempty"`
	// Data contains raw attachment data (for inline attachments).
	Data []byte `json:"data,omitempty"`
	// MIMEType is the MIME type of the attachment.
	MIMEType string `json:"mime_type,omitempty"`
	// Filename is the original filename.
	Filename string `json:"filename,omitempty"`
	// Size is the file size in bytes.
	Size int64 `json:"size,omitempty"`
}
