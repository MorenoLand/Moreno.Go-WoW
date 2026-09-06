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
	path         string
	variations   [3]string
	bake         string
	scale        float32
	displayScale float32
	modelScale   float32
	extra        worldCreatureExtra
}

type worldCreatureTables struct {
	displays charSectionsTable
	models   charSectionsTable
	extra    charSectionsTable
	loaded   bool
	cache    map[uint32]worldCreatureDefinition
}

type worldEntity struct {
	guid          uint64
	objectType    world.ObjectType
	fields        map[uint16]uint32
	movement      world.UpdateMovement
	hasPosition   bool
	path          []world.WorldPosition
	pathLengths   []float64
	pathTotal     float64
	pathElapsed   float64
	pathDuration  float64
	arrivalFacing world.MoveFacing
	node          *core.Node
	displayID     uint32
	attemptedID   uint32
}

func (tables *worldCreatureTables) definition(loader *ui.Loader, displayID, nativeDisplayID uint32) (worldCreatureDefinition, error) {
	cacheID := displayID
	if cacheID == 0 {
		cacheID = nativeDisplayID
	}
	if tables.cache == nil {
		tables.cache = make(map[uint32]worldCreatureDefinition)
	}
	if definition, ok := tables.cache[cacheID]; ok {
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
	resolvedDisplayID := displayID
	display := characterDBCRecordByID(tables.displays, resolvedDisplayID)
	if display < 0 && nativeDisplayID != 0 {
		resolvedDisplayID = nativeDisplayID
		display = characterDBCRecordByID(tables.displays, resolvedDisplayID)
	}
	if display < 0 || tables.displays.fields < 9 {
		return worldCreatureDefinition{}, fmt.Errorf("CreatureDisplayInfo.dbc has no display %d", resolvedDisplayID)
	}
	modelID := tables.displays.value(display, 1)
	model := characterDBCRecordByID(tables.models, modelID)
	if model < 0 || tables.models.fields < 3 {
		return worldCreatureDefinition{}, fmt.Errorf("CreatureModelData.dbc has no model %d for display %d", modelID, resolvedDisplayID)
	}
	displayScale, modelScale := worldCreatureDisplayScales(tables, display, model)
	definition := worldCreatureDefinition{
		path:         normalizeModelPath(tables.models.string(model, 2)),
		scale:        displayScale * modelScale,
		displayScale: displayScale,
		modelScale:   modelScale,
	}
	for index := range definition.variations {
		definition.variations[index] = tables.displays.string(display, 6+index)
	}
	if definition.scale <= 0 {
		definition.scale = 1
	}
	definition.extra = loadWorldCreatureExtra(tables, display)
	if definition.extra.ok {
		definition.bake = definition.extra.bake
	}
	if definition.path == "" {
		return worldCreatureDefinition{}, fmt.Errorf("display %d has no model path", resolvedDisplayID)
	}
	tables.cache[cacheID] = definition
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
	return buildWorldCreatureModelFromParts(loader, definition.path, definition.variations, definition.bake, definition.extra)
}

func wrapWorldModel(model *core.Node, position world.WorldPosition, scale float32) *core.Node {
	root := core.NewNode()
	root.SetPosition(position.X, position.Y, position.Z)
	root.SetRotation(0, 0, position.Orientation)
	root.SetScale(scale, scale, scale)
	model.SetRotation(float32(1.5707963267948966), 0, 0)
	if info, ok := model.UserData().(glueModelInfo); ok {
		root.SetUserData(info)
		offsetX, offsetY, offsetZ := float32(0), float32(0), float32(0)
		if info.hasStand {
			offsetX, offsetY, offsetZ = -info.standPosition.X, info.standPosition.Z, -info.standPosition.Y
		} else if info.modelBottom < 0 {
			offsetZ = -info.modelBottom
		}
		model.SetPosition(offsetX, offsetY, offsetZ)
	}
	root.Add(model)
	return root
}

func worldEntityMoving(entity *worldEntity) bool {
	flags := entity.movement.MovementFlags
	return len(entity.path) > 0 || flags&(world.MovementFlagForward|world.MovementFlagBackward|world.MovementFlagStrafeLeft|world.MovementFlagStrafeRight) != 0
}

func worldEntityMotion(entity *worldEntity) uint16 {
	flags := entity.movement.MovementFlags
	if flags&world.MovementFlagSwimming != 0 {
		switch {
		case flags&world.MovementFlagBackward != 0:
			return 45
		case flags&world.MovementFlagStrafeLeft != 0:
			return 43
		case flags&world.MovementFlagStrafeRight != 0:
			return 44
		default:
			return 42
		}
	}
	if flags&world.MovementFlagFlying != 0 {
		return 135
	}
	if flags&world.MovementFlagBackward != 0 {
		return 13
	}
	if flags&world.MovementFlagStrafeLeft != 0 {
		return 11
	}
	if flags&world.MovementFlagStrafeRight != 0 {
		return 12
	}
	if flags&world.MovementFlagWalking != 0 {
		return 4
	}
	return 5
}

func applyWorldMonsterMove(entities map[uint64]*worldEntity, move world.MonsterMove) {
	entity := entities[move.GUID]
	if entity == nil {
		entity = &worldEntity{guid: move.GUID, objectType: world.ObjectTypeUnit, fields: make(map[uint16]uint32)}
		entities[move.GUID] = entity
	}
	start := move.From
	if entity.hasPosition {
		start.Orientation = entity.movement.Position.Orientation
		if len(entity.path) > 0 {
			start = entity.movement.Position
		}
	}
	entity.movement.Position = start
	entity.hasPosition = true
	entity.pathElapsed = 0
	entity.pathDuration = float64(move.Duration) / 1000
	entity.path = nil
	entity.pathLengths = nil
	entity.pathTotal = 0
	entity.arrivalFacing = move.Facing
	entity.movement.MovementFlags &^= world.MovementFlagForward | world.MovementFlagBackward | world.MovementFlagStrafeLeft | world.MovementFlagStrafeRight
	if move.Stopped || move.Duration == 0 {
		if facing, ok := worldEntityArrivalFacing(entities, entity, move.Facing); ok {
			entity.movement.Position.Orientation = facing
		}
		return
	}
	entity.path = make([]world.WorldPosition, 0, len(move.Path)+2)
	entity.path = append(entity.path, start)
	if len(move.Path) == 0 {
		entity.path = append(entity.path, move.To)
	} else {
		entity.path = append(entity.path, move.Path...)
	}
	entity.pathLengths = make([]float64, len(entity.path)-1)
	for index := range entity.pathLengths {
		dx := float64(entity.path[index+1].X - entity.path[index].X)
		dy := float64(entity.path[index+1].Y - entity.path[index].Y)
		dz := float64(entity.path[index+1].Z - entity.path[index].Z)
		entity.pathLengths[index] = math.Sqrt(dx*dx + dy*dy + dz*dz)
		entity.pathTotal += entity.pathLengths[index]
	}
	if entity.pathTotal <= 0 {
		entity.movement.Position = entity.path[len(entity.path)-1]
		if facing, ok := worldEntityArrivalFacing(entities, entity, move.Facing); ok {
			entity.movement.Position.Orientation = facing
		}
		entity.path = nil
		return
	}
	entity.movement.MovementFlags |= world.MovementFlagForward
}

func worldEntityArrivalFacing(entities map[uint64]*worldEntity, entity *worldEntity, facing world.MoveFacing) (float32, bool) {
	switch facing.Kind {
	case 2:
		return float32(math.Atan2(float64(facing.Y-entity.movement.Position.Y), float64(facing.X-entity.movement.Position.X))), true
	case 3:
		if target := entities[facing.TargetGUID]; target != nil && target.hasPosition {
			return float32(math.Atan2(float64(target.movement.Position.Y-entity.movement.Position.Y), float64(target.movement.Position.X-entity.movement.Position.X))), true
		}
	case 4:
		return facing.Angle, true
	}
	return 0, false
}

func advanceWorldEntities(entities map[uint64]*worldEntity, elapsed float64, _ func(float32, float32, float32) (float32, bool)) {
	if elapsed <= 0 {
		return
	}
	for _, entity := range entities {
		if len(entity.path) < 2 || entity.pathDuration <= 0 || entity.pathTotal <= 0 {
			continue
		}
		entity.pathElapsed += elapsed
		points := entity.path
		remaining := float32(math.Min(entity.pathElapsed/entity.pathDuration, 1))
		distance := float64(remaining) * entity.pathTotal
		segment := 0
		for segment < len(entity.pathLengths)-1 && distance > entity.pathLengths[segment] {
			distance -= entity.pathLengths[segment]
			segment++
		}
		fraction := float32(0)
		if entity.pathLengths[segment] > 0 {
			fraction = float32(distance / entity.pathLengths[segment])
		}
		from, to := points[segment], points[segment+1]
		entity.movement.Position = world.WorldPosition{X: from.X + (to.X-from.X)*fraction, Y: from.Y + (to.Y-from.Y)*fraction, Z: from.Z + (to.Z-from.Z)*fraction, Orientation: float32(math.Atan2(float64(to.Y-from.Y), float64(to.X-from.X)))}
		if remaining >= 1 {
			entity.movement.Position = points[len(points)-1]
			switch entity.arrivalFacing.Kind {
			case 2:
				entity.movement.Position.Orientation = float32(math.Atan2(float64(entity.arrivalFacing.Y-entity.movement.Position.Y), float64(entity.arrivalFacing.X-entity.movement.Position.X)))
			case 4:
				entity.movement.Position.Orientation = entity.arrivalFacing.Angle
			}
		}
		if remaining >= 1 {
			entity.path = nil
			entity.pathElapsed = 0
			entity.pathDuration = 0
			entity.movement.MovementFlags &^= world.MovementFlagForward | world.MovementFlagBackward | world.MovementFlagStrafeLeft | world.MovementFlagStrafeRight
		}
		if entity.node != nil {
			entity.node.SetPosition(entity.movement.Position.X, entity.movement.Position.Y, entity.movement.Position.Z)
			entity.node.SetRotation(0, 0, entity.movement.Position.Orientation)
		}
	}
}

func syncWorldEntity(scene *core.Node, loader *ui.Loader, tables *worldCreatureTables, entity *worldEntity, ownGUID uint64, _ func(float32, float32, float32) (float32, bool)) error {
	if entity.guid == ownGUID || !entity.hasPosition || (entity.objectType != world.ObjectTypeUnit && entity.objectType != world.ObjectTypePlayer) {
		return nil
	}
	displayID := entity.fields[world.UnitDisplayIDField]
	nativeDisplayID := entity.fields[world.UnitNativeDisplayIDField]
	modelID := displayID
	if modelID == 0 {
		modelID = nativeDisplayID
	}
	if modelID == 0 {
		return nil
	}
	if entity.node == nil && entity.attemptedID != modelID {
		entity.attemptedID = modelID
		definition, err := tables.definition(loader, displayID, nativeDisplayID)
		if err != nil {
			return err
		}
		model, err := buildWorldCreatureModel(loader, definition)
		if err != nil {
			return err
		}
		entity.node = wrapWorldModel(model, entity.movement.Position, worldEntityScale(entity, definition.displayScale, definition.modelScale))
		entity.displayID = modelID
		scene.Add(entity.node)
	}
	if entity.node != nil {
		entity.node.SetPosition(entity.movement.Position.X, entity.movement.Position.Y, entity.movement.Position.Z)
		entity.node.SetRotation(0, 0, entity.movement.Position.Orientation)
		if definition, defErr := tables.definition(loader, displayID, nativeDisplayID); defErr == nil {
			scale := worldEntityScale(entity, definition.displayScale, definition.modelScale)
			entity.node.SetScale(scale, scale, scale)
		}
		if info, ok := entity.node.UserData().(glueModelInfo); ok && info.animation != nil {
			motion := uint16(0)
			if worldEntityMoving(entity) {
				motion = worldEntityMotion(entity)
			}
			info.animation.SetMotion(motion)
		}
	}
	return nil
}

func applyWorldUpdateBlocks(scene *core.Node, loader *ui.Loader, tables *worldCreatureTables, entities map[uint64]*worldEntity, blocks []world.UpdateBlock, ownGUID uint64, floor func(float32, float32, float32) (float32, bool)) (int, []error) {
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
			if err := syncWorldEntity(scene, loader, tables, entity, ownGUID, floor); err != nil {
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
			if err := syncWorldEntity(scene, loader, tables, entity, ownGUID, floor); err != nil {
				errors = append(errors, err)
			}
		case world.UpdateMovementBlock:
			entity := entities[block.GUID]
			if entity == nil {
				continue
			}
			if block.Movement.HasPosition {
				if len(entity.path) == 0 {
					entity.movement = block.Movement
					entity.hasPosition = true
				} else {
					entity.movement.Flags = block.Movement.Flags
					entity.movement.MovementFlags = block.Movement.MovementFlags
					entity.movement.Speeds = block.Movement.Speeds
				}
			}
			if err := syncWorldEntity(scene, loader, tables, entity, ownGUID, floor); err != nil {
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
