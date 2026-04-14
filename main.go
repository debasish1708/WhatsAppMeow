package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/joho/godotenv"
	_ "modernc.org/sqlite"

	"whatsmeow/handlers"
	"whatsmeow/services"
	"whatsmeow/whatsapp"
)

func main() {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Warning: .env file not found. Falling back to system environment variables.")
	}

	// Create data and media directories if they don't exist
	_ = os.MkdirAll("data", 0755)
	_ = os.MkdirAll("media/images", 0755)
	_ = os.MkdirAll("media/videos", 0755)
	_ = os.MkdirAll("media/documents", 0755)

	dsn := "file:data/whatsmeow.db?_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	
	// Open DB for message history
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		panic(fmt.Errorf("failed to open database: %v", err))
	}
	defer db.Close()

	// Create message_history table
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS message_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		phone TEXT,
		message TEXT,
		type TEXT, -- 'sent' or 'received'
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		panic(fmt.Errorf("failed to create message_history table: %v", err))
	}

	adapter, err := whatsapp.NewWhatsAppAdapter(dsn)
	if err != nil {
		panic(err)
	}

	msgSender := whatsapp.NewDefaultMessageSender(adapter)
	
	// Gemini AI Integration
	geminiKey := os.Getenv("GEMINI_API_KEY")
	var geminiService *services.GeminiService
	if geminiKey != "" {
		fmt.Println("[AI] Gemini API Key found, initializing Gemini service.")
		var err error
		geminiService, err = services.NewGeminiService(context.Background(), geminiKey)
		if err != nil {
			fmt.Printf("[AI] Error initializing Gemini: %v\n", err)
		}
	} else {
		fmt.Println("[AI] WARNING: GEMINI_API_KEY not found. AI features will be disabled.")
	}

	msgService := services.NewMessagingService(msgSender, geminiService, db)
	
	// Create the event dispatcher
	whatsapp.NewEventDispatcher(adapter, msgService)

	authenticator := whatsapp.NewQRCodeAuthenticator(adapter, msgService)
	authService := services.NewAuthService(authenticator)

	// Wiring the logout action back to MessagingService for OnLoggedOut event
	msgService.LogoutAction = func() {
		_, _ = authService.Logout(context.Background())
	}

	// Connect if previously logged in
	err = adapter.Connect()
	if err != nil {
		fmt.Println("Failed to connect:", err)
	}

	// Setup API Handlers
	apiHandlers := handlers.NewAPIHandlers(authService, msgService)

	// Setup Huma
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("WhatsApp API", "1.0.0"))

	huma.Register(api, huma.Operation{
		OperationID: "login",
		Method:      http.MethodPost,
		Path:        "/login",
		Summary:     "Get QR Code / Login",
		Description: "Returns a QR code to be scanned with the WhatsApp app if not logged in.",
	}, apiHandlers.LoginHandler)

	huma.Register(api, huma.Operation{
		OperationID: "send-message",
		Method:      http.MethodPost,
		Path:        "/send",
		Summary:     "Send a message",
		Description: "Sends a text message to a specified phone number.",
	}, apiHandlers.SendMessageHandler)

	huma.Register(api, huma.Operation{
		OperationID: "send-media-message",
		Method:      http.MethodPost,
		Path:        "/send-media-message",
		Summary:     "Send a media message",
		Description: "Sends a media message (image, document, video) using multipart/form-data. Requires fields: phone, media_type, file. Optional: caption.",
	}, apiHandlers.SendMediaMessageHandler)

	huma.Register(api, huma.Operation{
		OperationID: "status",
		Method:      http.MethodGet,
		Path:        "/status",
		Summary:     "Get connection status",
		Description: "Returns whether the client is currently connected and logged in.",
	}, apiHandlers.StatusHandler)
	
	huma.Register(api, huma.Operation{
		OperationID: "logout",
		Method:      http.MethodPost,
		Path:        "/logout",
		Summary:     "Logout",
		Description: "Logs out the current WhatsApp session.",
	}, apiHandlers.LogoutHandler)

	huma.Register(api, huma.Operation{
		OperationID: "history",
		Method:      http.MethodGet,
		Path:        "/history",
		Summary:     "Get Message History",
		Description: "Returns all captured incoming and outgoing messages.",
	}, apiHandlers.HistoryHandler)

	go func() {
		fmt.Println("Server running on http://localhost:8080")
		fmt.Println("Docs available at http://localhost:8080/docs")
		
		corsMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			mux.ServeHTTP(w, r)
		})

		if err := http.ListenAndServe(":8080", corsMux); err != nil {
			panic(err)
		}
	}()

	// Wait for exit
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	adapter.Disconnect()
}