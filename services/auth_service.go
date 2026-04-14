package services

import (
	"context"

	"whatsmeow/models"
	"whatsmeow/whatsapp"
)

type AuthService struct {
	Authenticator whatsapp.Authenticator
}

func NewAuthService(auth whatsapp.Authenticator) *AuthService {
	return &AuthService{
		Authenticator: auth,
	}
}

func (s *AuthService) GetLogin() (*models.LoginOutput, error) {
	resp := &models.LoginOutput{}
	
	status, msg := s.Authenticator.GetLoginStatus()
	resp.Body.Status = status
	resp.Body.Message = msg

	if status == "qr_ready" {
		qrCode, qrImage := s.Authenticator.GetQRCode()
		resp.Body.QRCode = qrCode
		resp.Body.QRCodeImage = qrImage
	}

	return resp, nil
}

func (s *AuthService) Status() (*models.StatusOutput, error) {
	resp := &models.StatusOutput{}
	resp.Body.Connected = s.Authenticator.IsConnected()
	resp.Body.LoggedIn = s.Authenticator.IsLoggedIn()
	return resp, nil
}

func (s *AuthService) Logout(ctx context.Context) (*models.LogoutOutput, error) {
	err := s.Authenticator.Logout(ctx)
	if err != nil {
		return nil, err
	}
	resp := &models.LogoutOutput{}
	resp.Body.Success = true
	return resp, nil
}