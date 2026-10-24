package render

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/math32"
)

type worldCreatureDefinition struct {
	path         string
	variations   [3]string
	bake         string
	scale        float32
	displayScale float32
	modelScale   float32
	extra        worldCreatureExtra
	// geoHeight is the model data's geobox height (CreatureModelData fields 19 and 22), before scale.
	geoHeight float32
}

type worldCreatureTables struct {
	displays charSectionsTable
	models   charSectionsTable
	extra    charSectionsTable
	loaded   bool
	cache    map[uint32]worldCreatureDefinition
	// gameObjects is GameObjectDisplayInfo.dbc, read when first needed.
	gameObjects       charSectionsTable
	gameObjectsLoaded bool
}

// gameObjectDisplayField is GAMEOBJECT_DISPLAYID.
const gameObjectDisplayField uint16 = 0x08

// gameObjectModel is the M2 a game object display uses (field 1 of
// GameObjectDisplayInfo.dbc); objects drawn as map objects have none here.
func (tables *worldCreatureTables) gameObjectModel(loader *ui.Loader, display uint32) string {
	if !tables.gameObjectsLoaded {
		tables.gameObjectsLoaded = true
		tables.gameObjects, _ = loadCharacterDBC(loader, `DBFilesClient\GameObjectDisplayInfo.dbc`)
	}
	record := characterDBCRecordByID(tables.gameObjects, display)
	if record < 0 || tables.gameObjects.fields < 2 {
		return ""
	}
	path := normalizeModelPath(tables.gameObjects.string(record, 1))
	if !strings.HasSuffix(strings.ToLower(path), ".m2") {
		return ""
	}
	return path
}

// syncWorldGameObject places a game object (a brazier, a bank vault, a mailbox)
// with the model its display names.
func syncWorldGameObject(scene *core.Node, loader *ui.Loader, tables *worldCreatureTables, entity *worldEntity) error {
	display := entity.fields[gameObjectDisplayField]
	if display == 0 {
		return nil
	}
	if entity.node == nil && entity.attemptedID != display {
		if !worldBuildDeadline.IsZero() && time.Now().After(worldBuildDeadline) {
			return nil
		}
		entity.attemptedID = display
		path := tables.gameObjectModel(loader, display)
		if path == "" {
			return nil
		}
		model, err := buildWorldCreatureModel(loader, worldCreatureDefinition{path: path, scale: 1, displayScale: 1, modelScale: 1})
		if err != nil {
			return err
		}
		scale := float32(1)
		if bits, ok := entity.fields[world.ObjectScaleField]; ok {
			if value := math.Float32frombits(bits); value > 0 {
				scale = value
			}
		}
		entity.node = wrapWorldModel(model, entity.movement.Position, scale)
		entity.displayID, entity.modelPath = display, path
		scene.Add(entity.node)
	}
	if entity.node != nil {
		entity.node.SetPosition(entity.movement.Position.X, entity.movement.Position.Y, entity.movement.Position.Z)
		entity.node.SetRotation(0, 0, entity.movement.Position.Orientation)
	}
	return nil
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
	mount         *worldMountRig
	modelPath     string
	extra         worldCreatureExtra
	equipment     *worldEquipment
	animPending   float64
	geoHeight     float32
	light         worldLightState
	shadow        worldShadowState
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
	if tables.models.fields >= 23 {
		definition.geoHeight = tables.models.valueFloat(model, 22) - tables.models.valueFloat(model, 19)
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
		if info.modelScale > 0 && info.modelScale != 1 {
			// A model built for the character screens is scaled to fit and centred
			// on its box, with attachment 0 as its stand point. In the world it
			// is its true size with its own origin (the feet) at the position.
			model.SetScale(1, 1, 1)
		} else if info.hasStand {
			offsetX, offsetY, offsetZ = -info.standPosition.X, info.standPosition.Z, -info.standPosition.Y
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
	// A spline walked at the walk speed plays the walk animation.
	if move.Walking {
		entity.movement.MovementFlags |= world.MovementFlagWalking
	} else {
		entity.movement.MovementFlags &^= world.MovementFlagWalking
	}
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

// worldEntityGroundReach is how far from its path a walking unit is brought to the
// floor it walks on; further than that the server's height stands.
const worldEntityGroundReach = 2.0

func advanceWorldEntities(entities map[uint64]*worldEntity, elapsed float64, floor func(float32, float32, float32) (float32, bool)) {
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
		// A walking unit stays on the ground it walks over: between two points of its
		// path the straight line goes through stairs and slopes.
		if floor != nil && entity.movement.MovementFlags&(world.MovementFlagFlying|world.MovementFlagSwimming|world.MovementFlagCanFly|world.MovementFlagLevitating) == 0 {
			position := entity.movement.Position
			if z, ok := floor(position.X, position.Y, position.Z); ok && math.Abs(float64(z-position.Z)) <= worldEntityGroundReach {
				entity.movement.Position.Z = z
			}
		}
		if remaining >= 1 {
			// Arriving, the unit keeps facing the way it walked unless the server
			// names another way to face.
			walked := entity.movement.Position.Orientation
			entity.movement.Position = points[len(points)-1]
			entity.movement.Position.Orientation = walked
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
	if entity.objectType == world.ObjectTypeGameObject && entity.hasPosition {
		return syncWorldGameObject(scene, loader, tables, entity)
	}
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
		// A crowd arrives in one batch; its models are built over several frames.
		if !worldBuildDeadline.IsZero() && time.Now().After(worldBuildDeadline) {
			return nil
		}
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
		entity.modelPath, entity.extra, entity.geoHeight = definition.path, definition.extra, definition.geoHeight
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
		syncWorldPlayerMount(scene, loader, tables, &entity.mount, entity.fields)
		syncWorldEquipment(loader, entity.node, &entity.equipment, entity.modelPath, entity, entity.extra)
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
			entity.mount.dispose(scene)
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

// worldEntitySkinSkipped reports whether a unit's vertices need no new pose this
// frame: it is outside the view, or far enough that a slower pose is not seen.
// Its bones still advance, so what attaches to them stays in place.
func worldEntitySkinSkipped(node *core.Node, info glueModelInfo, frustum *math32.Frustum, camera math32.Vector3, frame uint32, guid uint64) (skipped, idle bool) {
	position := node.Position()
	scale := node.Scale().Y
	height := info.modelTop * scale
	if height < 1.5 {
		height = 1.5
	}
	// The sphere holds the model with room for a mount or a weapon.
	centre := math32.NewVector3(position.X, position.Y, position.Z+height/2)
	sphere := math32.NewSphere(centre, height/2*1.5+3)
	if !frustum.IntersectsSphere(sphere) {
		return true, true
	}
	dx, dy, dz := position.X-camera.X, position.Y-camera.Y, position.Z-camera.Z
	distance := dx*dx + dy*dy + dz*dz
	interval := uint32(1)
	switch {
	case distance > 60*60:
		interval = 4
	case distance > 30*30:
		interval = 3
	case distance > 15*15:
		interval = 2
	}
	idle = distance > 60*60
	if (frame+uint32(guid))%interval != 0 {
		return true, idle
	}
	// A crowd is posed a number at a time; the rest keep their last pose.
	if worldSkinBudget <= 0 {
		return true, idle
	}
	worldSkinBudget--
	return false, idle
}

// worldSkinBudget is how many units may be re-posed this frame.
var worldSkinBudget int

// worldBuildDeadline, when set, is when this frame stops building entity models.
var worldBuildDeadline time.Time

// syncPendingWorldEntities builds the models of units that have arrived and have
// none yet, nearest first, until the frame's build time is spent.
func syncPendingWorldEntities(scene *core.Node, loader *ui.Loader, tables *worldCreatureTables, entities map[uint64]*worldEntity, ownGUID uint64, floor func(float32, float32, float32) (float32, bool), near [3]float32) {
	var pending []*worldEntity
	for _, entity := range entities {
		if entity.node == nil && entity.hasPosition && entity.guid != ownGUID {
			pending = append(pending, entity)
		}
	}
	if len(pending) == 0 {
		return
	}
	distance := func(entity *worldEntity) float32 {
		dx, dy := entity.movement.Position.X-near[0], entity.movement.Position.Y-near[1]
		return dx*dx + dy*dy
	}
	sort.Slice(pending, func(i, j int) bool { return distance(pending[i]) < distance(pending[j]) })
	for _, entity := range pending {
		if time.Now().After(worldBuildDeadline) {
			return
		}
		_ = syncWorldEntity(scene, loader, tables, entity, ownGUID, floor)
	}
}
