package services

import (
	"context"
	"fmt"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
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

	// Using gemini-flash-latest for best free-tier availability
	model := client.GenerativeModel("gemini-flash-latest")
	
	// Optional: Set a system instruction to keep it friendly like before
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text("You are a helpful WhatsApp assistant. Keep your responses concise and friendly.")},
	}

	return &GeminiService{
		Client: client,
		Model:  model,
	}, nil
}

func (s *GeminiService) GetAIResponse(ctx context.Context, userPrompt string) (string, error) {
	resp, err := s.Model.GenerateContent(ctx, genai.Text(userPrompt))
	if err != nil {
		return "", fmt.Errorf("Gemini error: %v", err)
	}

	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		if text, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
			return string(text), nil
		}
	}

	return "I'm not sure how to respond to that.", nil
}

func (s *GeminiService) Close() {
	if s.Client != nil {
		s.Client.Close()
	}
}
