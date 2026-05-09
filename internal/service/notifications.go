package service

import (
	"context"
	"log/slog"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"

	"diplom/internal/repository"
)

type NotificationService struct {
	client   *messaging.Client
	userRepo repository.UserRepository
	logger   *slog.Logger
}

func NewNotificationService(credentialsFile string, userRepo repository.UserRepository) (*NotificationService, error) {
	if credentialsFile == "" {
		return nil, nil
	}

	ctx := context.Background()
	app, err := firebase.NewApp(ctx, nil, option.WithCredentialsFile(credentialsFile))
	if err != nil {
		return nil, err
	}

	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, err
	}

	return &NotificationService{
		client:   client,
		userRepo: userRepo,
		logger:   slog.Default(),
	}, nil
}

func (s *NotificationService) Notify(userIDs []int64, title, body string, data map[string]string) {
	if s == nil || len(userIDs) == 0 {
		return
	}

	tokens, err := s.userRepo.GetFCMTokensByUsers(userIDs)
	if err != nil || len(tokens) == 0 {
		return
	}

	msg := &messaging.MulticastMessage{
		Tokens: tokens,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data: data,
	}

	response, err := s.client.SendEachForMulticast(context.Background(), msg)
	if err != nil {
		s.logger.Error("fcm multicast failed", "error", err)
		return
	}

	for i, result := range response.Responses {
		if result.Error != nil && messaging.IsRegistrationTokenNotRegistered(result.Error) {
			_ = s.userRepo.DeleteFCMTokenByToken(tokens[i])
		}
	}
}
