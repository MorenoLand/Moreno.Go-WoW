package ui

import (
	"image"
	"image/color"
	"strings"

	"golang.org/x/image/font"
)

func (eng *UIEngine) drawMessageLines(canvas *image.RGBA, w *widget, rect Rect, face, faceLarge font.Face, screenHeight float64) {
	fontWidget := w
	for _, child := range w.children {
		if child.kind == kindFontString {
			fontWidget = child
			break
		}
	}
	messageFace := eng.faceFor(fontWidget, face, faceLarge)
	if messageFace == nil {
		return
	}
	padding := 8.0
	textRect := Rect{X0: rect.X0 + padding, Y0: rect.Y0 + 4, X1: rect.X1 - padding, Y1: rect.Y1 - 4}
	if textRect.W() <= 0 || textRect.H() <= 0 {
		return
	}
	maxWidth := int(textRect.W() * eng.uiScale)
	lines := make([]messageLine, 0, len(w.messages))
	for _, message := range w.messages {
		text := cleanChatMarkup(message.text)
		for _, line := range wrapText(text, messageFace, maxWidth) {
			message.text = line
			lines = append(lines, message)
		}
	}
	lineHeight := float64(messageFace.Metrics().Height.Ceil()) / eng.uiScale
	if lineHeight <= 0 {
		return
	}
	visible := int(textRect.H() / lineHeight)
	if visible < 1 {
		return
	}
	if len(lines) > visible {
		lines = lines[len(lines)-visible:]
	}
	for index := len(lines) - 1; index >= 0; index-- {
		line := lines[index]
		bottom := textRect.Y0 + float64(len(lines)-1-index+1)*lineHeight
		lineRect := Rect{X0: textRect.X0, Y0: bottom - lineHeight, X1: textRect.X1, Y1: bottom}
		c := color.RGBA{R: uint8(line.color.r * 255), G: uint8(line.color.g * 255), B: uint8(line.color.b * 255), A: uint8(line.color.a * 255)}
		eng.drawTextAlignedWidget(canvas, messageFace, line.text, screenScaledRect(lineRect, eng.uiScale), screenHeight, c, fontWidget)
	}
}

func cleanChatMarkup(text string) string {
	text = cleanText(text)
	for {
		start := strings.Index(text, "|H")
		if start < 0 {
			break
		}
		end := strings.Index(text[start:], "|h")
		if end < 0 {
			break
		}
		text = text[:start] + text[start+end+2:]
	}
	text = strings.ReplaceAll(text, "|h", "")
	for {
		start := strings.Index(text, "|T")
		if start < 0 {
			break
		}
		end := strings.Index(text[start:], "|t")
		if end < 0 {
			break
		}
		text = text[:start] + text[start+end+2:]
	}
	return text
}
