package auth

import (
	"context"
	"errors"
	"strings"

	firebase "firebase.google.com/go/v4"
	fbauth "firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"
)

type FirebaseVerifier struct {
	client *fbauth.Client
}

func NewFirebaseVerifier(ctx context.Context, projectID string) (*FirebaseVerifier, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, errors.New("FIREBASE_PROJECT_ID belum diatur")
	}
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID}, option.WithoutAuthentication())
	if err != nil {
		return nil, err
	}
	client, err := app.Auth(ctx)
	if err != nil {
		return nil, err
	}
	return &FirebaseVerifier{client: client}, nil
}

func (v *FirebaseVerifier) Verify(ctx context.Context, idToken string) (*Identity, error) {
	token, err := v.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		return nil, err
	}
	email, _ := token.Claims["email"].(string)
	return &Identity{UID: token.UID, Email: email}, nil
}
