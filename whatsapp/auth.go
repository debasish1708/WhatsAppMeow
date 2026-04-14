package whatsapp

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/skip2/go-qrcode"
)

type QRCodeAuthenticator struct {
	Adapter      *WhatsAppAdapter
	qrCodeStr    string
	connecting   bool
	loginTimeout bool
	listener     MessageListener
}

func NewQRCodeAuthenticator(adapter *WhatsAppAdapter, listener MessageListener) *QRCodeAuthenticator {
	return &QRCodeAuthenticator{
		Adapter:  adapter,
		listener: listener,
	}
}

func (a *QRCodeAuthenticator) Connect() error {
	return a.Adapter.Client.Connect()
}

func (a *QRCodeAuthenticator) IsConnected() bool {
	return a.Adapter.Client.IsConnected()
}

func (a *QRCodeAuthenticator) IsLoggedIn() bool {
	return a.Adapter.Client.IsLoggedIn() && a.Adapter.Client.IsConnected()
}

func (a *QRCodeAuthenticator) GetLoginStatus() (string, string) {
	if a.IsLoggedIn() {
		return "logged_in", "Device is already logged in"
	}

	if a.Adapter.Client.Store.ID == nil {
		if a.loginTimeout {
			a.loginTimeout = false
			return "timeout", "QR code generation timed out. Please request again."
		}

		if a.qrCodeStr == "" && !a.connecting {
			a.connecting = true
			qrChan, _ := a.Adapter.Client.GetQRChannel(context.Background())
			err := a.Adapter.Client.Connect()
			if err != nil {
				a.connecting = false
				return "error", "Failed to connect: " + err.Error()
			}

			go func() {
				for evt := range qrChan {
					switch evt.Event {
						case "code":
							a.qrCodeStr = evt.Code
							fmt.Println(a.qrCodeStr)
							fmt.Println("New QR Code generated. It will expire in: ", evt.Timeout)
						case "success":
							a.qrCodeStr = ""
							a.connecting = false
							a.loginTimeout = false
							fmt.Println("Login successful!")
						case "timeout":
							a.qrCodeStr = ""
							a.connecting = false
							a.loginTimeout = true
							fmt.Println("Login timeout. Please request a new QR code.")
							a.Adapter.Client.Disconnect()
						}
				}
			}()
			time.Sleep(2 * time.Second) // Wait for first QR
		}

		if a.qrCodeStr != "" {
			return "qr_ready", "Please scan the QR code to log in."
		} else {
			return "generating", "QR code is being generated. Please request again."
		}
	} else {
		if !a.IsConnected() {
			err := a.Adapter.Client.Connect()
			if err != nil {
				return "error", "Failed to connect: " + err.Error()
			}
		}
		return "connecting", "Connecting using existing session"
	}
}

func (a *QRCodeAuthenticator) GetQRCode() (string, string) {
	if a.qrCodeStr == "" {
		return "", ""
	}
	png, err := qrcode.Encode(a.qrCodeStr, qrcode.Medium, 256)
	if err != nil {
		return a.qrCodeStr, ""
	}
	return a.qrCodeStr, "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
}

func (a *QRCodeAuthenticator) Logout(ctx context.Context) error {
	err := a.Adapter.Client.Logout(ctx)
	if err != nil {
		return err
	}
	a.qrCodeStr = ""
	a.connecting = false
	a.Adapter.InitClient() // Re-initialize after logout

	if a.listener != nil {
		a.listener.OnLoggedOut()
	}

	return nil
}