# Whatsmeow Library Guide

`whatsmeow` is a Go library for the WhatsApp multi-device API. It allows you to build clients that interact with WhatsApp exactly like WhatsApp Web or the WhatsApp Desktop applications.

This document serves as a comprehensive guide on how to log in, receive messages, and send various types of content (text, video, documents) using `whatsmeow`.

---

## Installation

To install the library, run the following in your Go project:

```bash
go get go.mau.fi/whatsmeow
```

---

## 1. Login Methods

`whatsmeow` supports two primary ways to authenticate a new session: **QR Code Scanning** and **Pairing Code**.

### Method A: QR Code (Standard)
This is the traditional method where the library generates a QR code, which you scan using the WhatsApp app on your primary phone (Linked Devices -> Link a Device).

```go
package main

import (
	"context"
	"fmt"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	_ "modernc.org/sqlite"
)

func main() {
	// 1. Setup local database for storing session keys
	dbLog := waLog.Stdout("Database", "INFO", true)
	container, err := sqlstore.New("sqlite", "file:examplestore.db?_pragma=foreign_keys(ON)", dbLog)
	if err != nil {
		panic(err)
	}

	// 2. Get the device session
	deviceStore, err := container.GetFirstDevice()
	if err != nil {
		panic(err)
	}

	// 3. Initialize Client
	clientLog := waLog.Stdout("Client", "INFO", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	if client.Store.ID == nil {
		// Not logged in, get QR channel
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			panic(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				fmt.Println("Scan this QR code:", evt.Code)
				// In a real app, render this string as a visual QR code (e.g., using go-qrcode)
			} else if evt.Event == "success" {
				fmt.Println("Login successful!")
			}
		}
	} else {
		// Already logged in
		err = client.Connect()
		if err != nil {
			panic(err)
		}
	}
}
```

### Method B: Pairing Code (Alternative)
Instead of a QR code, you can request an 8-character pairing code. You enter this code into the WhatsApp app on your phone.

```go
// Connect first
err = client.Connect()
if err != nil {
    panic(err)
}

// Check if already logged in
if client.Store.ID == nil {
    // Generate pairing code for a specific phone number (with country code)
    code, err := client.PairPhone("1234567890", true, whatsmeow.PairClientChrome, "Chrome (Linux)")
    if err != nil {
        panic(err)
    }
    fmt.Println("Enter this pairing code in WhatsApp:", code)
}
```

---

## 2. Receiving Messages

To process incoming messages, you need to register an `EventHandler` before connecting the client.

```go
import (
	"fmt"
	"go.mau.fi/whatsmeow/types/events"
)

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		fmt.Printf("Received a message from %s\n", v.Info.Sender.User)
		
		// 1. Text Message
		if v.Message.GetConversation() != "" {
			fmt.Println("Text:", v.Message.GetConversation())
		}
		
		// 2. Image Message
		if img := v.Message.GetImageMessage(); img != nil {
			fmt.Println("Received an image!")
			// To download:
			// data, err := client.Download(img)
		}

        // 3. Document Message
		if doc := v.Message.GetDocumentMessage(); doc != nil {
			fmt.Printf("Received a document: %s\n", doc.GetFileName())
		}
        
        // 4. Video Message
        if vid := v.Message.GetVideoMessage(); vid != nil {
			fmt.Printf("Received a video!\n")
		}
	}
}

// Attach the handler before connecting
// client.AddEventHandler(eventHandler)
```

---

## 3. Sending Messages

Sending messages requires compiling Protocol Buffer messages (`waE2E.Message`).

### A. Sending Text Messages

```go
import (
	"context"
	"go.mau.fi/whatsmeow/types"
	waProto "go.mau.fi/whatsmeow/binary/proto" // Old path
	"go.mau.fi/whatsmeow/proto/waE2E"       // New path
	"google.golang.org/protobuf/proto"
)

func sendText(client *whatsmeow.Client, phone string, text string) {
	targetJID := types.NewJID(phone, types.DefaultUserServer)
	
	msg := &waE2E.Message{
		Conversation: proto.String(text),
	}
	
	_, err := client.SendMessage(context.Background(), targetJID, msg)
	if err != nil {
		fmt.Println("Error sending text:", err)
	}
}
```

### B. Sending Media (Images and Video)

Before sending media, you **must upload it** to WhatsApp's servers using `client.Upload()`. This gives you the URL and keys needed to construct the message.

**Image Example:**
```go
import (
	"os"
	"go.mau.fi/whatsmeow"
)

func sendImage(client *whatsmeow.Client, phone string, imagePath string) {
	data, _ := os.ReadFile(imagePath)
	
	// 1. Upload to WhatsApp servers
	uploaded, err := client.Upload(context.Background(), data, whatsmeow.MediaImage)
	if err != nil {
		panic(err)
	}

	// 2. Construct Message
	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			Caption:       proto.String("Hello from Go!"),
			Mimetype:      proto.String("image/jpeg"),
			Url:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			FileEncSha256: uploaded.FileEncSHA256,
			FileSha256:    uploaded.FileSHA256,
			FileLength:    &uploaded.FileLength,
		},
	}
	
	targetJID := types.NewJID(phone, types.DefaultUserServer)
	client.SendMessage(context.Background(), targetJID, msg)
}
```

**Video Example:**
Sending a video works exactly the same as an image, but you use `whatsmeow.MediaVideo` and the `VideoMessage` struct.

```go
uploaded, _ := client.Upload(context.Background(), videoData, whatsmeow.MediaVideo)

msg := &waE2E.Message{
    VideoMessage: &waE2E.VideoMessage{
        Caption:       proto.String("Look at this video"),
        Mimetype:      proto.String("video/mp4"),
        Url:           &uploaded.URL,
        DirectPath:    &uploaded.DirectPath,
        MediaKey:      uploaded.MediaKey,
        FileEncSha256: uploaded.FileEncSHA256,
        FileSha256:    uploaded.FileSHA256,
        FileLength:    &uploaded.FileLength,
    },
}
```

### C. Sending Documents (PDFs, ZIPs)

Documents follow the exact same upload pattern, using `whatsmeow.MediaDocument`.

```go
func sendDocument(client *whatsmeow.Client, phone string, docPath string) {
	data, _ := os.ReadFile(docPath)
	
	uploaded, err := client.Upload(context.Background(), data, whatsmeow.MediaDocument)
	if err != nil {
		panic(err)
	}

	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			Title:         proto.String("Report.pdf"), // Title shown in UI
			FileName:      proto.String("Report.pdf"), // Actual filename
			Mimetype:      proto.String("application/pdf"),
			Url:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			FileEncSha256: uploaded.FileEncSHA256,
			FileSha256:    uploaded.FileSHA256,
			FileLength:    &uploaded.FileLength,
		},
	}
	
	targetJID := types.NewJID(phone, types.DefaultUserServer)
	client.SendMessage(context.Background(), targetJID, msg)
}
```

---

## Summary of Media Types
When calling `client.Upload()`, use the following constants depending on the file type:
- `whatsmeow.MediaImage`
- `whatsmeow.MediaVideo`
- `whatsmeow.MediaAudio`
- `whatsmeow.MediaDocument`

By using these events and protobuf message constructors, `whatsmeow` allows you to build complete programmatic integrations with WhatsApp.