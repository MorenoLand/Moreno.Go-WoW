package render

import (
	"fmt"
	"strings"
)

type debugPanelData struct {
	width, height   int
	fps, frameMS    float64
	uiRenderMS      float64
	modelLoadMS     float64
	gpuVendor       string
	gpuRenderer     string
	gpuVersion      string
	dataPath        string
	scenePath       string
	connection      string
	authAddress     string
	model           glueModelStats
	sceneParts      int
	assetCache      int
	mpqArchives     int
	mpqCachedFiles  int
	mpqMissingFiles int
	audio           bool
	cursor          bool
	modelError      string
	terminalDebug   bool
}

func debugPanelLines(data debugPanelData) []string {
	dataSource := "not configured"
	if data.dataPath != "" {
		dataSource = "live MPQ " + shortenDebugPath(data.dataPath, 48)
	}
	scene := "none"
	if data.scenePath != "" {
		scene = shortenDebugPath(data.scenePath, 48)
	}
	lines := []string{
		"G3N / OpenGL",
		"GPU " + shortenDebugPath(data.gpuRenderer, 52),
		"vendor " + shortenDebugPath(data.gpuVendor, 46),
		"GL " + shortenDebugPath(data.gpuVersion, 52),
		fmt.Sprintf("window %dx%d | %.0f fps | %.1f ms/frame", data.width, data.height, data.fps, data.frameMS),
		fmt.Sprintf("UI render %.1f ms | model load %.1f ms | terminal debug %s", data.uiRenderMS, data.modelLoadMS, debugEnabled(data.terminalDebug)),
		fmt.Sprintf("MPQ %d archives | %d cached files | %d misses", data.mpqArchives, data.mpqCachedFiles, data.mpqMissingFiles),
		fmt.Sprintf("UI cache %d decoded textures | assets %s", data.assetCache, dataSource),
		"scene " + scene,
		fmt.Sprintf("model %d parts | %d scene nodes", data.model.parts, data.sceneParts),
		fmt.Sprintf("geometry %d vertices | %d triangles", data.model.vertices, data.model.triangles),
		fmt.Sprintf("batches %d opaque | %d blended | %d textures", data.model.opaqueBatches, data.model.transparentBatches, data.model.textures),
		fmt.Sprintf("connection %s | auth %s", data.connection, shortenDebugPath(data.authAddress, 34)),
		fmt.Sprintf("audio %s | cursor %s", debugEnabled(data.audio), debugEnabled(data.cursor)),
		"live Glue UI and MPQ-backed assets",
		"F2 hide/show | drag title | click the arrow to collapse",
	}
	if data.modelError != "" {
		lines = append(lines, "model error "+shortenDebugPath(data.modelError, 76))
	}
	return lines
}

func debugEnabled(value bool) string {
	if value {
		return "ready"
	}
	return "off"
}

func shortenDebugPath(path string, limit int) string {
	path = strings.ReplaceAll(path, "\\", "/")
	if path == "" {
		return "none"
	}
	if len(path) <= limit {
		return path
	}
	if limit < 4 {
		return path[:limit]
	}
	return "..." + path[len(path)-limit+3:]
}
