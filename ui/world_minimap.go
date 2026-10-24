package ui

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	xdraw "golang.org/x/image/draw"
)

const worldMinimapTileSize = 533.333333

// minimapZoomLevels, minimapDefaultZoom and minimapOutdoorRadius come from
// Wow.exe: the zoom setter clamps to 0..5, the level starts at 3, and the
// outdoor radius in yards is {14,12,10,8,6,4} * 0.5 * 33.333.
const (
	minimapZoomLevels  = 6
	minimapDefaultZoom = 3
)

var minimapOutdoorRadius = [minimapZoomLevels]float64{14 * 0.5 * 33.333332, 12 * 0.5 * 33.333332, 10 * 0.5 * 33.333332, 8 * 0.5 * 33.333332, 6 * 0.5 * 33.333332, 4 * 0.5 * 33.333332}

// zoomLevel is the minimap zoom, its default until a script sets one.
func (w *widget) zoomLevel() int {
	if !w.zoomSet {
		return minimapDefaultZoom
	}
	return w.zoom
}

// applyMinimapZoom sets which part of the tile composite the minimap shows.
func (eng *UIEngine) applyMinimapZoom() {
	if eng == nil || eng.Rt == nil || !eng.minimapHasPosition {
		return
	}
	mapWidget := eng.Rt.widgets["MinimapMap"]
	minimap := eng.Rt.widgets["Minimap"]
	if mapWidget == nil {
		return
	}
	if eng.minimapIndoor {
		// The indoor view is drawn for the zoom it is asked at: all of it shows.
		mapWidget.texCoordL, mapWidget.texCoordR, mapWidget.texCoordT, mapWidget.texCoordB = 0, 1, 0, 1
		eng.minimapVersion++
		return
	}
	level := minimapDefaultZoom
	if minimap != nil {
		level = minimap.zoomLevel()
	}
	half := minimapOutdoorRadius[level] / (worldMinimapTileSize * 3)
	mapWidget.texCoordL = eng.minimapU - half
	mapWidget.texCoordR = eng.minimapU + half
	mapWidget.texCoordT = eng.minimapV - half
	mapWidget.texCoordB = eng.minimapV + half
	eng.minimapVersion++
}

func (eng *UIEngine) SetWorldMinimapPosition(mapName string, worldX, worldY float32) bool {
	if eng == nil || eng.AssetLoader == nil || eng.Rt == nil {
		return false
	}
	mapName = strings.TrimSpace(mapName)
	if mapName == "" {
		return false
	}
	if eng.minimapIndoor {
		eng.minimapWorldX, eng.minimapWorldY = worldX, worldY
		return false
	}
	tileX, tileY := worldMinimapTile(worldX, worldY)
	if eng.minimapHasPosition && eng.minimapMapName == mapName && eng.minimapTileX == tileX && eng.minimapTileY == tileY {
		// Same tiles: only the window onto them moves, which needs no new composite.
		if math.Hypot(float64(worldX-eng.minimapWorldX), float64(worldY-eng.minimapWorldY)) < 1 {
			return false
		}
		eng.minimapU = (1 + (32 - float64(tileY) - float64(worldX)/worldMinimapTileSize)) / 3
		eng.minimapV = (1 + (32 - float64(tileX) - float64(worldY)/worldMinimapTileSize)) / 3
		eng.minimapWorldX, eng.minimapWorldY = worldX, worldY
		eng.applyMinimapZoom()
		return true
	}
	eng.loadMinimapTRS()
	composite := image.NewRGBA(image.Rect(0, 0, 768, 768))
	draw.Draw(composite, composite.Bounds(), &image.Uniform{C: color.RGBA{R: 12, G: 20, B: 30, A: 255}}, image.Point{}, draw.Src)
	for row := -1; row <= 1; row++ {
		for column := -1; column <= 1; column++ {
			key := strings.ToLower(fmt.Sprintf("%s\\map%d_%d", mapName, tileX+row, tileY+column))
			hash := eng.minimapTRS[key]
			if hash == "" {
				continue
			}
			img := eng.loadBLP(`Textures\Minimap\` + hash)
			if img == nil {
				continue
			}
			img = transposeMinimapTile(img)
			dst := image.Rect((column+1)*256, (row+1)*256, (column+2)*256, (row+2)*256)
			if img.Bounds().Dx() == dst.Dx() && img.Bounds().Dy() == dst.Dy() {
				draw.Draw(composite, dst, img, img.Bounds().Min, draw.Src)
			} else {
				xdraw.NearestNeighbor.Scale(composite, dst, img, img.Bounds(), draw.Src, nil)
			}
		}
	}
	fracNorth := 32 - float64(tileX) - float64(worldY)/worldMinimapTileSize
	fracWest := 32 - float64(tileY) - float64(worldX)/worldMinimapTileSize
	playerU := (1 + fracWest) / 3
	playerV := (1 + fracNorth) / 3
	if eng.Rt.widgets["MinimapMap"] == nil {
		return false
	}
	eng.minimapU, eng.minimapV = playerU, playerV
	eng.minimapImage = composite
	eng.minimapMapName = mapName
	eng.minimapTileX = tileX
	eng.minimapTileY = tileY
	eng.minimapWorldX = worldX
	eng.minimapWorldY = worldY
	eng.minimapHasPosition = true
	eng.applyMinimapZoom()
	return true
}

// MinimapZoom is the zoom level the minimap is at, 0 the widest.
func (eng *UIEngine) MinimapZoom() int {
	if eng == nil || eng.Rt == nil {
		return minimapDefaultZoom
	}
	if minimap := eng.Rt.widgets["Minimap"]; minimap != nil {
		return minimap.zoomLevel()
	}
	return minimapDefaultZoom
}

// MinimapZoomLevels is how many zoom levels the minimap has.
func MinimapZoomLevels() int { return minimapZoomLevels }

// MinimapIndoor reports whether the minimap shows an indoor view.
func (eng *UIEngine) MinimapIndoor() bool { return eng != nil && eng.minimapIndoor }

// SetWorldMinimapIndoor puts a picture of the rooms around the player in the
// minimap, as it does inside a building; a nil picture goes back to the map.
// It reports whether the minimap changed.
func (eng *UIEngine) SetWorldMinimapIndoor(view image.Image) bool {
	if eng == nil || eng.Rt == nil || eng.Rt.widgets["MinimapMap"] == nil {
		return false
	}
	if view == nil {
		if !eng.minimapIndoor {
			return false
		}
		eng.minimapIndoor = false
		// The map is composed again from the tiles at the next position.
		eng.minimapHasPosition = false
		eng.minimapImage = nil
		eng.minimapVersion++
		return true
	}
	if !eng.minimapIndoor {
		eng.minimapIndoor = true
		eng.minimapHasPosition = true
	}
	eng.minimapImage = view
	eng.applyMinimapZoom()
	return true
}

func (eng *UIEngine) WorldMinimapVersion() uint64 {
	if eng == nil {
		return 0
	}
	return eng.minimapVersion
}

func transposeMinimapTile(source image.Image) image.Image {
	bounds := source.Bounds()
	result := image.NewRGBA(image.Rect(0, 0, bounds.Dy(), bounds.Dx()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			result.Set(y, x, source.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return result
}

func (eng *UIEngine) loadMinimapTRS() {
	if eng.minimapTRSLoaded {
		return
	}
	eng.minimapTRSLoaded = true
	eng.minimapTRS = make(map[string]string)
	data, err := eng.AssetLoader.ReadFile(`Textures\Minimap\md5translate.trs`)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(strings.ToLower(line), "dir:") {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(parts[0])), ".blp")
		hash := strings.TrimSuffix(strings.TrimSpace(parts[1]), ".blp")
		if key != "" && hash != "" {
			eng.minimapTRS[key] = hash
		}
	}
}

func worldMinimapTile(worldX, worldY float32) (int, int) {
	tileX := int(math.Floor(32 - float64(worldY)/worldMinimapTileSize))
	tileY := int(math.Floor(32 - float64(worldX)/worldMinimapTileSize))
	if tileX < 0 {
		tileX = 0
	}
	if tileX > 63 {
		tileX = 63
	}
	if tileY < 0 {
		tileY = 0
	}
	if tileY > 63 {
		tileY = 63
	}
	return tileX, tileY
}
