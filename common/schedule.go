package common

import (
    "encoding/json"
    "fmt"
    "io/ioutil"
    "net/http"
    "strings"
    "time"
)

const (
    // SalmonScheduleAPIURL はサーモンラン スケジュールAPI のURL
    SalmonScheduleAPIURL = "https://spla3.yuu26.com/api/coop-grouping/schedule"
)

// ScheduleResult represents a single schedule result
type ScheduleResult struct {
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
    IsBigRun bool `json:"is_big_run"`
}

// APIResponse is used to unmarshal the salmon schedule JSON.
type APIResponse struct {
    Results []ScheduleResult `json:"results"`
}

// GetSalmonSchedule は現在のサーモンランスケジュールをフェッチし、フォーマットします。
// 他のパッケージで使用するためにエクスポートされています。
// フォーマットされたスケジュール文字列と、残り時間短いかどうかを示すブールを返します。
func GetSalmonSchedule() (string, bool) {
    resp, err := http.Get(SalmonScheduleAPIURL)
    if err != nil {
        fmt.Printf("The HTTP request failed with error %s\n", err)
        return "", false
    }

    fmt.Println("The HTTP request succeeded")
    data, _ := ioutil.ReadAll(resp.Body)

    var responseObject APIResponse
    if err := json.Unmarshal(data, &responseObject); err != nil {
        fmt.Printf("JSON unmarshal failed: %v\n", err)
        return "", false
    }

    result := ""
    lastRun := false

    // APIのresultsが空でない場合、最初のスケジュール情報をフォーマットして返す
    if len(responseObject.Results) == 0 {
        return "", false
    } else {
        nextTime := diffStartEndTime(responseObject.Results[0].EndTime)
        if nextTime > 10 {
            // 10時間以上残っている場合は次のスケジュールを表示
            if (responseObject.Results[0].IsBigRun) {
                result = "■ 現在のステージ情報  ★ビッグラン開催中★\n"
            } else {
                result = "■ 現在のステージ情報\n"
            }
            result += formatScheduleInfo(responseObject.Results[0])
        } else if nextTime < 5 {
            // 5時間未満の場合は次のスケジュールも表示
            result += "■ 次のステージ情報　"
            result += fmt.Sprintf("【開始まであと %d 時間！】\n", nextTime)
            result += formatScheduleInfo(responseObject.Results[1])
        } else {
            // 5時間以上10時間未満の場合は現在のスケジュールを表示
            if (responseObject.Results[0].IsBigRun) {
                result = "■ 現在のステージ情報 " + fmt.Sprintf("【ビッグラン終了まであと %d 時間！】\n", nextTime)
            } else {
                result = "■ 現在のステージ情報 " + fmt.Sprintf("【終了まであと %d 時間！】\n", nextTime)
            }
            result += formatScheduleInfo(responseObject.Results[0])
            lastRun = true
        }
            
        result += "\n\n"
    }

    return result, lastRun
}

// 現在のスケジュールを取得する関数
func GetCurrentSalmonSchedule() string {
    resp, err := http.Get(SalmonScheduleAPIURL)
    if err != nil {
        fmt.Printf("The HTTP request failed with error %s\n", err)
        return ""
    }

    fmt.Println("The HTTP request succeeded")
    data, _ := ioutil.ReadAll(resp.Body)

    var responseObject APIResponse
    if err := json.Unmarshal(data, &responseObject); err != nil {
        fmt.Printf("JSON unmarshal failed: %v\n", err)
        return ""
    }

    result := ""

    // APIのresultsが空でない場合、最初のスケジュール情報をフォーマットして返す
    if len(responseObject.Results) == 0 {
        return ""
    } else {
        nextTime := diffStartEndTime(responseObject.Results[0].EndTime)
        result = "■ 現在のステージ情報 " + fmt.Sprintf("【終了まであと %d 時間！】\n", nextTime)
        result += formatScheduleInfo(responseObject.Results[0])
            
        result += "\n\n"
    }

    return result
}

// 次のスケジュールを取得する関数
func GetNextSalmonSchedule() string {
    resp, err := http.Get(SalmonScheduleAPIURL)
    if err != nil {
        fmt.Printf("The HTTP request failed with error %s\n", err)
        return ""
    }

    fmt.Println("The HTTP request succeeded")
    data, _ := ioutil.ReadAll(resp.Body)

    var responseObject APIResponse
    if err := json.Unmarshal(data, &responseObject); err != nil {
        fmt.Printf("JSON unmarshal failed: %v\n", err)
        return ""
    }

    result := ""

    // APIのresultsが空でない場合、最初のスケジュール情報をフォーマットして返す
    if len(responseObject.Results) == 0 {
        return ""
    } else {
        nextTime := diffStartEndTime(responseObject.Results[0].EndTime)
        result = "■ 次のステージ情報 " + fmt.Sprintf("【開始まであと %d 時間！】\n", nextTime)
        result += formatScheduleInfo(responseObject.Results[1])
            
        result += "\n\n"
    }

    return result
}

// formatScheduleInfo formats schedule information from API results
func formatScheduleInfo(result ScheduleResult) string {
    var sb strings.Builder
    r := result

    sb.WriteString("・スケジュール：")
    sb.WriteString(formatTimeJST(r.StartTime))
    sb.WriteString("　～　")
    sb.WriteString(formatTimeJST(r.EndTime))
    sb.WriteString("\n")

    sb.WriteString("・ステージ　　：")
    sb.WriteString(getStageStr(r.Stage.Name))
    sb.WriteString("\n")

    sb.WriteString("・オオモノ　　：")
    sb.WriteString(r.Boss.Name)
    sb.WriteString("\n")

    sb.WriteString("・ブキ　　　　：")
    sb.WriteString("\n")
    for _, w := range r.Weapons {
        sb.WriteString("　　" + w.Name)
        sb.WriteString("\n")
    }
    sb.WriteString("\n")

    return sb.String()
}

// ２つの時間差を取得し、EndTimeと現在日時の差分を計算
func diffStartEndTime(EndTime string) int {
    // フォーマット: time.RFC3339 (例: "2006-01-02T15:04:05Z07:00")
    layout := time.RFC3339

    t2, err2 := time.Parse(layout, EndTime)
    if err2 != nil {
        fmt.Println("エラー: 終了時間のパースに失敗しました", err2)
        return -1
    }

    // 現在時刻を取得
    t1 := time.Now()

    duration := t2.Sub(t1)
    hours := int(duration.Hours())

    return hours
}

// formatTimeJST は RFC3339 形式のタイムスタンプを JST の "MM/DD HH:MM" 形式に変換する。
// パースに失敗した場合は元の文字列を返す。
func formatTimeJST(rfc string) string {

    if t, err := time.Parse(time.RFC3339, rfc); err == nil {
        return t.In(time.FixedZone("JST", 9*60*60)).Format("01/02 15:04")
    }
    return rfc
}

// ステージ名に対応する絵文字を付与する関数
func getStageStr(stage string) string {
    result := ""

    switch stage {
    case "アラマキ砦":
        result = "🐚 "
    case "ムニ・エール海洋発電所":
        result = "🕰️ "
    case "シェケナダム":
        result = "🏞️ "
    case "難破船ドン・ブラコ":
        result = "🛳️ "
    case "すじこジャンクション跡":
        result = "🛣️ "
    case "トキシラズいぶし工房":
        result = "🕰️ "
    case "どんぴこ闘技場":
        result = "⚔️️ "
    default:
        result = "️"
    }

    result += stage

    return result
}


