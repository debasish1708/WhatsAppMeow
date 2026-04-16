package services

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"whatsmeow/models"
	"whatsmeow/whatsapp"

	"go.mau.fi/whatsmeow/types"
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

func (s *MessagingService) DeleteAllHistory() error {
	_, err := s.DB.Exec("DELETE FROM message_history")
	if err != nil {
		return fmt.Errorf("failed to delete history: %v", err)
	}
	fmt.Println("[Command] All message history deleted.")
	return nil
}

func (s *MessagingService) DeleteAllMedia() error {
	mediaDirs := []string{"media/images", "media/videos", "media/documents"}
	for _, dir := range mediaDirs {
		files, err := filepath.Glob(filepath.Join(dir, "*"))
		if err != nil {
			fmt.Printf("[Error] Failed to glob media directory %s: %v\n", dir, err)
			continue
		}
		for _, f := range files {
			if err := os.Remove(f); err != nil {
				fmt.Printf("[Error] Failed to delete media file %s: %v\n", f, err)
			}
		}
	}
	fmt.Println("[Command] All media files deleted.")
	return nil
}

// HandleCommand processes commands sent to the authorized number
func (s *MessagingService) HandleCommand(phone string, message string) bool {
	cmdParts := strings.Fields(strings.ToLower(strings.TrimSpace(message)))
	if len(cmdParts) == 0 {
		return false
	}

	cmd := cmdParts[0]

	switch cmd {
	case "delete_history":
		if err := s.DeleteAllHistory(); err == nil {
			s.SendAutoReply(phone, "✅ All conversation history has been deleted.")
		} else {
			s.SendAutoReply(phone, "❌ Failed to delete conversation history.")
		}
		return true
	case "delete_media":
		if err := s.DeleteAllMedia(); err == nil {
			s.SendAutoReply(phone, "✅ All media files have been deleted.")
		} else {
			s.SendAutoReply(phone, "❌ Failed to delete media files.")
		}
		return true
	case "logout_me":
		s.SendAutoReply(phone, "👋 Logging out... The device will be unlinked in 2 seconds.")
		go func() {
			time.Sleep(2 * time.Second)
			if s.LogoutAction != nil {
				s.LogoutAction()
			}
		}()
		return true
	case "activate":
		if len(cmdParts) < 2 {
			s.SendAutoReply(phone, "❌ Usage: activate {mobile_number}")
			return true
		}
		targetPhone := cmdParts[1]
		if err := s.ActivateUser(targetPhone); err != nil {
			s.SendAutoReply(phone, fmt.Sprintf("❌ Failed to activate %s: %v", targetPhone, err))
		} else {
			s.SendAutoReply(phone, fmt.Sprintf("✅ User %s has been activated.", targetPhone))
		}
		return true
	case "deactivate":
		if len(cmdParts) < 2 {
			s.SendAutoReply(phone, "❌ Usage: deactivate {mobile_number}")
			return true
		}
		targetPhone := cmdParts[1]
		if err := s.DeactivateUser(targetPhone); err != nil {
			s.SendAutoReply(phone, fmt.Sprintf("❌ Failed to deactivate %s: %v", targetPhone, err))
		} else {
			s.SendAutoReply(phone, fmt.Sprintf("✅ User %s has been deactivated.", targetPhone))
		}
		return true
	case "list_activate":
		users, err := s.GetActiveUsers()
		if err != nil {
			s.SendAutoReply(phone, fmt.Sprintf("❌ Failed to fetch active users: %v", err))
		} else if len(users) == 0 {
			s.SendAutoReply(phone, "ℹ️ No active users found.")
		} else {
			reply := "📋 *Active Users:*\n"
			for i, u := range users {
				reply += fmt.Sprintf("%d. %s\n", i+1, u)
			}
			s.SendAutoReply(phone, reply)
		}
		return true
	}
	return false
}

func (s *MessagingService) IsUserActive(phone string) bool {
	var exists bool
	err := s.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM active_users WHERE phone = ?)", phone).Scan(&exists)
	if err != nil {
		fmt.Printf("[Error] Failed to check active status for %s: %v\n", phone, err)
		return false
	}
	return exists
}

func (s *MessagingService) ActivateUser(phone string) error {
	_, err := s.DB.Exec("INSERT OR REPLACE INTO active_users (phone) VALUES (?)", phone)
	return err
}

func (s *MessagingService) DeactivateUser(phone string) error {
	_, err := s.DB.Exec("DELETE FROM active_users WHERE phone = ?", phone)
	return err
}

func (s *MessagingService) GetActiveUsers() ([]string, error) {
	rows, err := s.DB.Query("SELECT phone FROM active_users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err == nil {
			users = append(users, u)
		}
	}
	return users, nil
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

		// Create a ticker to keep typing status alive (WhatsApp status usually expires after ~10-20s)
		ticker := time.NewTicker(5 * time.Second)
		done := make(chan bool)
		go func() {
			for {
				select {
					case <-ticker.C:
						s.Sender.SendChatPresence(ctx, phone, true)
					case <-done:
						ticker.Stop()
						return
				}
			}
		}()

		// Get response from Gemini with history
		aiResponse, err := s.GeminiService.GetAIResponse(ctx, userMessage, history)
		done <- true // Stop the ticker

		if err != nil {
			fmt.Printf("[Error] Gemini failure: %v\n", err)
			
			// Optional: Notify user that AI is busy if it's a quota error
			if strings.Contains(err.Error(), "429") {
				s.Sender.SendTextMessage(ctx, phone, "I'm a bit overwhelmed with messages right now, but I'll be back shortly!")
			}

			s.Sender.SendChatPresence(ctx, phone, false)
			return
		}

		// Small delay to make it feel more natural if it was too fast
		time.Sleep(1 * time.Second)

		fmt.Printf("[AI-reply] Sending AI response to %s: %s\n", phone, aiResponse)
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

func (s *MessagingService) OnMessageReceived(phone string, message string, isFromMe bool, isWeb bool, timestamp string, msgID string, chatJID types.JID, senderJID types.JID) {
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

		// Command Handling for Outgoing messages to authorized number
		if phone == "917815030574" {
			s.HandleCommand(phone, message)
		}
	} else {
		fmt.Printf("[Incoming (%s)] From %s: %s\n", origin, phone, message)

		// Dynamic active user check
		if !s.IsUserActive(phone) {
			return
		}

		// Mark as read immediately (Blue Tick)
		err := s.Sender.MarkRead(context.Background(), msgID, chatJID, senderJID)
		if err != nil {
			fmt.Printf("[Error] Failed to mark message as read: %v\n", err)
		}

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
