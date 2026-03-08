package main

import (
	"context"
	"crypto/tls"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/mixigroup/mixi2-application-sample-go/config"
	"github.com/mixigroup/mixi2-application-sample-go/common"
	"github.com/mixigroup/mixi2-application-sample-go/handler"
	"github.com/mixigroup/mixi2-application-sdk-go/auth"
	"github.com/mixigroup/mixi2-application-sdk-go/event/stream"
	application_apiv1 "github.com/mixigroup/mixi2-application-sdk-go/gen/go/social/mixi/application/service/application_api/v1"
	application_streamv1 "github.com/mixigroup/mixi2-application-sdk-go/gen/go/social/mixi/application/service/application_stream/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type APIResponse struct {
	Results []struct {
        StartTime string `json:"start_time"`
        EndTime   string `json:"end_time"`
		Boss struct {
			Name string `json:"name"`
		} `json:"boss"`
		Stage struct {
			Name string `json:"name"`
		} `json:"stage"`
		Weapons []struct {
			Name string `json:"name"`
		} `json:"weapons"`
	} `json:"results"`
}

func main() {
	cfg := config.GetConfig()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create authenticator
	authenticator, err := auth.NewAuthenticator(cfg.ClientID, cfg.ClientSecret, cfg.TokenURL)
	if err != nil {
		log.Fatalf("failed to create authenticator: %v", err)
	}

	// Create gRPC connection for stream
	streamConn, err := grpc.NewClient(
		cfg.StreamAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})),
	)
	if err != nil {
		log.Fatalf("failed to connect to stream: %v", err)
	}
	defer streamConn.Close()

	// Create gRPC connection for API
	apiConn, err := grpc.NewClient(
		cfg.APIAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})),
	)
	if err != nil {
		log.Fatalf("failed to connect to api: %v", err)
	}
	defer apiConn.Close()

	// Create stream client and watcher
	streamClient := application_streamv1.NewApplicationServiceClient(streamConn)
	watcher := stream.NewStreamWatcher(streamClient, authenticator, stream.WithLogger(logger))

	// Create API client
	apiClient := application_apiv1.NewApplicationServiceClient(apiConn)

	// Create event handler
	eventHandler := handler.NewHandler(apiClient, authenticator)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutting down...")
		cancel()
	}()

	//// Start watching
	//logger.Info("starting stream watcher", slog.String("address", cfg.StreamAddress))
	//if err := watcher.Watch(ctx, eventHandler); err != nil {
	//	if err != context.Canceled {
	//		log.Fatalf("watcher error: %v", err)
	//	}
	//}
	//logger.Info("stopped")


    // WaitGroupで複数のゴルーチンの完了を待つ
    var wg sync.WaitGroup

    // 定期処理を実行するゴルーチン
    wg.Add(1)
    go func() {
        defer wg.Done()
        ticker := time.NewTicker(10 * time.Second) // 30秒ごとに実行
        defer ticker.Stop()

        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                logger.Info("executing periodic task")
                // ここに定期実行したい処理を記述
                // 例: データベース更新、ログ収集、ヘルスチェックなど
                if err := periodicTask(ctx, apiClient, authenticator, logger); err != nil {
                    logger.Error("periodic task error", slog.String("error", err.Error()))
                }
            }
        }
    }()

    // ストリーム監視を実行するゴルーチン
    wg.Add(1)
    go func() {
        defer wg.Done()
        logger.Info("starting stream watcher", slog.String("address", cfg.StreamAddress))
        if err := watcher.Watch(ctx, eventHandler); err != nil {
            if err != context.Canceled {
                log.Fatalf("watcher error: %v", err)
            }
        }
    }()

    // 全てのゴルーチンの完了を待つ
    wg.Wait()
    logger.Info("stopped")
}

// 定期実行する処理
func periodicTask(ctx context.Context, apiClient application_apiv1.ApplicationServiceClient, authenticator auth.Authenticator, logger *slog.Logger) error {

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

