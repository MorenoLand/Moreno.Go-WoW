package ui

// clampToScreen slides a frame that is clamped to the screen back inside it.
// The insets (left, right, top, bottom) let it go that far past each edge.
// The left and bottom edges are held first, then the right and top ones, so a
// frame larger than the screen keeps its right and top edges in view.
func clampToScreen(rect, screen Rect, scale float64, insets [4]float64) Rect {
	minX := screen.X0 - scale*insets[0]
	maxX := screen.X1 - scale*insets[1]
	maxY := screen.Y1 - scale*insets[2]
	minY := screen.Y0 - scale*insets[3]
	if minX > rect.X0 {
		rect.X1 += minX - rect.X0
		rect.X0 = minX
	}
	if minY > rect.Y0 {
		rect.Y1 += minY - rect.Y0
		rect.Y0 = minY
	}
	if maxX < rect.X1 {
		rect.X0 -= rect.X1 - maxX
		rect.X1 = maxX
	}
	if maxY < rect.Y1 {
		rect.Y0 -= rect.Y1 - maxY
		rect.Y1 = maxY
	}
	return rect
}
