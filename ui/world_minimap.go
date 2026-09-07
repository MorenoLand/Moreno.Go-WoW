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

func (eng *UIEngine) SetWorldMinimapPosition(mapName string, worldX, worldY float32) bool {
	if eng == nil || eng.AssetLoader == nil || eng.Rt == nil {
		return false
	}
	mapName = strings.TrimSpace(mapName)
	if mapName == "" {
		return false
	}
	tileX, tileY := worldMinimapTile(worldX, worldY)
	if eng.minimapHasPosition && eng.minimapMapName == mapName && eng.minimapTileX == tileX && eng.minimapTileY == tileY && math.Hypot(float64(worldX-eng.minimapWorldX), float64(worldY-eng.minimapWorldY)) < 6 {
		return false
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
	half := 400.0 / (worldMinimapTileSize * 3)
	mapWidget := eng.Rt.widgets["MinimapMap"]
	if mapWidget == nil {
		return false
	}
	mapWidget.texCoordL = playerU - half
	mapWidget.texCoordR = playerU + half
	mapWidget.texCoordT = playerV - half
	mapWidget.texCoordB = playerV + half
	eng.minimapImage = composite
	eng.minimapMapName = mapName
	eng.minimapTileX = tileX
	eng.minimapTileY = tileY
	eng.minimapWorldX = worldX
	eng.minimapWorldY = worldY
	eng.minimapHasPosition = true
	return true
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
