package whatsapp

import (
	"context"

	"go.mau.fi/whatsmeow/types"
)

// Authenticator handles login mechanisms (Strategy Pattern)
type Authenticator interface {
	GetLoginStatus() (status string, message string)
	GetQRCode() (string, string) // Returns QR code string and Base64 image
	Logout(ctx context.Context) error
	IsConnected() bool
	IsLoggedIn() bool
	Connect() error
}

// MessageSender abstracts the sending mechanism (Strategy Pattern)
type MessageSender interface {
	SendTextMessage(ctx context.Context, to string, message string) (string, error)
	SendMediaMessage(ctx context.Context, to string, data []byte, fileName string, mediaType string, caption string) (string, error)
	SendChatPresence(ctx context.Context, to string, isTyping bool) error
	MarkRead(ctx context.Context, messageID string, chatJID types.JID, senderJID types.JID) error
}

// MessageListener interface to receive events from WhatsApp (Observer Pattern)
type MessageListener interface {
	OnMessageReceived(phone string, message string, isFromMe bool, isWeb bool, timestamp string, msgID string, chatJID types.JID, senderJID types.JID)
	OnLoggedOut()
}