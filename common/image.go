package common

import (
	"bytes"
	_ "image/jpeg"

	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"net/http"
	"time"

	_ "embed"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

//go:embed assets/fonts/Kosugi-Regular.ttf
var scheduleFontTTF []byte

var scheduleFont *opentype.Font

func loadScheduleFont() (*opentype.Font, error) {
	if scheduleFont != nil {
		return scheduleFont, nil
	}
	f, err := opentype.Parse(scheduleFontTTF)
	if err != nil {
		return nil, fmt.Errorf("failed to parse embedded font: %w", err)
	}
	scheduleFont = f
	return scheduleFont, nil
}

func newFace(size float64) (font.Face, error) {
	f, err := loadScheduleFont()
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

// レイアウト定数
const (
	canvasWidth  = 1200
	canvasHeight = 420

	accentBarWidth = 10

	contentMarginX = 32
	headerHeight   = 76
	contentBottom  = 24

	panelGap      = 24
	leftPanelFrac = 0.56

	panelRadius   = 16
	badgeRadius   = 10
	weaponBoxSize = 96
)

var (
	colorBackground  = color.RGBA{0x20, 0x24, 0x2e, 0xff}
	colorAccent      = color.RGBA{0x8b, 0xc3, 0x4a, 0xff}
	colorRightPanel  = color.RGBA{0x2d, 0x32, 0x3f, 0xff}
	colorWeaponSlot  = color.RGBA{0x18, 0x1a, 0x22, 0xff}
	colorBadge       = color.RGBA{0x00, 0x00, 0x00, 0xb8}
	colorWhite       = color.RGBA{0xff, 0xff, 0xff, 0xff}
	colorBigRunBadge = color.RGBA{0xe0, 0xa5, 0x2f, 0xff}
	colorPlaceholder = color.RGBA{0x3a, 0x3f, 0x4c, 0xff}
)

var httpImageClient = &http.Client{Timeout: 10 * time.Second}

// BuildScheduleImagePNG は、渡されたスケジュール情報から公式アプリ風のシフト画像（PNG）を生成します。
func BuildScheduleImagePNG(r ScheduleResult) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, canvasWidth, canvasHeight))
	draw.Draw(img, img.Bounds(), &image.Uniform{colorBackground}, image.Point{}, draw.Src)
	fillRect(img, image.Rect(0, 0, accentBarWidth, canvasHeight), colorAccent)

	if err := drawHeader(img, r); err != nil {
		return nil, err
	}

	contentWidth := canvasWidth - contentMarginX*2
	leftWidth := int(float64(contentWidth) * leftPanelFrac)
	rightWidth := contentWidth - leftWidth - panelGap

	leftRect := image.Rect(
		contentMarginX, headerHeight,
		contentMarginX+leftWidth, canvasHeight-contentBottom,
	)
	rightRect := image.Rect(
		leftRect.Max.X+panelGap, headerHeight,
		leftRect.Max.X+panelGap+rightWidth, canvasHeight-contentBottom,
	)

	if err := drawStagePanel(img, leftRect, r); err != nil {
		return nil, err
	}
	if err := drawBossPanel(img, rightRect, r); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("failed to encode schedule image: %w", err)
	}
	return buf.Bytes(), nil
}

func drawHeader(img *image.RGBA, r ScheduleResult) error {
	face, err := newFace(26)
	if err != nil {
		return err
	}
	defer face.Close()

	iconRect := image.Rect(contentMarginX, 24, contentMarginX+30, 24+26)
	drawFishIcon(img, iconRect)

	scheduleText := fmt.Sprintf("%s - %s", formatTimeWithWeekdayJST(r.StartTime), formatTimeWithWeekdayJST(r.EndTime))
	drawText(img, face, scheduleText, iconRect.Max.X+14, 24, colorWhite)

	if r.IsBigRun {
		badgeText := "ビッグラン"
		badgeFace, err := newFace(18)
		if err != nil {
			return err
		}
		defer badgeFace.Close()
		w := measureText(badgeFace, badgeText)
		pad := 12
		badgeRect := image.Rect(canvasWidth-contentMarginX-w-pad*2, 22, canvasWidth-contentMarginX, 22+32)
		drawRoundedRect(img, badgeRect, 8, colorBigRunBadge)
		drawTextCentered(img, badgeFace, badgeText, badgeRect, color.RGBA{0x2b, 0x1d, 0x00, 0xff})
	}

	return nil
}

func drawStagePanel(img *image.RGBA, rect image.Rectangle, r ScheduleResult) error {
	stageImg, err := fetchImage(r.Stage.Image)
	if err != nil || stageImg == nil {
		fillRoundedRect(img, rect, panelRadius, colorPlaceholder)
	} else {
		fitted := scaleToFill(stageImg, rect.Dx(), rect.Dy())
		drawImageInRoundedRect(img, fitted, rect, panelRadius)
	}

	labelFace, err := newFace(22)
	if err != nil {
		return err
	}
	defer labelFace.Close()

	pad := 10
	textW := measureText(labelFace, r.Stage.Name)
	badgeRect := image.Rect(
		rect.Min.X+16, rect.Max.Y-16-32-pad,
		rect.Min.X+16+textW+pad*2, rect.Max.Y-16,
	)
	drawRoundedRect(img, badgeRect, badgeRadius, colorBadge)
	drawTextCentered(img, labelFace, r.Stage.Name, badgeRect, colorWhite)

	return nil
}

func drawBossPanel(img *image.RGBA, rect image.Rectangle, r ScheduleResult) error {
	fillRoundedRect(img, rect, panelRadius, colorRightPanel)

	weaponAreaHeight := weaponBoxSize + 32
	nameAreaRect := image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y-weaponAreaHeight)

	bossName := r.Boss.Name
	size := 48.0
	maxWidth := nameAreaRect.Dx() - 40
	var face font.Face
	for {
		f, err := newFace(size)
		if err != nil {
			return err
		}
		w := measureText(f, bossName)
		if w <= maxWidth || size <= 20 {
			face = f
			break
		}
		f.Close()
		size -= 2
	}
	defer face.Close()
	drawTextCentered(img, face, bossName, nameAreaRect, colorWhite)

	weapons := r.Weapons
	if len(weapons) > 4 {
		weapons = weapons[:4]
	}
	n := len(weapons)
	if n == 0 {
		return nil
	}

	slotGap := 12
	totalGap := slotGap * (n - 1)
	availableWidth := rect.Dx() - 32
	slotSize := (availableWidth - totalGap) / n
	if slotSize > weaponBoxSize {
		slotSize = weaponBoxSize
	}
	rowWidth := slotSize*n + totalGap
	startX := rect.Min.X + (rect.Dx()-rowWidth)/2
	slotY := rect.Max.Y - 16 - slotSize

	for i, w := range weapons {
		slotRect := image.Rect(startX+i*(slotSize+slotGap), slotY, startX+i*(slotSize+slotGap)+slotSize, slotY+slotSize)
		fillRoundedRect(img, slotRect, badgeRadius, colorWeaponSlot)

		weaponImg, err := fetchImage(w.Image)
		if err != nil || weaponImg == nil {
			continue
		}
		pad := slotSize / 8
		innerRect := image.Rect(slotRect.Min.X+pad, slotRect.Min.Y+pad, slotRect.Max.X-pad, slotRect.Max.Y-pad)
		fitted := scaleToFit(weaponImg, innerRect.Dx(), innerRect.Dy())
		offsetX := innerRect.Min.X + (innerRect.Dx()-fitted.Bounds().Dx())/2
		offsetY := innerRect.Min.Y + (innerRect.Dy()-fitted.Bounds().Dy())/2
		draw.Draw(img, image.Rect(offsetX, offsetY, offsetX+fitted.Bounds().Dx(), offsetY+fitted.Bounds().Dy()), fitted, image.Point{}, draw.Over)
	}

	return nil
}

// fetchImage は指定URLから画像を取得してデコードします。取得やデコードに失敗した場合は nil を返します。
func fetchImage(url string) (image.Image, error) {
	if url == "" {
		return nil, nil
	}
	resp, err := httpImageClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status fetching image: %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return img, nil
}

// scaleToFill は、指定サイズを埋めるように画像を拡大縮小し、はみ出た部分を中央基準で切り抜きます（カバーフィット）。
func scaleToFill(src image.Image, w, h int) image.Image {
	sb := src.Bounds()
	srcW, srcH := sb.Dx(), sb.Dy()
	if srcW == 0 || srcH == 0 {
		return image.NewRGBA(image.Rect(0, 0, w, h))
	}

	scale := math.Max(float64(w)/float64(srcW), float64(h)/float64(srcH))
	scaledW := int(math.Ceil(float64(srcW) * scale))
	scaledH := int(math.Ceil(float64(srcH) * scale))

	scaled := image.NewRGBA(image.Rect(0, 0, scaledW, scaledH))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), src, sb, xdraw.Over, nil)

	offsetX := (scaledW - w) / 2
	offsetY := (scaledH - h) / 2
	cropped := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(cropped, cropped.Bounds(), scaled, image.Point{offsetX, offsetY}, draw.Src)
	return cropped
}

// scaleToFit は、指定サイズの範囲に収まるようにアスペクト比を保って画像を縮小します（コンテインフィット）。
func scaleToFit(src image.Image, maxW, maxH int) image.Image {
	sb := src.Bounds()
	srcW, srcH := sb.Dx(), sb.Dy()
	if srcW == 0 || srcH == 0 {
		return image.NewRGBA(image.Rect(0, 0, 0, 0))
	}

	scale := math.Min(float64(maxW)/float64(srcW), float64(maxH)/float64(srcH))
	if scale > 1 {
		scale = 1
	}
	dstW := int(float64(srcW) * scale)
	dstH := int(float64(srcH) * scale)
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, sb, xdraw.Over, nil)
	return dst
}

// drawImageInRoundedRect は、rect と同じサイズの画像を、角丸クリッピングしながら描画します。
func drawImageInRoundedRect(dst *image.RGBA, src image.Image, rect image.Rectangle, radius int) {
	w, h := rect.Dx(), rect.Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !insideRoundedRect(x, y, w, h, radius) {
				continue
			}
			dst.Set(rect.Min.X+x, rect.Min.Y+y, src.At(x, y))
		}
	}
}

// fillRoundedRect は、角丸の矩形を単色で塗りつぶします。
func fillRoundedRect(dst *image.RGBA, rect image.Rectangle, radius int, col color.Color) {
	drawRoundedRect(dst, rect, radius, col)
}

func drawRoundedRect(dst *image.RGBA, rect image.Rectangle, radius int, col color.Color) {
	w, h := rect.Dx(), rect.Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !insideRoundedRect(x, y, w, h, radius) {
				continue
			}
			blendPixel(dst, rect.Min.X+x, rect.Min.Y+y, col)
		}
	}
}

func fillRect(dst *image.RGBA, rect image.Rectangle, col color.Color) {
	draw.Draw(dst, rect, &image.Uniform{col}, image.Point{}, draw.Over)
}

func blendPixel(dst *image.RGBA, x, y int, col color.Color) {
	if !(image.Point{x, y}.In(dst.Bounds())) {
		return
	}
	draw.Draw(dst, image.Rect(x, y, x+1, y+1), &image.Uniform{col}, image.Point{}, draw.Over)
}

// insideRoundedRect は、(x,y) が幅 w 高さ h・角丸半径 radius の矩形内に収まるかを判定します。
func insideRoundedRect(x, y, w, h, radius int) bool {
	if radius <= 0 {
		return true
	}
	if x >= radius && x < w-radius {
		return true
	}
	if y >= radius && y < h-radius {
		return true
	}

	var cx, cy int
	switch {
	case x < radius && y < radius:
		cx, cy = radius, radius
	case x >= w-radius && y < radius:
		cx, cy = w-radius-1, radius
	case x < radius && y >= h-radius:
		cx, cy = radius, h-radius-1
	case x >= w-radius && y >= h-radius:
		cx, cy = w-radius-1, h-radius-1
	default:
		return true
	}

	dx := float64(x - cx)
	dy := float64(y - cy)
	return dx*dx+dy*dy <= float64(radius*radius)
}

// drawFishIcon は、シンプルな魚アイコンを塗りつぶし図形で描画します。
func drawFishIcon(img *image.RGBA, rect image.Rectangle) {
	w, h := rect.Dx(), rect.Dy()
	cx, cy := float64(w)*0.4, float64(h)*0.5
	rx, ry := float64(w)*0.38, float64(h)*0.42
	col := color.RGBA{0xe0, 0x5a, 0x47, 0xff}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x), float64(y)
			dx, dy := (fx-cx)/rx, (fy-cy)/ry
			inBody := dx*dx+dy*dy <= 1
			inTail := fx >= cx && (fy-cy) <= (fx-cx)*0.9 && (cy-fy) <= (fx-cx)*0.9 && fx <= float64(w)
			if inBody || inTail {
				img.Set(rect.Min.X+x, rect.Min.Y+y, col)
			}
		}
	}
}

func drawText(img *image.RGBA, face font.Face, s string, x, y int, col color.Color) {
	metrics := face.Metrics()
	baseline := y + metrics.Ascent.Round()
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(x, baseline),
	}
	d.DrawString(s)
}

func drawTextCentered(img *image.RGBA, face font.Face, s string, rect image.Rectangle, col color.Color) {
	w := measureText(face, s)
	metrics := face.Metrics()
	lineHeight := metrics.Ascent.Round() + metrics.Descent.Round()
	x := rect.Min.X + (rect.Dx()-w)/2
	y := rect.Min.Y + (rect.Dy()-lineHeight)/2
	drawText(img, face, s, x, y, col)
}

func measureText(face font.Face, s string) int {
	d := &font.Drawer{Face: face}
	return d.MeasureString(s).Round()
}

// formatTimeWithWeekdayJST は RFC3339 のタイムスタンプを JST の "M/D(曜) HH:MM" 形式に変換します。
func formatTimeWithWeekdayJST(rfc string) string {
	t, err := parseRFC3339JST(rfc)
	if err != nil {
		return rfc
	}
	weekdays := [...]string{"日", "月", "火", "水", "木", "金", "土"}
	return fmt.Sprintf("%d/%d(%s) %02d:%02d", t.Month(), t.Day(), weekdays[t.Weekday()], t.Hour(), t.Minute())
}

// parseRFC3339JST は RFC3339 形式の文字列を JST の time.Time にパースします。
func parseRFC3339JST(rfc string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return time.Time{}, err
	}
	return t.In(time.FixedZone("JST", 9*60*60)), nil
}
