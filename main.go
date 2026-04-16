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
		fmt.Println("Warning: .env file not found. Using system environment variables.")
	}

	// ================================
	// 📁 Setup Data Directory
	// ================================
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "data" // local fallback
	}

	// Create directories
	_ = os.MkdirAll(dataDir, 0755)
	_ = os.MkdirAll("media/images", 0755)
	_ = os.MkdirAll("media/videos", 0755)
	_ = os.MkdirAll("media/documents", 0755)

	// ================================
	// 🗄️ SQLite DSN
	// ================================
	dsn := fmt.Sprintf(
		"file:%s/whatsmeow.db?_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)",
		dataDir,
	)

	// Open DB
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		panic(fmt.Errorf("failed to open database: %v", err))
	}
	defer db.Close()

	// Create tables
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS message_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		phone TEXT,
		message TEXT,
		type TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		panic(fmt.Errorf("failed to create message_history table: %v", err))
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS active_users (
		phone TEXT PRIMARY KEY,
		added_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		panic(fmt.Errorf("failed to create active_users table: %v", err))
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS system_prompts (
		phone TEXT PRIMARY KEY,
		prompt TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		panic(fmt.Errorf("failed to create system_prompts table: %v", err))
	}

	// Pre-populate with initial authorized numbers if empty
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM active_users").Scan(&count)
	if err == nil && count == 0 {
		initialUsers := []string{"917815030574"}
		for _, phone := range initialUsers {
			_, _ = db.Exec("INSERT INTO active_users (phone) VALUES (?)", phone)
		}
		fmt.Println("[DB] Pre-populated active_users with initial numbers.")
	}

	// ================================
	// 🤖 WhatsApp + Services
	// ================================
	adapter, err := whatsapp.NewWhatsAppAdapter(dsn)
	if err != nil {
		panic(err)
	}

	msgSender := whatsapp.NewDefaultMessageSender(adapter)

	// Gemini AI
	geminiKey := os.Getenv("GEMINI_API_KEY")
	var geminiService *services.GeminiService

	if geminiKey != "" {
		fmt.Println("[AI] Gemini API Key found.")
		geminiService, err = services.NewGeminiService(context.Background(), geminiKey)
		if err != nil {
			fmt.Println("[AI] Error:", err)
		}
	} else {
		fmt.Println("[AI] Gemini disabled (no API key).")
	}

	msgService := services.NewMessagingService(msgSender, geminiService, db)

	// Event system
	whatsapp.NewEventDispatcher(adapter, msgService)

	authenticator := whatsapp.NewQRCodeAuthenticator(adapter, msgService)
	authService := services.NewAuthService(authenticator)

	msgService.LogoutAction = func() {
		_, _ = authService.Logout(context.Background())
	}

	// Connect WhatsApp
	err = adapter.Connect()
	if err != nil {
		fmt.Println("WhatsApp connect error:", err)
	}

	// ================================
	// 🌐 API Setup
	// ================================
	apiHandlers := handlers.NewAPIHandlers(authService, msgService)

	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("WhatsApp API", "1.0.0"))

	huma.Register(api, huma.Operation{
		OperationID: "login",
		Method:      http.MethodPost,
		Path:        "/login",
	}, apiHandlers.LoginHandler)

	huma.Register(api, huma.Operation{
		OperationID: "send-message",
		Method:      http.MethodPost,
		Path:        "/send",
	}, apiHandlers.SendMessageHandler)

	huma.Register(api, huma.Operation{
		OperationID: "send-media-message",
		Method:      http.MethodPost,
		Path:        "/send-media-message",
	}, apiHandlers.SendMediaMessageHandler)

	huma.Register(api, huma.Operation{
		OperationID: "status",
		Method:      http.MethodGet,
		Path:        "/status",
	}, apiHandlers.StatusHandler)

	huma.Register(api, huma.Operation{
		OperationID: "logout",
		Method:      http.MethodPost,
		Path:        "/logout",
	}, apiHandlers.LogoutHandler)

	huma.Register(api, huma.Operation{
		OperationID: "history",
		Method:      http.MethodGet,
		Path:        "/history",
	}, apiHandlers.HistoryHandler)

	// ================================
	// 🚀 Server Start
	// ================================
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	corsMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		mux.ServeHTTP(w, r)
	})

	go func() {
		fmt.Println("🚀 Server running on port:", port)
		fmt.Println("📄 API Docs available at: /docs")

		if err := http.ListenAndServe(":"+port, corsMux); err != nil {
			panic(err)
		}
	}()

	// ================================
	// 🛑 Graceful Shutdown
	// ================================
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	fmt.Println("Shutting down...")
	adapter.Disconnect()
}
