package render

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/math32"
)

// Attachment ids of the character models the equipment sits on.
const (
	attachShield        = 0
	attachHandRight     = 1
	attachHandLeft      = 2
	attachShoulderRight = 5
	attachShoulderLeft  = 6
	attachHelm          = 11
)

// Update fields the equipment of a unit is read from.
const (
	fieldUnitBytes0          = 23  // UNIT_FIELD_BYTES_0: race, class, gender, power type
	fieldUnitVirtualItem     = 56  // UNIT_VIRTUAL_ITEM_SLOT_ID, three item entries
	fieldUnitBytes2          = 122 // UNIT_FIELD_BYTES_2: sheath state in the first byte
	fieldPlayerVisibleItem1  = 283 // PLAYER_VISIBLE_ITEM_1_ENTRYID, two fields per slot
	visibleItemSlotsReadable = 19
)

// worldItemCatalog is Item.dbc as the world loader read it (entry to display id
// and inventory type); the equipment of units the server describes only by item
// entry is resolved through it.
var worldItemCatalog map[uint32]ui.ItemLocal

// raceModelAbbreviation is the race part of the head models' file names.
var raceModelAbbreviation = map[uint8]string{1: "Hu", 2: "Or", 3: "Dw", 4: "Ni", 5: "Sc", 6: "Ta", 7: "Gn", 8: "Tr", 9: "Go", 10: "Be", 11: "Dr"}

// equipSource is one piece of equipment a unit wears: the ItemDisplayInfo row
// and the inventory type that says where it goes.
type equipSource struct {
	displayID     uint32
	inventoryType uint32
	offHand       bool
}

// equipPiece is a model attached to an attachment point of its wearer.
type equipPiece struct {
	node     *core.Node
	bone     uint16
	position [3]float32
}

// worldEquipment is the set of equipment models a unit wears.
type worldEquipment struct {
	key    string
	pieces []equipPiece
}

var (
	equipTableMu       sync.Mutex
	equipDisplayTable  charSectionsTable
	equipDisplayLoaded bool
	attachmentCache    = map[string][]m2Attachment{}
)

func equipmentDisplayTable(loader *ui.Loader) (charSectionsTable, bool) {
	equipTableMu.Lock()
	defer equipTableMu.Unlock()
	if !equipDisplayLoaded {
		equipDisplayTable, _ = loadCharacterDBC(loader, `DBFilesClient\ItemDisplayInfo.dbc`)
		equipDisplayLoaded = true
	}
	return equipDisplayTable, equipDisplayTable.records > 0 && equipDisplayTable.fields >= 5
}

// cachedModelAttachments are the attachment points of a model file already
// read, and whether it has been.
func cachedModelAttachments(path string) ([]m2Attachment, bool) {
	equipTableMu.Lock()
	defer equipTableMu.Unlock()
	cached, ok := attachmentCache[path]
	return cached, ok
}

// modelAttachments are the attachment points of a model file.
func modelAttachments(loader *ui.Loader, path string) []m2Attachment {
	equipTableMu.Lock()
	cached, ok := attachmentCache[path]
	equipTableMu.Unlock()
	if ok {
		return cached
	}
	var attachments []m2Attachment
	if data, err := loader.ReadFile(path); err == nil {
		if parsed, parseErr := parseM2(data); parseErr == nil {
			attachments = parsed.attachments
		}
	}
	equipTableMu.Lock()
	attachmentCache[path] = attachments
	equipTableMu.Unlock()
	return attachments
}

func equipmentKey(sources []equipSource, race, gender uint8) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%d/%d", race, gender)
	for _, source := range sources {
		fmt.Fprintf(&builder, "|%d:%d:%t", source.displayID, source.inventoryType, source.offHand)
	}
	return builder.String()
}

func itemModelName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		name = name[:dot]
	}
	return name
}

type equipModelData struct {
	model parsedM2
	skin  parsedSkin
}

var equipModelCache = map[string]*equipModelData{}

// loadEquipmentModel builds one item model from Item\ObjectComponents.
func loadEquipmentModel(loader *ui.Loader, folder, name, textureName string) (*core.Node, error) {
	base := itemModelName(name)
	if base == "" {
		return nil, fmt.Errorf("no model name")
	}
	path := `Item\ObjectComponents\` + folder + `\` + base + `.m2`
	equipTableMu.Lock()
	data := equipModelCache[path]
	equipTableMu.Unlock()
	if data == nil {
		raw, err := loader.ReadFile(path)
		if err != nil {
			return nil, err
		}
		model, err := parseM2(raw)
		if err != nil {
			return nil, err
		}
		loadM2AnimationTracks(loader, path, &model)
		skinData, err := loader.ReadFile(worldM2SkinPath(path))
		if err != nil {
			return nil, err
		}
		skin, err := parseSkin(skinData)
		if err != nil {
			return nil, err
		}
		data = &equipModelData{model: model, skin: skin}
		equipTableMu.Lock()
		equipModelCache[path] = data
		equipTableMu.Unlock()
	}
	overrides := make(map[int]string)
	if texture := itemModelName(textureName); texture != "" {
		for index, existing := range data.model.textures {
			if existing == "" {
				overrides[index] = `Item\ObjectComponents\` + folder + `\` + texture + `.blp`
			}
		}
	}
	node, err := buildWorldUnitModel(loader, path, data.model, data.skin, overrides, nil, nil)
	if err != nil {
		return nil, err
	}
	node.SetRotation(float32(math.Pi/2), 0, 0)
	holder := core.NewNode()
	holder.Add(node)
	return holder, nil
}

// buildWorldEquipment attaches the models of the sources to the attachment
// points of the model at ownerPath, and returns them parented to parent.
func buildWorldEquipment(loader *ui.Loader, parent *core.Node, ownerPath string, race, gender uint8, sources []equipSource) *worldEquipment {
	equipment := &worldEquipment{key: equipmentKey(sources, race, gender)}
	table, ok := equipmentDisplayTable(loader)
	if !ok {
		return equipment
	}
	attachments := modelAttachments(loader, ownerPath)
	attach := func(id uint32, holder *core.Node) {
		for _, attachment := range attachments {
			if attachment.id == id {
				equipment.pieces = append(equipment.pieces, equipPiece{node: holder, bone: attachment.bone, position: attachment.position})
				parent.Add(holder)
				return
			}
		}
		holder.Dispose()
	}
	for _, source := range sources {
		record := characterDBCRecordByID(table, source.displayID)
		if record < 0 {
			continue
		}
		models := [2]string{table.string(record, 1), table.string(record, 2)}
		textures := [2]string{table.string(record, 3), table.string(record, 4)}
		switch source.inventoryType {
		case 1: // head: the model is made for each race and sex
			suffix := raceModelAbbreviation[race]
			sex := "M"
			if gender == 1 {
				sex = "F"
			}
			name := itemModelName(models[0])
			if name == "" || suffix == "" {
				continue
			}
			holder, err := loadEquipmentModel(loader, "Head", name+"_"+suffix+sex, textures[0])
			if err != nil {
				continue
			}
			attach(attachHelm, holder)
		case 3: // shoulders: a left and a right model
			for side := 0; side < 2; side++ {
				if models[side] == "" {
					continue
				}
				id := uint32(attachShoulderLeft)
				if side == 1 {
					id = attachShoulderRight
				}
				if strings.HasPrefix(strings.ToLower(models[side]), "rshoulder") {
					id = attachShoulderRight
				} else if strings.HasPrefix(strings.ToLower(models[side]), "lshoulder") {
					id = attachShoulderLeft
				}
				holder, err := loadEquipmentModel(loader, "Shoulder", models[side], textures[side])
				if err != nil {
					continue
				}
				attach(id, holder)
			}
		case 13, 17, 21, 22, 23, 14:
			folder, id := "Weapon", uint32(attachHandRight)
			switch source.inventoryType {
			case 14:
				folder, id = "Shield", attachShield
			case 22, 23:
				id = attachHandLeft
			case 13:
				if source.offHand {
					id = attachHandLeft
				}
			}
			holder, err := loadEquipmentModel(loader, folder, models[0], textures[0])
			if err != nil {
				continue
			}
			attach(id, holder)
		}
	}
	return equipment
}

// update poses the equipment on its wearer's current bone matrices. offset is
// where the wearer's model sits inside the node the equipment is parented to.
func (equipment *worldEquipment) update(info glueModelInfo, offset math32.Vector3) {
	if equipment == nil || info.animation == nil {
		return
	}
	for _, piece := range equipment.pieces {
		if int(piece.bone) >= len(info.animation.boneMatrices) {
			continue
		}
		matrix := info.animation.boneMatrices[piece.bone].point
		position := [3]float32{}
		for row := 0; row < 3; row++ {
			position[row] = matrix[row*4]*piece.position[0] + matrix[row*4+1]*piece.position[1] + matrix[row*4+2]*piece.position[2] + matrix[row*4+3]
		}
		piece.node.SetPosition(position[0]+offset.X, position[1]+offset.Y, position[2]+offset.Z)
		piece.node.SetRotationQuat(boneRotation(matrix))
	}
}

// boneRotation is the rotation of a bone matrix with any scale taken out.
func boneRotation(matrix [12]float32) *math32.Quaternion {
	var columns [3][3]float32
	for column := 0; column < 3; column++ {
		length := float32(math.Sqrt(float64(matrix[column]*matrix[column] + matrix[4+column]*matrix[4+column] + matrix[8+column]*matrix[8+column])))
		if length == 0 {
			length = 1
		}
		for row := 0; row < 3; row++ {
			columns[column][row] = matrix[row*4+column] / length
		}
	}
	rotation := math32.NewMatrix4()
	rotation.Set(
		columns[0][0], columns[1][0], columns[2][0], 0,
		columns[0][1], columns[1][1], columns[2][1], 0,
		columns[0][2], columns[1][2], columns[2][2], 0,
		0, 0, 0, 1)
	quaternion := math32.NewQuaternion(0, 0, 0, 1)
	quaternion.SetFromRotationMatrix(rotation)
	return quaternion
}

// unitEquipmentSources reads what a unit wears from its update fields and, for
// the humanoid models that have one, its CreatureDisplayInfoExtra row.
func unitEquipmentSources(entity *worldEntity, extra worldCreatureExtra) (sources []equipSource, race, gender uint8) {
	fields := entity.fields
	bytes0 := fields[fieldUnitBytes0]
	race, gender = uint8(bytes0), uint8(bytes0>>16)
	if extra.ok {
		race, gender = extra.race, extra.gender
		for index, displayID := range extra.items {
			if displayID != 0 && (npcExtraItemSlots[index] == 1 || npcExtraItemSlots[index] == 3) {
				sources = append(sources, equipSource{displayID: displayID, inventoryType: uint32(npcExtraItemSlots[index])})
			}
		}
	}
	sheathed := uint8(fields[fieldUnitBytes2]) == 0
	add := func(entry uint32, offHand bool) {
		if entry == 0 || worldItemCatalog == nil {
			return
		}
		item, ok := worldItemCatalog[entry]
		if !ok || item.DisplayID == 0 {
			return
		}
		sources = append(sources, equipSource{displayID: item.DisplayID, inventoryType: item.InventoryType, offHand: offHand})
	}
	if entity.objectType == world.ObjectTypePlayer {
		// Players: the visible item slots (head 0, shoulders 2, hands 15 and 16).
		for _, slot := range []int{0, 2} {
			add(fields[uint16(fieldPlayerVisibleItem1+slot*2)], false)
		}
		if !sheathed {
			add(fields[uint16(fieldPlayerVisibleItem1+15*2)], false)
			add(fields[uint16(fieldPlayerVisibleItem1+16*2)], true)
		}
		return sources, race, gender
	}
	if !sheathed {
		add(fields[fieldUnitVirtualItem], false)
		add(fields[fieldUnitVirtualItem+1], true)
	}
	return sources, race, gender
}

// characterEquipmentSources is the equipment of a character from the
// character list: its display ids come with the inventory types.
func characterEquipmentSources(character world.Character, drawn bool) []equipSource {
	var sources []equipSource
	for slot, piece := range character.Equipment {
		if piece.DisplayID == 0 {
			continue
		}
		switch slot {
		case 0, 2:
			sources = append(sources, equipSource{displayID: piece.DisplayID, inventoryType: uint32(piece.InventoryType)})
		case 15, 16:
			if drawn {
				sources = append(sources, equipSource{displayID: piece.DisplayID, inventoryType: uint32(piece.InventoryType), offHand: slot == 16})
			}
		}
	}
	return sources
}

// remove takes the equipment off its wearer.
func (equipment *worldEquipment) remove(parent *core.Node) {
	if equipment == nil {
		return
	}
	for _, piece := range equipment.pieces {
		parent.Remove(piece.node)
	}
	equipment.pieces = nil
}

// syncWorldEquipment makes the equipment attached to node match what the unit
// wears now; it does nothing while that is unchanged.
func syncWorldEquipment(loader *ui.Loader, node *core.Node, current **worldEquipment, ownerPath string, entity *worldEntity, extra worldCreatureExtra) {
	if node == nil || ownerPath == "" {
		return
	}
	sources, race, gender := unitEquipmentSources(entity, extra)
	key := equipmentKey(sources, race, gender)
	if *current != nil && (*current).key == key {
		return
	}
	(*current).remove(node)
	*current = buildWorldEquipment(loader, node, ownerPath, race, gender, sources)
}

// poseWorldEquipment poses a unit's equipment on its model.
func poseWorldEquipment(node *core.Node, equipment *worldEquipment) {
	if node == nil || equipment == nil || len(equipment.pieces) == 0 {
		return
	}
	info, ok := node.UserData().(glueModelInfo)
	if !ok {
		return
	}
	children := node.Children()
	if len(children) == 0 {
		return
	}
	equipment.update(info, children[0].GetNode().Position())
}
