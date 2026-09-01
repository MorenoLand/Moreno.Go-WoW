package render

import (
	"fmt"
	"math"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/core"
)

type worldCreatureDefinition struct {
	path       string
	variations [3]string
	bake       string
	scale      float32
}

type worldCreatureTables struct {
	displays charSectionsTable
	models   charSectionsTable
	extra    charSectionsTable
	loaded   bool
	cache    map[uint32]worldCreatureDefinition
}

type worldEntity struct {
	guid        uint64
	objectType  world.ObjectType
	fields      map[uint16]uint32
	movement    world.UpdateMovement
	hasPosition bool
	node        *core.Node
	displayID   uint32
	attemptedID uint32
}

func (tables *worldCreatureTables) definition(loader *ui.Loader, displayID uint32) (worldCreatureDefinition, error) {
	if tables.cache == nil {
		tables.cache = make(map[uint32]worldCreatureDefinition)
	}
	if definition, ok := tables.cache[displayID]; ok {
		return definition, nil
	}
	if !tables.loaded {
		var err error
		if tables.displays, err = loadCharacterDBC(loader, `DBFilesClient\CreatureDisplayInfo.dbc`); err != nil {
			return worldCreatureDefinition{}, err
		}
		if tables.models, err = loadCharacterDBC(loader, `DBFilesClient\CreatureModelData.dbc`); err != nil {
			return worldCreatureDefinition{}, err
		}
		tables.extra, _ = loadCharacterDBC(loader, `DBFilesClient\CreatureDisplayInfoExtra.dbc`)
		tables.loaded = true
	}
	display := characterDBCRecordByID(tables.displays, displayID)
	if display < 0 || tables.displays.fields < 9 {
		return worldCreatureDefinition{}, fmt.Errorf("CreatureDisplayInfo.dbc has no display %d", displayID)
	}
	modelID := tables.displays.value(display, 1)
	model := characterDBCRecordByID(tables.models, modelID)
	if model < 0 || tables.models.fields < 3 {
		return worldCreatureDefinition{}, fmt.Errorf("CreatureModelData.dbc has no model %d for display %d", modelID, displayID)
	}
	definition := worldCreatureDefinition{path: normalizeModelPath(tables.models.string(model, 2)), scale: tables.displays.valueFloat(display, 4) * tables.models.valueFloat(model, 4)}
	for index := range definition.variations {
		definition.variations[index] = tables.displays.string(display, 6+index)
	}
	if definition.scale <= 0 {
		definition.scale = 1
	}
	if tables.extra.fields >= 21 {
		extraID := tables.displays.value(display, 3)
		if extra := characterDBCRecordByID(tables.extra, extraID); extra >= 0 {
			definition.bake = tables.extra.string(extra, 20)
		}
	}
	if definition.path == "" {
		return worldCreatureDefinition{}, fmt.Errorf("display %d has no model path", displayID)
	}
	tables.cache[displayID] = definition
	return definition, nil
}

func (table charSectionsTable) valueFloat(record, field int) float32 {
	return math.Float32frombits(table.value(record, field))
}

func worldCreatureVariationPath(modelPath, variation string) string {
	variation = normalizeModelPath(variation)
	if variation == "" {
		return ""
	}
	if !strings.Contains(strings.ToLower(variation), ".blp") {
		variation += ".blp"
	}
	if strings.Contains(variation, `\`) {
		return variation
	}
	if slash := strings.LastIndex(modelPath, `\`); slash >= 0 {
		return modelPath[:slash+1] + variation
	}
	return variation
}

func buildWorldCreatureModel(loader *ui.Loader, definition worldCreatureDefinition) (*core.Node, error) {
	modelData, err := loader.ReadFile(definition.path)
	if err != nil {
		return nil, fmt.Errorf("read creature model %s: %w", definition.path, err)
	}
	model, err := parseM2(modelData)
	if err != nil {
		return nil, err
	}
	loadM2AnimationTracks(loader, definition.path, &model)
	skinData, err := loader.ReadFile(worldM2SkinPath(definition.path))
	if err != nil {
		return nil, fmt.Errorf("read creature skin %s: %w", worldM2SkinPath(definition.path), err)
	}
	skin, err := parseSkin(skinData)
	if err != nil {
		return nil, err
	}
	overrides := make(map[int]string)
	bakePath := ""
	if definition.bake != "" {
		bakePath = `Textures\BakedNpcTextures\` + definition.bake
		if !strings.Contains(strings.ToLower(bakePath), ".blp") {
			bakePath += ".blp"
		}
	}
	for index, textureType := range model.textureTypes {
		path := ""
		if textureType == 1 && bakePath != "" {
			path = bakePath
		} else {
			slot := -1
			switch textureType {
			case 1, 2, 11:
				slot = 0
			case 12:
				slot = 1
			case 13:
				slot = 2
			}
			if slot >= 0 {
				path = worldCreatureVariationPath(definition.path, definition.variations[slot])
			}
		}
		if path != "" {
			overrides[index] = path
		}
	}
	return buildGlueModel(loader, definition.path, model, skin, overrides, nil, nil)
}

func wrapWorldModel(model *core.Node, position world.WorldPosition, scale float32) *core.Node {
	root := core.NewNode()
	root.SetPosition(position.X, position.Y, position.Z)
	root.SetRotation(0, 0, position.Orientation)
	root.SetScale(scale, scale, scale)
	model.SetRotation(float32(1.5707963267948966), 0, 0)
	if info, ok := model.UserData().(glueModelInfo); ok {
		root.SetUserData(info)
		if info.hasStand {
			model.SetPosition(-info.standPosition.X, info.standPosition.Z, -info.standPosition.Y)
		}
	}
	root.Add(model)
	return root
}

func worldEntityMoving(entity *worldEntity) bool {
	flags := entity.movement.MovementFlags
	return flags&(world.MovementFlagForward|world.MovementFlagBackward|world.MovementFlagStrafeLeft|world.MovementFlagStrafeRight) != 0
}

func syncWorldEntity(scene *core.Node, loader *ui.Loader, tables *worldCreatureTables, entity *worldEntity, ownGUID uint64) error {
	if entity.guid == ownGUID || !entity.hasPosition || (entity.objectType != world.ObjectTypeUnit && entity.objectType != world.ObjectTypePlayer) {
		return nil
	}
	displayID := entity.fields[world.UnitDisplayIDField]
	if displayID == 0 {
		return nil
	}
	if entity.node == nil && entity.attemptedID != displayID {
		entity.attemptedID = displayID
		definition, err := tables.definition(loader, displayID)
		if err != nil {
			return err
		}
		model, err := buildWorldCreatureModel(loader, definition)
		if err != nil {
			return err
		}
		entity.node = wrapWorldModel(model, entity.movement.Position, definition.scale)
		entity.displayID = displayID
		scene.Add(entity.node)
	}
	if entity.node != nil {
		entity.node.SetPosition(entity.movement.Position.X, entity.movement.Position.Y, entity.movement.Position.Z)
		entity.node.SetRotation(0, 0, entity.movement.Position.Orientation)
		if info, ok := entity.node.UserData().(glueModelInfo); ok && info.animation != nil {
			motion := uint16(0)
			if worldEntityMoving(entity) {
				motion = 5
			}
			info.animation.SetMotion(motion)
		}
	}
	return nil
}

func applyWorldUpdateBlocks(scene *core.Node, loader *ui.Loader, tables *worldCreatureTables, entities map[uint64]*worldEntity, blocks []world.UpdateBlock, ownGUID uint64) (int, []error) {
	created := 0
	errors := make([]error, 0)
	remove := func(guid uint64) {
		if entity := entities[guid]; entity != nil {
			if entity.node != nil {
				scene.Remove(entity.node)
				entity.node.Dispose()
			}
			delete(entities, guid)
		}
	}
	for _, block := range blocks {
		switch block.Type {
		case world.UpdateCreate, world.UpdateCreate2:
			entity := entities[block.GUID]
			if entity == nil {
				entity = &worldEntity{guid: block.GUID, fields: make(map[uint16]uint32)}
				entities[block.GUID] = entity
			}
			entity.objectType = block.ObjectType
			for field, value := range block.Fields {
				entity.fields[field] = value
			}
			if block.Movement.HasPosition {
				entity.movement = block.Movement
				entity.hasPosition = true
			}
			if err := syncWorldEntity(scene, loader, tables, entity, ownGUID); err != nil {
				errors = append(errors, err)
			} else if entity.node != nil {
				created++
			}
		case world.UpdateValues:
			entity := entities[block.GUID]
			if entity == nil {
				continue
			}
			for field, value := range block.Fields {
				entity.fields[field] = value
			}
			if err := syncWorldEntity(scene, loader, tables, entity, ownGUID); err != nil {
				errors = append(errors, err)
			}
		case world.UpdateMovementBlock:
			entity := entities[block.GUID]
			if entity == nil {
				continue
			}
			if block.Movement.HasPosition {
				entity.movement = block.Movement
				entity.hasPosition = true
			}
			if err := syncWorldEntity(scene, loader, tables, entity, ownGUID); err != nil {
				errors = append(errors, err)
			}
		case world.UpdateOutOfRange:
			for _, guid := range block.GUIDs {
				remove(guid)
			}
		}
	}
	return created, errors
}
