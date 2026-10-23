package ui

import (
	"image"

	xdraw "golang.org/x/image/draw"
)

// CursorIcon is the icon of what the cursor carries (a picked up item or
// spell), scaled to size pixels square. The key changes when the carried thing
// does, so the caller only rebuilds its texture then. It returns "" and nil
// when the cursor holds nothing.
func (eng *UIEngine) CursorIcon(size int) (string, *image.RGBA) {
	if eng == nil || eng.Rt == nil || size < 1 {
		return "", nil
	}
	cursor := eng.Rt.gameplay.Cursor
	if cursor.ID == 0 {
		return "", nil
	}
	icon := ""
	switch cursor.Type {
	case 0:
		icon = eng.Rt.gameplay.Catalog[cursor.ID].Icon
	case 128:
		icon = eng.Rt.itemIcon(cursor.ID)
	}
	if icon == "" {
		return "", nil
	}
	if eng.cursorIconPath == icon && eng.cursorIconImage != nil && eng.cursorIconImage.Bounds().Dx() == size {
		return icon, eng.cursorIconImage
	}
	source := eng.loadBLP(icon)
	if source == nil {
		return "", nil
	}
	scaled := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.BiLinear.Scale(scaled, scaled.Bounds(), source, source.Bounds(), xdraw.Src, nil)
	eng.cursorIconPath, eng.cursorIconImage = icon, scaled
	return icon, scaled
}

// CursorPosition is where the interface last saw the pointer, in pixels from
// the top left of the window.
func (eng *UIEngine) CursorPosition() (float64, float64) {
	if eng == nil || eng.Rt == nil {
		return 0, 0
	}
	return eng.Rt.cursorX, eng.Rt.cursorY
}

// UIScale is the pixel size of one interface unit.
func (eng *UIEngine) UIScale() float64 { return eng.uiScale }
