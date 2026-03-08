package main

import (
	"context"
	"crypto/tls"
	"log"
	"os"

	"github.com/U4ko3/mixi2-api-salmonrun-schedule-bot/common"
	"github.com/U4ko3/mixi2-api-salmonrun-schedule-bot/config"
	"github.com/mixigroup/mixi2-application-sdk-go/auth"
	application_apiv1 "github.com/mixigroup/mixi2-application-sdk-go/gen/go/social/mixi/application/service/application_api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	cfg := config.GetConfig()

	// Create authenticator
	authenticator, err := auth.NewAuthenticator(cfg.ClientID, cfg.ClientSecret, cfg.TokenURL)
	if err != nil {
		log.Fatalf("failed to create authenticator: %v", err)
	}

	// Create gRPC connection for API
	apiConn, err := grpc.NewClient(
		cfg.APIAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})),
	)
	if err != nil {
		log.Fatalf("failed to connect to api: %v", err)
	}
	defer apiConn.Close()

	// Create API client
	apiClient := application_apiv1.NewApplicationServiceClient(apiConn)

	// Execute periodic task
	if err := periodicTask(context.Background(), apiClient, authenticator); err != nil {
		log.Fatalf("periodic task error: %v", err)
	}

	log.Println("periodic task executed successfully")
}

func periodicTask(ctx context.Context, apiClient application_apiv1.ApplicationServiceClient, authenticator auth.Authenticator) error {
	// ここに定期実行したい処理を記述
	authCtx, err := authenticator.AuthorizedContext(ctx)
	if err != nil {
		return err
	}

	postText := common.GetSalmonSchedule()
	if postText == "" {
		log.Println("no schedule information available")
		return nil
	} else {
		_, err = apiClient.CreatePost(authCtx, &application_apiv1.CreatePostRequest{
			Text: postText,
		})
		if err != nil {
			return err
		}
	}

	return nil
}