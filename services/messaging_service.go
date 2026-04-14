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
	GeminiService  *GeminiService
	LogoutAction   func()
	MessageHistory []models.MessageLog
}

func NewMessagingService(sender whatsapp.MessageSender, geminiService *GeminiService) *MessagingService {
	return &MessagingService{
		Sender:         sender,
		GeminiService:  geminiService,
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

		// Wait for 1 second to simulate typing
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

func (s *MessagingService) SendAIAutoReply(phone string, userMessage string) {
	go func() {
		ctx := context.Background()
		fmt.Printf("[AI-reply] Showing typing and fetching response for %s\n", phone)
		s.Sender.SendChatPresence(ctx, phone, true)

		// Get response from Gemini
		aiResponse, err := s.GeminiService.GetAIResponse(ctx, userMessage)
		if err != nil {
			fmt.Printf("[Error] Gemini failure: %v\n", err)
			s.Sender.SendChatPresence(ctx, phone, false)
			return
		}

		fmt.Printf("[AI-reply] Sending AI response to %s\n", phone)
		_, err = s.Sender.SendTextMessage(ctx, phone, aiResponse)
		if err != nil {
			fmt.Printf("[Error] Failed to send AI auto-reply to %s: %v\n", phone, err)
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

		// Auto-reply logic
		if s.GeminiService != nil {
			// If Gemini is enabled, use it for ALL incoming messages
			s.SendAIAutoReply(phone, message)
		} else {
			// Fallback to simple regex if AI is not configured
			hiRegex := regexp.MustCompile(`^(hi+|hello)$`)
			lowerMsg := strings.ToLower(strings.TrimSpace(message))
			if hiRegex.MatchString(lowerMsg) {
				s.SendAutoReply(phone, "Hello! (AI is currently disabled)")
			}
		}
	}

	s.MessageHistory = append(s.MessageHistory, entry)
}

func (s *MessagingService) OnLoggedOut() {
	if s.LogoutAction != nil {
		s.LogoutAction()
	}
}