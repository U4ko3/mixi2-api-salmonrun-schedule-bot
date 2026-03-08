package handler

import (
	"context"
	"log/slog"

	"github.com/mixigroup/mixi2-application-sdk-go/auth"
	constv1 "github.com/mixigroup/mixi2-application-sdk-go/gen/go/social/mixi/application/const/v1"
	modelv1 "github.com/mixigroup/mixi2-application-sdk-go/gen/go/social/mixi/application/model/v1"
	application_apiv1 "github.com/mixigroup/mixi2-application-sdk-go/gen/go/social/mixi/application/service/application_api/v1"
	"fmt"
	"net/http"
	"encoding/json"
	"io/ioutil"
	"strings"
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

// Handler implements event.EventHandler interface.
type Handler struct {
	logger        *slog.Logger
	apiClient     application_apiv1.ApplicationServiceClient
	authenticator auth.Authenticator
}

// NewHandler creates a new Handler.
func NewHandler(apiClient application_apiv1.ApplicationServiceClient, authenticator auth.Authenticator) *Handler {
	return &Handler{
		logger:        slog.Default(),
		apiClient:     apiClient,
		authenticator: authenticator,
	}
}

// Handle processes events from mixi2.
func (h *Handler) Handle(ctx context.Context, ev *modelv1.Event) error {
	switch ev.EventType {
	case constv1.EventType_EVENT_TYPE_POST_CREATED:
		h.logger.Info("received POST_CREATED event",
			slog.String("event_id", ev.EventId),
		)
		// Add your post created event handling logic here
	case constv1.EventType_EVENT_TYPE_CHAT_MESSAGE_RECEIVED:
		h.logger.Info("received CHAT_MESSAGE_RECEIVED event",
			slog.String("event_id", ev.EventId),
		)
		if err := h.handleChatMessage(ctx, ev.GetChatMessageReceivedEvent()); err != nil {
			h.logger.Error("failed to handle chat message", slog.String("error", err.Error()))
			return err
		}
	default:
		h.logger.Info("received event",
			slog.String("event_id", ev.EventId),
			slog.Int("event_type", int(ev.EventType)),
		)
	}
	return nil
}

// handleChatMessage handles chat message received events by echoing the message back.
func (h *Handler) handleChatMessage(ctx context.Context, ev *modelv1.ChatMessageReceivedEvent) error {
	msg := ev.GetMessage()
	if msg == nil {
		return nil
	}

	text := msg.GetText()
	if text == "" {
		return nil
	}

	authCtx, err := h.authenticator.AuthorizedContext(ctx)
	if err != nil {
		return err
	}

	_, err = h.apiClient.SendChatMessage(authCtx, &application_apiv1.SendChatMessageRequest{
		RoomId: msg.GetRoomId(),
		Text:   &text,
	})
	if err != nil {
		return err
	}

	h.logger.Info("echoed chat message",
		slog.String("room_id", msg.GetRoomId()),
		slog.String("text", text),
	)

	// http.Getを用いて外部APIを呼び出す
	resp, err := http.Get("https://spla3.yuu26.com/api/coop-grouping/schedule")
	// _, err = http.Get("https://spla3.yuu26.com/api/coop-grouping/schedule")
	// エラーハンドリング
	if err != nil {
		fmt.Printf("The HTTP request failed with error %s\n", err)
	} else {
		fmt.Println("The HTTP request succeeded")
		data, _ := ioutil.ReadAll(resp.Body)

		var responseObject APIResponse
		// json.UnmarshalでJSONデータをGoのオブジェクトに変換する
		json.Unmarshal(data, &responseObject)

		var sb strings.Builder
		sb.WriteString("■現在のステージ情報:\n")
		sb.WriteString(responseObject.Results[0].StartTime)
		sb.WriteString("　～　")
		sb.WriteString(responseObject.Results[0].EndTime)
		sb.WriteString("\n")
		sb.WriteString("　ステージ: ")
		sb.WriteString(responseObject.Results[0].Stage.Name)
		sb.WriteString("\n")
		sb.WriteString("　ボス: ")
		sb.WriteString(responseObject.Results[0].Boss.Name)
		sb.WriteString("　ブキ: ")
		sb.WriteString(" ／ ")
		sb.WriteString(responseObject.Results[0].Weapons[0].Name)
		sb.WriteString(" ／ ")
		sb.WriteString(responseObject.Results[0].Weapons[1].Name)
		sb.WriteString(" ／ ")
		sb.WriteString(responseObject.Results[0].Weapons[2].Name)
		sb.WriteString(" ／ ")
		sb.WriteString(responseObject.Results[0].Weapons[3].Name)
		sb.WriteString("\n")

		resultText := sb.String()
		fmt.Println(resultText)

		_, err = h.apiClient.CreatePost(authCtx, &application_apiv1.CreatePostRequest{
		    Text: resultText,
		})
		if err != nil {
			return err
		}


	}



	return nil
}
