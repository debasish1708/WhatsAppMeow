package services

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"whatsmeow/models"
	"whatsmeow/whatsapp"
)

type MessagingService struct {
	Sender        whatsapp.MessageSender
	GeminiService *GeminiService
	LogoutAction  func()
	DB            *sql.DB
}

func NewMessagingService(sender whatsapp.MessageSender, geminiService *GeminiService, db *sql.DB) *MessagingService {
	return &MessagingService{
		Sender:        sender,
		GeminiService: geminiService,
		DB:            db,
	}
}

func (s *MessagingService) SaveMessage(phone string, message string, msgType string) {
	_, err := s.DB.Exec("INSERT INTO message_history (phone, message, type) VALUES (?, ?, ?)", phone, message, msgType)
	if err != nil {
		fmt.Printf("[Error] Failed to save message to DB: %v\n", err)
	}
}

func (s *MessagingService) GetRecentMessages(phone string, limit int) []models.MessageLog {
	rows, err := s.DB.Query("SELECT phone, message, type, timestamp FROM message_history WHERE phone = ? ORDER BY id DESC LIMIT ?", phone, limit)
	if err != nil {
		fmt.Printf("[Error] Failed to fetch history from DB: %v\n", err)
		return nil
	}
	defer rows.Close()

	var history []models.MessageLog
	for rows.Next() {
		var m models.MessageLog
		if err := rows.Scan(&m.Phone, &m.Message, &m.Type, &m.Timestamp); err != nil {
			fmt.Printf("[Error] Failed to scan history row: %v\n", err)
			continue
		}
		// Insert at the beginning to keep chronological order (since we queried DESC)
		history = append([]models.MessageLog{m}, history...)
	}
	return history
}

func (s *MessagingService) SendMessage(ctx context.Context, input *models.SendMessageInput) (*models.SendMessageOutput, error) {
	msgID, err := s.Sender.SendTextMessage(ctx, input.Body.Phone, input.Body.Message)
	fmt.Printf("[Outgoing (API)] To %s: %s\n", input.Body.Phone, input.Body.Message)

	if err != nil {
		return nil, err
	}

	s.SaveMessage(input.Body.Phone, input.Body.Message, "sent")

	output := &models.SendMessageOutput{}
	output.Body.Success = true
	output.Body.MessageID = msgID
	return output, nil
}

func (s *MessagingService) SendMediaMessage(ctx context.Context, phone string, data []byte, fileName string, mediaType string, caption string) (*models.SendMediaMessageOutput, error) {
	msgID, err := s.Sender.SendMediaMessage(ctx, phone, data, fileName, mediaType, caption)
	fmt.Printf("[Outgoing (API) Media] To %s: %s - %s\n", phone, mediaType, fileName)

	// Actually I must provide the FULL logic or it will be lost.
	
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
	}

	if err != nil {
		return nil, err
	}

	s.SaveMessage(phone, fmt.Sprintf("[%s] %s", mediaType, caption), "sent")
	
	output := &models.SendMediaMessageOutput{}
	output.Body.Success = true
	output.Body.MessageID = msgID
	return output, nil
}

func (s *MessagingService) GetHistory() []models.MessageLog {
	rows, err := s.DB.Query("SELECT phone, message, type, timestamp FROM message_history ORDER BY id ASC")
	if err != nil {
		fmt.Printf("[Error] Failed to fetch all history from DB: %v\n", err)
		return nil
	}
	defer rows.Close()

	var history []models.MessageLog
	for rows.Next() {
		var m models.MessageLog
		if err := rows.Scan(&m.Phone, &m.Message, &m.Type, &m.Timestamp); err != nil {
			continue
		}
		history = append(history, m)
	}
	return history
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
		} else {
			s.SaveMessage(phone, replyText, "sent")
		}

		// Stop typing status
		s.Sender.SendChatPresence(ctx, phone, false)
	}()
}

func (s *MessagingService) SendAIAutoReply(phone string, userMessage string) {
	// Fetch history (last 6 messages) for context BEFORE saving current message
	// so that history only contains PAST messages.
	history := s.GetRecentMessages(phone, 6)

	// Save current message to DB for future context
	s.SaveMessage(phone, userMessage, "received")

	go func() {
		ctx := context.Background()
		fmt.Printf("[AI-reply] Showing typing and fetching response for %s\n", phone)
		s.Sender.SendChatPresence(ctx, phone, true)

		// Get response from Gemini with history
		aiResponse, err := s.GeminiService.GetAIResponse(ctx, userMessage, history)
		if err != nil {
			fmt.Printf("[Error] Gemini failure: %v\n", err)
			
			// Optional: Notify user that AI is busy if it's a quota error
			if strings.Contains(err.Error(), "429") {
				s.Sender.SendTextMessage(ctx, phone, "I'm a bit overwhelmed with messages right now, but I'll be back shortly!")
			}

			s.Sender.SendChatPresence(ctx, phone, false)
			return
		}

		fmt.Printf("[AI-reply] Sending AI response to %s\n", phone)
		_, err = s.Sender.SendTextMessage(ctx, phone, aiResponse)
		if err != nil {
			fmt.Printf("[Error] Failed to send AI auto-reply to %s: %v\n", phone, err)
		} else {
			s.SaveMessage(phone, aiResponse, "sent")
		}

		// Stop typing status
		s.Sender.SendChatPresence(ctx, phone, false)
	}()
}

func (s *MessagingService) OnMessageReceived(phone string, message string, isFromMe bool, isWeb bool, timestamp string) {
	msgType := "received"
	if isFromMe {
		msgType = "sent"
	}

	origin := "Mobile"
	if isWeb {
		origin = "Web"
	}

	if isFromMe {
		fmt.Printf("[Outgoing (%s)] To %s: %s\n", origin, phone, message)
		s.SaveMessage(phone, message, msgType)
	} else {
		fmt.Printf("[Incoming (%s)] From %s: %s\n", origin, phone, message)

		if s.GeminiService != nil {
			// SendAIAutoReply now handles saving the incoming message
			s.SendAIAutoReply(phone, message)
		} else {
			s.SaveMessage(phone, message, msgType)
			hiRegex := regexp.MustCompile(`^(hi+|hello)$`)
			lowerMsg := strings.ToLower(strings.TrimSpace(message))
			if hiRegex.MatchString(lowerMsg) {
				s.SendAutoReply(phone, "Hello! (AI is currently disabled)")
			}
		}
	}
}

func (s *MessagingService) OnLoggedOut() {
	if s.LogoutAction != nil {
		s.LogoutAction()
	}
}