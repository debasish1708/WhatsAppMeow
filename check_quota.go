package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/google/generative-ai-go/genai"
	"github.com/joho/godotenv"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

func checkGemini() {
	godotenv.Load()
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY not found")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	fmt.Println("Fetching available models...")
	iter := client.ListModels(ctx)
	var availableModels []string
	for {
		m, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		// We only care about models that support GenerateContent
		supportsGenerate := false
		for _, method := range m.SupportedGenerationMethods {
			if method == "generateContent" {
				supportsGenerate = true
				break
			}
		}
		if supportsGenerate {
			// Strip "models/" prefix if it exists for the GenerativeModel call
			name := m.Name
			if len(name) > 7 && name[:7] == "models/" {
				name = name[7:]
			}
			availableModels = append(availableModels, name)
		}
	}

	fmt.Printf("Found %d models supporting GenerateContent. Testing quota...\n", len(availableModels))
	for _, modelName := range availableModels {
		model := client.GenerativeModel(modelName)
		// Use a very short timeout context for testing
		resp, err := model.GenerateContent(ctx, genai.Text("ping"))
		if err != nil {
			fmt.Printf("- %s: FAILED (%v)\n", modelName, err)
		} else if len(resp.Candidates) > 0 {
			fmt.Printf("- %s: SUCCESS\n", modelName)
		} else {
			fmt.Printf("- %s: EMPTY RESPONSE\n", modelName)
		}
	}
}