package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"github.com/Seann-Moser/credentials/oauth/oserver"
	"github.com/Seann-Moser/credentials/session"
	"github.com/Seann-Moser/credentials/user"
	"github.com/Seann-Moser/mserve"
	"github.com/Seann-Moser/rbac"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"time"
)

func main() {
	ctx := context.Background()
	mongoDB, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017").SetAuth(options.Credential{
		Username: "mongoadmin",
		Password: "secretpassword",
	}))
	if err != nil {
		log.Fatal(err)
	}
	rbacManager, err := rbac.NewMongoStoreManager(ctx, mongoDB.Database("local"))
	if err != nil {
		log.Fatal(err)
	}
	secret, err := GenerateSecret(32)
	if err != nil {
		log.Fatal(err)
	}
	oServer := oserver.NewMongoServer(mongoDB.Database("local"))
	oServer.RegisterClient(ctx, &oserver.OAuthClient{
		ClientID:           uuid.New().String(),
		ClientSecret:       uuid.New().String(),
		Name:               "Test",
		ImageURL:           "",
		RedirectURIs:       []string{"localhost:8080"},
		Scopes:             nil,
		TokenEndpointAuth:  "",
		GrantTypes:         nil,
		ResponseTypes:      nil,
		ConnectedUserCount: 0,
	})
	ses := session.NewClient(oServer, rbacManager, secret, 24*time.Hour)
	s := mserve.NewServer("Example", rbacManager, []string{}, ses, mserve.SSLConfig{Port: 8080})

	userServer, err := user.NewServer(user.NewMongoDBStore(mongoDB, "local", "user"), rbacManager, secret, "", "", "")
	if err != nil {
		log.Fatal(err)
	}
	err = s.SetupOServer(ctx, oServer).
		SetupRbac(ctx).
		SetupUserLogin(ctx, userServer).
		HealthCheck("/healthz", nil).
		GenerateOpenAPIDocs().
		Run(ctx)
	if err != nil {
		log.Fatal(err)
	}

}

func GenerateSecret(length int) ([]byte, error) {
	if length <= 0 {
		return nil, fmt.Errorf("secret length must be positive")
	}

	secret := make([]byte, length)
	// Read generates cryptographically secure random bytes and writes them into secret.
	// It returns the number of bytes read and an error, if any.
	n, err := rand.Read(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to read random bytes for secret: %w", err)
	}
	if n != length {
		return nil, fmt.Errorf("expected to read %d bytes, but only read %d", length, n)
	}

	return secret, nil
}
