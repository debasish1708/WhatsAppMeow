package services

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"whatsmeow/models"
	"whatsmeow/whatsapp"
)

type MessagingService struct {
	Sender         whatsapp.MessageSender
	LogoutAction   func()
	MessageHistory []models.MessageLog
}

func NewMessagingService(sender whatsapp.MessageSender) *MessagingService {
	return &MessagingService{
		Sender:         sender,
		MessageHistory: make([]models.MessageLog, 0),
	}
}

func (s *MessagingService) SendMessage(ctx context.Context, input *models.SendMessageInput) (*models.SendMessageOutput, error) {
	msgID, err := s.Sender.SendTextMessage(ctx, input.Body.Phone, input.Body.Message)
	fmt.Printf("[Outgoing (API)] To %s: %s\n", input.Body.Phone, input.Body.Message)

	if err != nil {
		return nil, err
	}

	output := &models.SendMessageOutput{}
	output.Body.Success = true
	output.Body.MessageID = msgID
	return output, nil
}

func (s *MessagingService) SendMediaMessage(ctx context.Context, phone string, data []byte, fileName string, mediaType string, caption string) (*models.SendMediaMessageOutput, error) {
	msgID, err := s.Sender.SendMediaMessage(ctx, phone, data, fileName, mediaType, caption)
	fmt.Printf("[Outgoing (API) Media] To %s: %s - %s\n", phone, mediaType, fileName)

	var localFileName string
	timestamp := time.Now().Unix()

	switch mediaType {
		case "image":
			localFileName = fmt.Sprintf("media/images/sent_%d.jpg", timestamp)
		case "video":
			localFileName = fmt.Sprintf("media/videos/sent_%d.mp4", timestamp)
		default:
			origName := "document.file"
			if fileName != "" {
				origName = fileName
			}
			localFileName = fmt.Sprintf("media/documents/sent_%d_%s", timestamp, origName)
	}

	errWrite := os.WriteFile(localFileName, data, 0644)
	if errWrite != nil {
		fmt.Printf("[Error] Failed to save local media file: %v\n", errWrite)
	} else {
		fmt.Printf("[Outgoing (API) Media] To %s: %s - saved to %s\n", phone, mediaType, localFileName)
	}

	if err != nil {
		return nil, err
	}
	
	output := &models.SendMediaMessageOutput{}
	output.Body.Success = true
	output.Body.MessageID = msgID
	return output, nil
}

func (s *MessagingService) GetHistory() []models.MessageLog {
	return s.MessageHistory
}

func (s *MessagingService) SendAutoReply(phone string, replyText string) {
	go func() {
		ctx := context.Background()
		fmt.Printf("[Auto-reply] Showing typing to %s\n", phone)
		s.Sender.SendChatPresence(ctx, phone, true)

		// Wait for 2 seconds to simulate typing
		time.Sleep(1 * time.Second)

		fmt.Printf("[Auto-reply] Sending %s to %s\n", replyText, phone)
		_, err := s.Sender.SendTextMessage(ctx, phone, replyText)
		if err != nil {
			fmt.Printf("[Error] Failed to send auto-reply to %s: %v\n", phone, err)
		}

		// Stop typing status
		s.Sender.SendChatPresence(ctx, phone, false)
	}()
}

func (s *MessagingService) OnMessageReceived(phone string, message string, isFromMe bool, isWeb bool, timestamp string) {
	entry := models.MessageLog{
		Phone:     phone,
		Message:   message,
		Timestamp: timestamp,
	}

	origin := "Mobile"
	if isWeb {
		origin = "Web"
	}

	if isFromMe {
		entry.Type = "sent"
		fmt.Printf("[Outgoing (%s)] To %s: %s\n", origin, entry.Phone, entry.Message)
	} else {
		entry.Type = "received"
		fmt.Printf("[Incoming (%s)] From %s: %s\n", origin, entry.Phone, entry.Message)

		// Auto-reply logic using Regex
		// Matches: hi, hii, hiii..., hello (case-insensitive due to lowerMsg)
		hiRegex := regexp.MustCompile(`^(hi+|hello)$`)
		lowerMsg := strings.ToLower(strings.TrimSpace(message))
		if hiRegex.MatchString(lowerMsg) && entry.Phone == "918260646245" {
			s.SendAutoReply(phone, "Hello")
		}
	}

	s.MessageHistory = append(s.MessageHistory, entry)
}

func (s *MessagingService) OnLoggedOut() {
	if s.LogoutAction != nil {
		s.LogoutAction()
	}
}