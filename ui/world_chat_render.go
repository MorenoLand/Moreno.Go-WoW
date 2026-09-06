package ui

import (
	"image"
	"image/color"
	"strings"

	"golang.org/x/image/font"
)

func (eng *UIEngine) drawMessageLines(canvas *image.RGBA, w *widget, rect Rect, face, faceLarge font.Face, screenHeight float64) {
	if eng == nil || w == nil || canvas == nil || len(w.messages) == 0 {
		return
	}
	scale := eng.uiScale
	if scale <= 0 {
		scale = 1
	}
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
	maxWidth := int(textRect.W() * scale)
	lines := make([]messageLine, 0, len(w.messages))
	for _, message := range w.messages {
		text := cleanChatMarkup(message.text)
		for _, line := range wrapText(text, messageFace, maxWidth) {
			message.text = line
			lines = append(lines, message)
		}
	}
	lineHeight := float64(messageFace.Metrics().Height.Ceil()) / scale
	if lineHeight <= 0 {
		return
	}
	visible := int(textRect.H() / lineHeight)
	if visible < 1 {
		return
	}
	end := len(lines) - w.messageOffset
	if end < 0 {
		end = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	start := end - visible
	if start < 0 {
		start = 0
	}
	if start < end {
		lines = lines[start:end]
	} else {
		lines = nil
	}

	// Clip message glyphs to the scrolling message frame so a wrong-scale
	// draw cannot spill onto the world below the chat chrome.
	clip := ScreenRect(screenScaledRect(rect, scale), screenHeight).Intersect(canvas.Bounds())
	if clip.Empty() {
		return
	}
	target := canvas.SubImage(clip).(*image.RGBA)

	for index := len(lines) - 1; index >= 0; index-- {
		line := lines[index]
		bottom := textRect.Y0 + float64(len(lines)-1-index+1)*lineHeight
		lineRect := Rect{X0: textRect.X0, Y0: bottom - lineHeight, X1: textRect.X1, Y1: bottom}
		c := color.RGBA{
			R: clampColorByte(line.color.r),
			G: clampColorByte(line.color.g),
			B: clampColorByte(line.color.b),
			A: clampColorByte(line.color.a),
		}
		// Pass a scaled rect once; faceFor already baked uiScale into glyph size.
		eng.drawTextAlignedWidget(target, messageFace, line.text, screenScaledRect(lineRect, scale), screenHeight, c, fontWidget)
	}
}

func clampColorByte(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return uint8(v * 255)
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
