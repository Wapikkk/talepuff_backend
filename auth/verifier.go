package auth

import "context"

type Verifier interface {
	Verify(ctx context.Context, idToken string) (*Identity, error)
}
