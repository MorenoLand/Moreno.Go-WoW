package ui

import (
	"image/png"
	"os"
	"strings"
	"testing"
)

func TestTmpMinimapProbe(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if err := engine.LoadWorldUI(); err != nil {
		t.Fatal(err)
	}
	if data, readErr := engine.AssetLoader.ReadFile(`Interface\FrameXML\Minimap.xml`); readErr == nil {
		lines := strings.Split(string(data), "\n")
		for index, line := range lines {
			if index < 220 {
				t.Logf("minimap-xml %d %s", index+1, strings.TrimSpace(line))
			}
		}
		for index, line := range lines {
			if !strings.Contains(line, `name="MinimapBackdrop"`) {
				continue
			}
			for end := index; end < len(lines) && end < index+90; end++ {
				t.Logf("minimap-backdrop-xml %d %s", end+1, strings.TrimSpace(lines[end]))
			}
		}
	}
	if data, readErr := engine.AssetLoader.ReadFile(`Textures\Minimap\md5translate.trs`); readErr == nil {
		t.Logf("trs bytes=%d", len(data))
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(strings.ToLower(line), "azeroth\\map") {
				t.Logf("trs %s", strings.TrimSpace(line))
			}
		}
	} else {
		t.Logf("trs missing: %v", readErr)
	}
	for _, name := range []string{"MinimapCluster", "Minimap", "MinimapBackdrop", "MinimapBorder", "MinimapNorthTag"} {
		widget := engine.Rt.widgets[name]
		if widget == nil {
			t.Logf("widget %s missing", name)
			continue
		}
		t.Logf("widget %s kind=%s size=%gx%g shown=%t rect=%v parent=%s", name, widget.kind.objectType(), widget.width, widget.height, widget.shown, widget.renderRect, widgetParentForTmp(widget))
	}
	if list, listErr := engine.AssetLoader.ReadFile("(listfile)"); listErr == nil {
		for _, line := range strings.Split(string(list), "\n") {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "minimap") || strings.Contains(lower, "worldmap") || strings.Contains(lower, "elwynn") {
				t.Logf("list %s", strings.TrimSpace(line))
			}
		}
	}
	for _, path := range []string{
		`Interface\Minimap\Minimap.blp`,
		`Interface\Minimap\MinimapBorder.blp`,
		`Interface\Minimap\MinimapBackdrop.blp`,
		`Interface\Minimap\UI-Minimap-Border`,
		`World\Minimaps\Azeroth\map0_0.blp`,
		`World\Minimaps\Azeroth\map0_1.blp`,
		`World\Minimaps\Azeroth\map0_2.blp`,
		`World\Minimaps\Azeroth\Azeroth_32_32.blp`,
		`World\Minimaps\Azeroth\Azeroth_31_32.blp`,
		`Interface\WorldMap\Azeroth\Azeroth1.blp`,
		`Interface\WorldMap\Azeroth\Azeroth2.blp`,
		`Interface\WorldMap\Azeroth\Azeroth3.blp`,
		`Interface\WorldMap\Azeroth\Azeroth4.blp`,
	} {
		data, readErr := engine.AssetLoader.ReadAsset(path)
		if readErr != nil {
			t.Logf("missing %s: %v", path, readErr)
			continue
		}
		t.Logf("found %s bytes=%d", path, len(data))
		if path == `Interface\Minimap\UI-Minimap-Border` {
			image, decodeErr := DecodeBLP(data)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			file, createErr := os.Create(`..\bin\tmp-minimap-border.png`)
			if createErr != nil {
				t.Fatal(createErr)
			}
			if encodeErr := png.Encode(file, image); encodeErr != nil {
				file.Close()
				t.Fatal(encodeErr)
			}
			file.Close()
		}
		if strings.HasPrefix(path, `Interface\WorldMap\Azeroth\Azeroth`) {
			image, decodeErr := DecodeBLP(data)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			name := strings.TrimSuffix(strings.TrimPrefix(path, `Interface\WorldMap\Azeroth\`), `.blp`)
			file, createErr := os.Create(`..\bin\tmp-` + name + `.png`)
			if createErr != nil {
				t.Fatal(createErr)
			}
			if encodeErr := png.Encode(file, image); encodeErr != nil {
				file.Close()
				t.Fatal(encodeErr)
			}
			file.Close()
		}
	}
}
