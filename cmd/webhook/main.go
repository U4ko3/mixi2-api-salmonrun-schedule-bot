package main

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/U4ko3/mixi2-api-salmonrun-schedule-bot/config"
	"github.com/U4ko3/mixi2-api-salmonrun-schedule-bot/handler"
	"github.com/mixigroup/mixi2-application-sdk-go/auth"
	"github.com/mixigroup/mixi2-application-sdk-go/event/webhook"
	application_apiv1 "github.com/mixigroup/mixi2-application-sdk-go/gen/go/social/mixi/application/service/application_api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	cfg := config.GetConfig()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Decode public key
	if cfg.SignaturePublicKey == "" {
		log.Fatal("SIGNATURE_PUBLIC_KEY is required")
	}
	publicKeyBytes, err := base64.StdEncoding.DecodeString(cfg.SignaturePublicKey)
	if err != nil {
		log.Fatalf("failed to decode public key: %v", err)
	}
	publicKey := ed25519.PublicKey(publicKeyBytes)

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

	// Create event handler
	eventHandler := handler.NewHandler(apiClient, authenticator)

	// Create server
	addr := ":" + cfg.Port
	server := webhook.NewServer(addr, publicKey, eventHandler, webhook.WithLogger(logger))

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutting down...")
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown error", slog.Any("error", err))
		}
	}()

	// WaitGroupで複数のゴルーチンを管理
	var wg sync.WaitGroup

	// Webhookサーバーをゴルーチンで実行
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("starting webhook server", slog.String("port", cfg.Port))
		if err := server.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	// 定期処理を実行するゴルーチン
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(30 * time.Second) // 30秒ごとに実行
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				logger.Info("executing periodic task")
				if err := periodicTask(ctx, apiClient, authenticator, logger); err != nil {
					logger.Error("periodic task error", slog.String("error", err.Error()))
				}
			}
		}
	}()

	// 全てのゴルーチンの完了を待つ
	wg.Wait()
	logger.Info("stopped")
}

func periodicTask(ctx context.Context, apiClient application_apiv1.ApplicationServiceClient, authenticator auth.Authenticator, logger *slog.Logger) error {
	// ここに定期実行したい処理を記述
	authCtx, err := authenticator.AuthorizedContext(ctx)
	if err != nil {
		return err
	}

	postText := common.GetSalmonSchedule()
	if postText == "" {
		logger.Info("no schedule information available")
		return nil
	} else {
		_, err = apiClient.CreatePost(authCtx, &application_apiv1.CreatePostRequest{
			Text: postText,
		})
		if err != nil {
			return err
		}
	}

    logger.Info("periodic task executed")
    return nil

}
