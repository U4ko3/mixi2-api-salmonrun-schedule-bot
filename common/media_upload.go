package common

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mixigroup/mixi2-application-sdk-go/auth"
	application_apiv1 "github.com/mixigroup/mixi2-application-sdk-go/gen/go/social/mixi/application/service/application_api/v1"
	"google.golang.org/grpc/status"
)

const (
	mediaUploadContentType  = "image/png"
	mediaStatusPollInterval = 1 * time.Second
	mediaStatusTimeout      = 30 * time.Second
)

var mediaUploadHTTPClient = &http.Client{Timeout: 30 * time.Second}

// BuildAndUploadScheduleImage は、与えられたスケジュール情報から画像を生成してmixi2にアップロードし、
// CreatePostRequest.MediaIdList に指定できるメディアID一覧を返します。
// 画像の生成・アップロードに失敗した場合はログを出力した上で nil を返すため、
// 呼び出し元は本文のみのテキスト投稿にフォールバックできます。
func BuildAndUploadScheduleImage(ctx context.Context, apiClient application_apiv1.ApplicationServiceClient, authenticator auth.Authenticator, r *ScheduleResult) (mediaIds []string) {
	if r == nil {
		return nil
	}

	// 画像生成・アップロードの過程で予期しないpanicが起きても、投稿処理全体を巻き込まないようにする。
	defer func() {
		if rec := recover(); rec != nil {
			fmt.Printf("failed to build/upload schedule image (recovered from panic): %v\n", rec)
			mediaIds = nil
		}
	}()

	imgBytes, err := BuildScheduleImagePNG(*r)
	if err != nil {
		fmt.Printf("failed to build schedule image: %v\n", err)
		return nil
	}

	mediaId, err := uploadImageMedia(ctx, apiClient, authenticator, imgBytes)
	if err != nil {
		fmt.Printf("failed to upload schedule image: %v\n", err)
		return nil
	}

	return []string{mediaId}
}

// uploadImageMedia は、mixi2のメディアアップロードAPIを使って画像をアップロードし、メディアIDを返します。
func uploadImageMedia(ctx context.Context, apiClient application_apiv1.ApplicationServiceClient, authenticator auth.Authenticator, imgBytes []byte) (string, error) {
	initResp, err := apiClient.InitiatePostMediaUpload(ctx, &application_apiv1.InitiatePostMediaUploadRequest{
		ContentType: mediaUploadContentType,
		DataSize:    uint64(len(imgBytes)),
		MediaType:   application_apiv1.InitiatePostMediaUploadRequest_TYPE_IMAGE,
	})
	if err != nil {
		return "", fmt.Errorf("initiate media upload: %w", describeGRPCErr(err))
	}

	accessToken, err := authenticator.GetAccessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("get access token: %w", err)
	}

	if err := postMediaBytes(ctx, initResp.GetUploadUrl(), accessToken, imgBytes); err != nil {
		return "", fmt.Errorf("upload media bytes: %w", err)
	}

	if err := waitForMediaProcessing(ctx, apiClient, initResp.GetMediaId()); err != nil {
		return "", fmt.Errorf("wait for media processing: %w", describeGRPCErr(err))
	}

	return initResp.GetMediaId(), nil
}

// postMediaBytes は、アップロード先URLに画像データをアップロードします。
// このURLは mixi2 側のアップロードエンドポイントであり、POSTメソッドかつ
// Authorizationヘッダー（Bearerトークン）が必要です。
func postMediaBytes(ctx context.Context, uploadURL, accessToken string, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mediaUploadContentType)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.ContentLength = int64(len(data))

	resp, err := mediaUploadHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}
	return nil
}

func waitForMediaProcessing(ctx context.Context, apiClient application_apiv1.ApplicationServiceClient, mediaId string) error {
	deadline := time.Now().Add(mediaStatusTimeout)

	for {
		statusResp, err := apiClient.GetPostMediaStatus(ctx, &application_apiv1.GetPostMediaStatusRequest{
			MediaId: mediaId,
		})
		if err != nil {
			return err
		}

		switch statusResp.GetStatus() {
		case application_apiv1.GetPostMediaStatusResponse_STATUS_COMPLETED:
			return nil
		case application_apiv1.GetPostMediaStatusResponse_STATUS_FAILED:
			return fmt.Errorf("media processing failed")
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for media processing")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(mediaStatusPollInterval):
		}
	}
}

// describeGRPCErr は、grpcのステータスコードが取れる場合にそれを含めたエラーを返します。
func describeGRPCErr(err error) error {
	if st, ok := status.FromError(err); ok {
		return fmt.Errorf("grpc code=%s message=%s", st.Code(), st.Message())
	}
	return err
}
