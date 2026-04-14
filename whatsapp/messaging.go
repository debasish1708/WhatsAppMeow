package whatsapp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

type DefaultMessageSender struct {
	Adapter *WhatsAppAdapter
}

func NewDefaultMessageSender(adapter *WhatsAppAdapter) *DefaultMessageSender {
	return &DefaultMessageSender{
		Adapter: adapter,
	}
}

// Factory method for creating message payload (Strategy/Builder)
func buildTextMessage(text string) *waProto.Message {
	return &waProto.Message{
		Conversation: proto.String(text),
	}
}

func (s *DefaultMessageSender) SendTextMessage(ctx context.Context, to string, message string) (string, error) {
	if !s.Adapter.Client.IsConnected() || !s.Adapter.Client.IsLoggedIn() {
		return "", fmt.Errorf("WhatsApp is not logged in or connected")
	}

	jidStr := to
	if !strings.Contains(jidStr, "@") {
		jidStr = jidStr + "@s.whatsapp.net"
	}

	targetJID, err := types.ParseJID(jidStr)
	if err != nil {
		return "", fmt.Errorf("invalid phone number format")
	}

	msgPayload := buildTextMessage(message)

	resp, err := s.Adapter.Client.SendMessage(ctx, targetJID, msgPayload)
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}

func (s *DefaultMessageSender) SendChatPresence(ctx context.Context, to string, isTyping bool) error {
	if !s.Adapter.Client.IsConnected() || !s.Adapter.Client.IsLoggedIn() {
		return fmt.Errorf("WhatsApp is not logged in or connected")
	}

	jidStr := to
	if !strings.Contains(jidStr, "@") {
		jidStr = jidStr + "@s.whatsapp.net"
	}

	targetJID, err := types.ParseJID(jidStr)
	if err != nil {
		return fmt.Errorf("invalid phone number format")
	}

	presence := types.ChatPresencePaused
	if isTyping {
		presence = types.ChatPresenceComposing
	}

	return s.Adapter.Client.SendChatPresence(ctx, targetJID, presence, types.ChatPresenceMediaText)
}

func (s *DefaultMessageSender) SendMediaMessage(ctx context.Context, to string, data []byte, fileName string, mediaType string, caption string) (string, error) {
	if !s.Adapter.Client.IsConnected() || !s.Adapter.Client.IsLoggedIn() {
		return "", fmt.Errorf("WhatsApp is not logged in or connected")
	}

	jidStr := to
	if !strings.Contains(jidStr, "@") {
		jidStr = jidStr + "@s.whatsapp.net"
	}

	targetJID, err := types.ParseJID(jidStr)
	if err != nil {
		return "", fmt.Errorf("invalid phone number format")
	}

	var waMediaType whatsmeow.MediaType
	switch mediaType {
		case "image":
			waMediaType = whatsmeow.MediaImage
		case "document":
			waMediaType = whatsmeow.MediaDocument
		case "video":
			waMediaType = whatsmeow.MediaVideo
		default:
			return "", fmt.Errorf("unsupported media type")
	}

	uploaded, err := s.Adapter.Client.Upload(ctx, data, waMediaType)
	if err != nil {
		return "", fmt.Errorf("failed to upload media: %v", err)
	}

	msgPayload := &waProto.Message{}

	// Simple mime type detection
	mimeType := http.DetectContentType(data)

	switch mediaType {
		case "image":
				msgPayload.ImageMessage = &waProto.ImageMessage{
					Caption:       proto.String(caption),
					Mimetype:      proto.String(mimeType),
					URL:           &uploaded.URL,
					DirectPath:    &uploaded.DirectPath,
					MediaKey:      uploaded.MediaKey,
					FileEncSHA256: uploaded.FileEncSHA256,
					FileSHA256:    uploaded.FileSHA256,
					FileLength:    &uploaded.FileLength,
				}
			case "document":
				msgPayload.DocumentMessage = &waProto.DocumentMessage{
					Title:         proto.String(fileName),
					FileName:      proto.String(fileName),
					Mimetype:      proto.String(mimeType),
					URL:           &uploaded.URL,
					DirectPath:    &uploaded.DirectPath,
					MediaKey:      uploaded.MediaKey,
					FileEncSHA256: uploaded.FileEncSHA256,
					FileSHA256:    uploaded.FileSHA256,
					FileLength:    &uploaded.FileLength,
				}
			case "video":
				msgPayload.VideoMessage = &waProto.VideoMessage{
					Caption:       proto.String(caption),
					Mimetype:      proto.String(mimeType),
					URL:           &uploaded.URL,
					DirectPath:    &uploaded.DirectPath,
					MediaKey:      uploaded.MediaKey,
					FileEncSHA256: uploaded.FileEncSHA256,
					FileSHA256:    uploaded.FileSHA256,
					FileLength:    &uploaded.FileLength,
				}
	}

	resp, err := s.Adapter.Client.SendMessage(ctx, targetJID, msgPayload)
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}