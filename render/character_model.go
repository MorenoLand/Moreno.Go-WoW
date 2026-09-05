package render

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/texture"
	xdraw "golang.org/x/image/draw"
)

type characterSectionTextures struct {
	bodySkin        string
	skinExtra       string
	faceLower       string
	faceUpper       string
	facialHairLower string
	facialHairUpper string
	hair            string
	scalpLower      string
	scalpUpper      string
	cape            string
	underwear       []characterTextureRegionLayer
	regions         []characterTextureRegionLayer
}

type characterTextureRegionLayer struct {
	region int
	path   string
}

type charSectionsTable struct {
	data        []byte
	records     int
	fields      int
	stride      int
	stringStart int
	stringSize  int
}

type charSectionsFields struct {
	raceID      int
	sexID       int
	baseSection int
	variation   int
	color       int
	texture1    int
	texture2    int
	texture3    int
}

func loadGlueCharacterModel(loader *ui.Loader, character world.Character) (*core.Node, error) {
	modelPath := normalizeModelPath(worldCharacterModelPath(character))
	modelData, err := loader.ReadFile(modelPath)
	if err != nil {
		return nil, fmt.Errorf("read model %s: %w", modelPath, err)
	}
	model, err := parseM2(modelData)
	if err != nil {
		return nil, err
	}
	loadM2AnimationTracks(loader, modelPath, &model)
	skinData, err := loader.ReadFile(worldM2SkinPath(modelPath))
	if err != nil {
		return nil, fmt.Errorf("read skin %s: %w", worldM2SkinPath(modelPath), err)
	}
	skin, err := parseSkin(skinData)
	if err != nil {
		return nil, err
	}
	sections, sectionErr := resolveCharacterSections(loader, character)
	if sectionErr != nil {
		return nil, fmt.Errorf("resolve sections for %s: %w", modelPath, sectionErr)
	}
	activeGeosets, equipmentErr := resolveCharacterEquipment(loader, character, &sections, skin)
	if equipmentErr != nil {
		return nil, fmt.Errorf("resolve equipment for %s: %w", modelPath, equipmentErr)
	}
	overrides := make(map[int]string)
	preloaded := make(map[string]*texture.Texture2D)
	bodyPath := sections.bodySkin
	if bodyPath != "" && (sections.faceLower != "" || sections.faceUpper != "" || len(sections.underwear) > 0) {
		composite, compositeErr := composeCharacterSkin(loader, sections)
		if compositeErr != nil {
			return nil, fmt.Errorf("compose skin for %s: %w", modelPath, compositeErr)
		}
		bodyPath = fmt.Sprintf("__character_skin_%016x", character.GUID)
		preloaded[bodyPath] = composite
	}
	for index, textureType := range model.textureTypes {
		path := ""
		switch textureType {
		case 1:
			path = bodyPath
		case 2:
			path = sections.cape
		case 6:
			path = sections.hair
			if path == "" {
				path = fmt.Sprintf(`Character\%s\Hair00_00.blp`, characterRaceFolder(character.Race))
			}
		case 8:
			path = sections.skinExtra
			if path == "" && len(sections.underwear) > 0 {
				path = sections.underwear[0].path
			}
			if path == "" {
				path = bodyPath
			}
		}
		if path != "" {
			overrides[index] = path
		}
	}
	return buildGlueModel(loader, modelPath, model, skin, overrides, preloaded, activeGeosets)
}

func characterRaceFolder(race uint8) string {
	switch race {
	case 2:
		return "Orc"
	case 3:
		return "Dwarf"
	case 7:
		return "Gnome"
	case 4:
		return "NightElf"
	case 5:
		return "Scourge"
	case 6:
		return "Tauren"
	case 8:
		return "Troll"
	case 10:
		return "BloodElf"
	case 11:
		return "Draenei"
	default:
		return "Human"
	}
}

func resolveCharacterSections(loader *ui.Loader, character world.Character) (characterSectionTextures, error) {
	data, err := loader.ReadFile(`DBFilesClient\CharSections.dbc`)
	if err != nil {
		return characterSectionTextures{}, err
	}
	table, err := parseCharSectionsTable(data)
	if err != nil {
		return characterSectionTextures{}, err
	}
	if table.fields < 10 {
		return characterSectionTextures{}, fmt.Errorf("CharSections.dbc has %d fields", table.fields)
	}
	fields := detectCharSectionsFields(table)
	result := characterSectionTextures{}
	foundSkin := false
	foundHair := false
	foundUnderwear := false
	exactFace := false
	altFace := characterSectionTextures{}
	haveAltFace := false
	for record := 0; record < table.records; record++ {
		if table.value(record, fields.raceID) != uint32(character.Race) || table.value(record, fields.sexID) != uint32(character.Gender) {
			continue
		}
		section := table.value(record, fields.baseSection)
		variation := table.value(record, fields.variation)
		color := table.value(record, fields.color)
		texture1 := table.string(record, fields.texture1)
		texture2 := table.string(record, fields.texture2)
		texture3 := table.string(record, fields.texture3)
		if section == 0 && !foundSkin && color == uint32(character.Skin) {
			result.bodySkin = texture1
			result.skinExtra = texture2
			foundSkin = result.bodySkin != ""
		} else if section == 1 && !exactFace && variation == uint32(character.Face) && color == uint32(character.Skin) {
			result.faceLower = texture1
			result.faceUpper = texture2
			exactFace = result.faceLower != ""
		} else if section == 1 && !exactFace && !haveAltFace && (variation == uint32(character.Face) || color == uint32(character.Skin)) {
			if texture1 != "" {
				altFace.faceLower = texture1
				altFace.faceUpper = texture2
				haveAltFace = true
			}
		} else if section == 2 && variation == uint32(character.FacialHair) && color == uint32(character.HairColor) && result.facialHairLower == "" {
			result.facialHairLower = texture1
			result.facialHairUpper = texture2
		} else if section == 3 && !foundHair && variation == uint32(character.HairStyle) && color == uint32(character.HairColor) {
			result.hair = texture1
			result.scalpLower = texture2
			result.scalpUpper = texture3
			foundHair = result.hair != ""
		} else if section == 4 && !foundUnderwear && color == uint32(character.Skin) {
			for _, path := range []string{texture1, texture2, texture3} {
				region, ok := characterUnderwearRegion(path)
				if !ok {
					continue
				}
				if _, readErr := loader.ReadAsset(path); readErr == nil {
					result.underwear = append(result.underwear, characterTextureRegionLayer{region: region, path: path})
				}
			}
			foundUnderwear = len(result.underwear) > 0
		}
		if foundSkin && foundHair && exactFace && foundUnderwear {
			break
		}
	}
	if !exactFace && haveAltFace {
		result.faceLower = altFace.faceLower
		result.faceUpper = altFace.faceUpper
	}
	if result.bodySkin == "" {
		return result, fmt.Errorf("CharSections.dbc has no skin for race=%d sex=%d skin=%d", character.Race, character.Gender, character.Skin)
	}
	return result, nil
}

func parseCharSectionsTable(data []byte) (charSectionsTable, error) {
	if len(data) < 20 || string(data[:4]) != "WDBC" {
		return charSectionsTable{}, fmt.Errorf("invalid CharSections.dbc")
	}
	table := charSectionsTable{data: data, records: int(binary.LittleEndian.Uint32(data[4:8])), fields: int(binary.LittleEndian.Uint32(data[8:12])), stride: int(binary.LittleEndian.Uint32(data[12:16])), stringSize: int(binary.LittleEndian.Uint32(data[16:20]))}
	if table.records < 1 || table.fields < 3 || table.stride < table.fields*4 || table.stringSize < 1 || table.records > (len(data)-20-table.stringSize)/table.stride {
		return charSectionsTable{}, fmt.Errorf("invalid CharSections.dbc dimensions")
	}
	table.stringStart = 20 + table.records*table.stride
	if table.stringStart < 20 || table.stringStart+table.stringSize > len(data) {
		return charSectionsTable{}, fmt.Errorf("invalid CharSections.dbc string block")
	}
	return table, nil
}

func detectCharSectionsFields(table charSectionsTable) charSectionsFields {
	fields := charSectionsFields{raceID: 1, sexID: 2, baseSection: 3, variation: 4, color: 5, texture1: 6, texture2: 7, texture3: 8}
	large, small := 0, 0
	for record := 0; record < table.records && record < 20; record++ {
		if table.value(record, 4) > 50 {
			large++
		} else {
			small++
		}
	}
	if large > small {
		fields.texture1 = 4
		fields.texture2 = 5
		fields.texture3 = 6
		fields.variation = 8
		fields.color = 9
	}
	return fields
}

func (table charSectionsTable) value(record, field int) uint32 {
	base := 20 + record*table.stride + field*4
	return binary.LittleEndian.Uint32(table.data[base : base+4])
}

func (table charSectionsTable) string(record, field int) string {
	offset := int(table.value(record, field))
	if offset < 0 || offset >= table.stringSize {
		return ""
	}
	start := table.stringStart + offset
	end := start
	for end < table.stringStart+table.stringSize && table.data[end] != 0 {
		end++
	}
	return normalizeModelPath(string(table.data[start:end]))
}

func resolveCharacterEquipment(loader *ui.Loader, character world.Character, sections *characterSectionTextures, skin parsedSkin) (map[uint16]bool, error) {
	available := make(map[uint16]bool)
	for _, submesh := range skin.submeshes {
		available[submesh.submeshID] = true
	}
	active := make(map[uint16]bool)
	add := func(preferred uint16) {
		if chosen := resolveCharacterGeoset(preferred, available); chosen != 0 {
			active[chosen] = true
		}
	}
	replace := func(group, preferred uint16) {
		replaceCharacterGeosetGroup(active, group, preferred, available)
	}
	active[0] = true
	hairGeoset, showScalp := resolveCharacterHairGeoset(loader, character)
	add(hairGeoset)
	if showScalp {
		add(1)
	}
	add(1501)
	if character.Race == 4 {
		add(1701)
	}
	add(401)
	add(501)
	add(702)
	add(801)
	add(902)
	add(1301)
	add(2001)
	add(2002)
	facial100, facial200, facial300 := resolveCharacterFacialHair(loader, character)
	if facial100 != 0 {
		add(uint16(100 + facial100))
	}
	if facial200 != 0 {
		add(uint16(200 + facial200))
	}
	if facial300 != 0 {
		add(uint16(300 + facial300))
	}
	itemTable, err := loadCharacterDBC(loader, `DBFilesClient\ItemDisplayInfo.dbc`)
	if err != nil {
		return active, err
	}
	if itemTable.fields < 23 {
		return active, fmt.Errorf("ItemDisplayInfo.dbc has %d fields", itemTable.fields)
	}
	itemTextureBase := 15
	if itemTable.fields < 25 {
		itemTextureBase = 14
	}
	for _, equipment := range character.Equipment {
		if equipment.DisplayID == 0 {
			continue
		}
		record := characterDBCRecordByID(itemTable, equipment.DisplayID)
		if record < 0 {
			continue
		}
		if equipment.InventoryType == 4 || equipment.InventoryType == 5 {
			if value := itemTable.value(record, 7); value > 0 {
				replace(8, uint16(801+value))
			}
		} else if equipment.InventoryType == 20 {
			if value := itemTable.value(record, 7); value > 0 {
				replace(13, uint16(1301+value))
			}
		} else if equipment.InventoryType == 7 {
			if value := itemTable.value(record, 7); value > 0 {
				replace(13, uint16(1301+value))
			}
		} else if equipment.InventoryType == 8 {
			if value := itemTable.value(record, 7); value > 0 {
				replace(5, uint16(501+value))
			}
		} else if equipment.InventoryType == 10 {
			if value := itemTable.value(record, 7); value > 0 {
				replace(4, uint16(401+value))
			}
		} else if equipment.InventoryType == 16 {
			if value := itemTable.value(record, 7); value > 0 {
				add(uint16(1500 + value))
			}
			if sections.cape == "" {
				name := itemTable.string(record, 3)
				if name != "" {
					path := fmt.Sprintf(`Item\ObjectComponents\Cape\%s.blp`, name)
					if _, readErr := loader.ReadAsset(path); readErr == nil {
						sections.cape = path
					}
				}
			}
		}
		for region := 0; region < 8; region++ {
			field := itemTextureBase + region
			if field >= itemTable.fields {
				break
			}
			name := itemTable.string(record, field)
			if path := resolveItemTexturePath(loader, region, name, character.Gender == 1); path != "" {
				sections.regions = append(sections.regions, characterTextureRegionLayer{region: region, path: path})
			}
		}
	}
	for _, equipment := range character.Equipment {
		if equipment.DisplayID != 0 && equipment.InventoryType == 19 {
			add(1201)
		}
	}
	return active, nil
}

func loadCharacterDBC(loader *ui.Loader, path string) (charSectionsTable, error) {
	data, err := loader.ReadFile(path)
	if err != nil {
		return charSectionsTable{}, err
	}
	return parseCharSectionsTable(data)
}

func characterDBCRecordByID(table charSectionsTable, id uint32) int {
	for record := 0; record < table.records; record++ {
		if table.value(record, 0) == id {
			return record
		}
	}
	return -1
}

func resolveCharacterGeoset(preferred uint16, available map[uint16]bool) uint16 {
	if len(available) == 0 {
		return preferred
	}
	if available[preferred] {
		return preferred
	}
	if preferred%100 <= 1 {
		return 0
	}
	group := preferred / 100
	var lowest uint16
	for id := range available {
		if id/100 == group && (lowest == 0 || id < lowest) {
			lowest = id
		}
	}
	return lowest
}

func replaceCharacterGeosetGroup(active map[uint16]bool, group, preferred uint16, available map[uint16]bool) {
	for id := range active {
		if id/100 == group {
			delete(active, id)
		}
	}
	if chosen := resolveCharacterGeoset(preferred, available); chosen != 0 {
		active[chosen] = true
	}
}

func resolveCharacterHairGeoset(loader *ui.Loader, character world.Character) (uint16, bool) {
	table, err := loadCharacterDBC(loader, `DBFilesClient\CharHairGeosets.dbc`)
	if err != nil {
		return uint16(character.HairStyle + 1), false
	}
	for record := 0; record < table.records; record++ {
		if table.value(record, 1) == uint32(character.Race) && table.value(record, 2) == uint32(character.Gender) && table.value(record, 3) == uint32(character.HairStyle) {
			return uint16(table.value(record, 4)), table.fields > 5 && table.value(record, 5) != 0
		}
	}
	return uint16(character.HairStyle + 1), false
}

func resolveCharacterFacialHair(loader *ui.Loader, character world.Character) (uint32, uint32, uint32) {
	table, err := loadCharacterDBC(loader, `DBFilesClient\CharacterFacialHairStyles.dbc`)
	if err != nil {
		return 0, 0, 0
	}
	geoset100, geoset200, geoset300 := 3, 5, 4
	if table.fields >= 9 {
		geoset100, geoset200, geoset300 = 6, 8, 7
	}
	for record := 0; record < table.records; record++ {
		if table.value(record, 0) == uint32(character.Race) && table.value(record, 1) == uint32(character.Gender) && table.value(record, 2) == uint32(character.FacialHair) {
			return table.value(record, geoset100), table.value(record, geoset200), table.value(record, geoset300)
		}
	}
	return 0, 0, 0
}

func resolveItemTexturePath(loader *ui.Loader, region int, name string, female bool) string {
	if name == "" || region < 0 || region >= 8 {
		return ""
	}
	directories := []string{"ArmUpperTexture", "ArmLowerTexture", "HandTexture", "TorsoUpperTexture", "TorsoLowerTexture", "LegUpperTexture", "LegLowerTexture", "FootTexture"}
	base := fmt.Sprintf(`Item\TextureComponents\%s\%s`, directories[region], name)
	suffixes := []string{"_F.blp", "_U.blp", ".blp"}
	if !female {
		suffixes[0] = "_M.blp"
	}
	for _, suffix := range suffixes {
		path := base + suffix
		if _, err := loader.ReadAsset(path); err == nil {
			return path
		}
	}
	return ""
}

func composeCharacterSkin(loader *ui.Loader, sections characterSectionTextures) (*texture.Texture2D, error) {
	base, err := loadCharacterImage(loader, sections.bodySkin)
	if err != nil {
		return nil, err
	}
	composite := imageToRGBA(base)
	for _, layer := range []string{sections.faceUpper, sections.faceLower, sections.facialHairLower, sections.facialHairUpper, sections.scalpLower, sections.scalpUpper} {
		if layer == "" {
			continue
		}
		overlay, readErr := loadCharacterImage(loader, layer)
		if readErr != nil {
			continue
		}
		region, ok := characterTextureRegion(layer, composite.Bounds())
		if !ok {
			continue
		}
		drawScaledCharacterLayer(composite, region, overlay)
	}
	for _, layer := range sections.underwear {
		overlay, readErr := loadCharacterImage(loader, layer.path)
		if readErr != nil {
			continue
		}
		region, ok := characterRegionRectangle(layer.region, composite.Bounds())
		if ok {
			drawScaledCharacterLayer(composite, region, overlay)
		}
	}
	for _, layer := range sections.regions {
		overlay, readErr := loadCharacterImage(loader, layer.path)
		if readErr != nil {
			continue
		}
		region, ok := characterRegionRectangle(layer.region, composite.Bounds())
		if ok {
			drawScaledCharacterLayer(composite, region, overlay)
		}
	}
	return texture.NewTexture2DFromRGBA(composite), nil
}

func loadCharacterImage(loader *ui.Loader, path string) (image.Image, error) {
	data, err := loader.ReadAsset(path)
	if err != nil {
		return nil, err
	}
	img, err := ui.DecodeBLP(data)
	if err != nil {
		return nil, err
	}
	return img, nil
}

func imageToRGBA(source image.Image) *image.RGBA {
	bounds := source.Bounds()
	result := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(result, result.Bounds(), source, bounds.Min, draw.Src)
	return result
}

func characterTextureRegion(path string, bounds image.Rectangle) (image.Rectangle, bool) {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, "faceupper"), strings.Contains(lower, "facialupper"), strings.Contains(lower, "scalpupper"):
		return image.Rect(0, bounds.Dy()*320/512, bounds.Dx()*256/512, bounds.Dy()*384/512), true
	case strings.Contains(lower, "facelower"), strings.Contains(lower, "faciallower"), strings.Contains(lower, "scalplower"):
		return image.Rect(0, bounds.Dy()*384/512, bounds.Dx()*256/512, bounds.Dy()), true
	default:
		return image.Rectangle{}, false
	}
}

func characterUnderwearRegion(path string) (int, bool) {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, "nakedpelvis"):
		return 5, true
	case strings.Contains(lower, "nakedtorso"):
		return 3, true
	default:
		return 0, false
	}
}

func characterRegionRectangle(region int, bounds image.Rectangle) (image.Rectangle, bool) {
	regionCoords := [8][4]float64{{0, 0, 256, 128}, {0, 128, 256, 128}, {0, 256, 256, 64}, {256, 0, 256, 128}, {256, 128, 256, 64}, {256, 192, 256, 128}, {256, 320, 256, 128}, {256, 448, 256, 64}}
	if region < 0 || region >= len(regionCoords) || bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return image.Rectangle{}, false
	}
	scaleX := float64(bounds.Dx()) / 512
	scaleY := float64(bounds.Dy()) / 512
	coordinates := regionCoords[region]
	return image.Rect(int(coordinates[0]*scaleX), int(coordinates[1]*scaleY), int((coordinates[0]+coordinates[2])*scaleX), int((coordinates[1]+coordinates[3])*scaleY)), true
}

func drawScaledCharacterLayer(dst *image.RGBA, region image.Rectangle, source image.Image) {
	if region.Empty() {
		return
	}
	xdraw.NearestNeighbor.Scale(dst, region, source, source.Bounds(), xdraw.Over, nil)
}
