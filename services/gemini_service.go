package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
	"whatsmeow/models"
)

type GeminiService struct {
	Client *genai.Client
	Model  *genai.GenerativeModel
}

func NewGeminiService(ctx context.Context, apiKey string) (*GeminiService, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %v", err)
	}

	// Using gemini-2.0-flash as confirmed by ListModels
	model := client.GenerativeModel("gemini-2.5-flash")
	
	// System instruction to maintain a human persona and not identify as an AI
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text("Your name is Debasish Das. You are a friendly, helpful, and kind-natured person here to help your friends with any problem and provide the best suggestions. You are chatting on WhatsApp. CRITICAL: Never mention that you are an AI, a bot, or a large language model. Always speak as a real human. If you do not know the answer to a question, respond with: 'Sorry, I am not aware of this. Please ask me another question.' Keep your responses natural and concise.")},
	}

	return &GeminiService{
		Client: client,
		Model:  model,
	}, nil
}

func (s *GeminiService) GetAIResponse(ctx context.Context, userPrompt string, history []models.MessageLog) (string, error) {
	var lastErr error
	maxRetries := 3

	for i := 0; i < maxRetries; i++ {
		chat := s.Model.StartChat()
		
		// Convert history to genai.Content
		for _, msg := range history {
			role := "user"
			if msg.Type == "sent" {
				role = "model"
			}
			chat.History = append(chat.History, &genai.Content{
				Role: role,
				Parts: []genai.Part{genai.Text(msg.Message)},
			})
		}

		resp, err := chat.SendMessage(ctx, genai.Text(userPrompt))
		if err == nil {
			if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
				if text, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
					return string(text), nil
				}
			}
			return "I'm not sure how to respond to that.", nil
		}

		lastErr = err
		// Check for 429 (Rate Limit) error
		if strings.Contains(err.Error(), "429") {
			// Exponential backoff: 2s, 4s, 8s...
			backoff := time.Duration(2<<(uint(i))) * time.Second
			fmt.Printf("[Gemini] Quota exceeded, retrying in %v... (Attempt %d/%d)\n", backoff, i+1, maxRetries)
			time.Sleep(backoff)
			continue
		}
		
		// If it's not a rate limit error (e.g., 404 or something else), don't retry
		break
	}

	return "", fmt.Errorf("Gemini error: %v", lastErr)
}

func (s *GeminiService) Close() {
	if s.Client != nil {
		s.Client.Close()
	}
}
