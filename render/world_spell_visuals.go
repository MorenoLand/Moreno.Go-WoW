package render

import (
	"math"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/core"
)

// Spell effects: a spell names a SpellVisual (Spell.dbc), which names kits for
// the cast's start, its end, and its impact; a kit names the effect models to
// put on the head, chest, feet, hands or weapons of the unit.
const (
	spellVisualColumn = 131
	// Kit columns of SpellVisual.dbc.
	visualPrecastKit = 1
	visualCastKit    = 2
	visualImpactKit  = 3
	// Effect columns of SpellVisualKit.dbc.
	kitHead        = 3
	kitChest       = 4
	kitBase        = 5
	kitLeftHand    = 6
	kitRightHand   = 7
	kitLeftWeapon  = 9
	kitRightWeapon = 10
	noSpellEffect  = 0xFFFFFFFF

	spellCastEffectLife   = time.Second
	spellImpactEffectLife = 2 * time.Second
)

type spellVisualRow struct{ precast, cast, impact uint32 }

type spellKitRow struct {
	effects [7]uint32
}

type spellEffectName struct {
	path  string
	scale float32
}

// worldSpellEffect is a model playing on a unit for a while.
type worldSpellEffect struct {
	node    *core.Node
	info    glueModelInfo
	unit    uint64
	place   int
	until   time.Time
	cast    uint64
	forCast bool
}

// worldSpellVisuals reads the visual tables and runs the effects in play.
type worldSpellVisuals struct {
	loaded  bool
	spells  map[uint32]uint32
	visuals map[uint32]spellVisualRow
	kits    map[uint32]spellKitRow
	effects map[uint32]spellEffectName
	active  []*worldSpellEffect
}

func (v *worldSpellVisuals) load(loader *ui.Loader) {
	if v.loaded {
		return
	}
	v.loaded = true
	v.spells = make(map[uint32]uint32)
	v.visuals = make(map[uint32]spellVisualRow)
	v.kits = make(map[uint32]spellKitRow)
	v.effects = make(map[uint32]spellEffectName)
	if table, err := loadCharacterDBC(loader, `DBFilesClient\Spell.dbc`); err == nil && table.fields > spellVisualColumn {
		for row := 0; row < table.records; row++ {
			if visual := table.value(row, spellVisualColumn); visual != 0 {
				v.spells[table.value(row, 0)] = visual
			}
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\SpellVisual.dbc`); err == nil && table.fields > 3 {
		for row := 0; row < table.records; row++ {
			v.visuals[table.value(row, 0)] = spellVisualRow{precast: table.value(row, visualPrecastKit), cast: table.value(row, visualCastKit), impact: table.value(row, visualImpactKit)}
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\SpellVisualKit.dbc`); err == nil && table.fields > kitRightWeapon {
		for row := 0; row < table.records; row++ {
			var kit spellKitRow
			for index, column := range []int{kitHead, kitChest, kitBase, kitLeftHand, kitRightHand, kitLeftWeapon, kitRightWeapon} {
				kit.effects[index] = table.value(row, column)
			}
			v.kits[table.value(row, 0)] = kit
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\SpellVisualEffectName.dbc`); err == nil && table.fields > 4 {
		for row := 0; row < table.records; row++ {
			name := spellEffectName{path: table.string(row, 2), scale: math.Float32frombits(table.value(row, 4))}
			if name.scale <= 0 || name.scale > 20 {
				name.scale = 1
			}
			v.effects[table.value(row, 0)] = name
		}
	}
}

// spellEffectPath is the model file an effect names, with the extension the
// client's data uses.
func spellEffectPath(path string) string {
	lower := strings.ToLower(path)
	for _, extension := range []string{".mdx", ".mdl"} {
		if strings.HasSuffix(lower, extension) {
			return path[:len(path)-len(extension)] + ".m2"
		}
	}
	return path
}

// play puts a kit's effects on a unit.
func (v *worldSpellVisuals) play(scene *core.Node, loader *ui.Loader, kitID uint32, unit uint64, life time.Duration, caster uint64, forCast bool, now time.Time) {
	if kitID == 0 || kitID == noSpellEffect {
		return
	}
	kit, ok := v.kits[kitID]
	if !ok {
		return
	}
	for place, effectID := range kit.effects {
		if effectID == 0 || effectID == noSpellEffect {
			continue
		}
		name, ok := v.effects[effectID]
		if !ok || name.path == "" {
			continue
		}
		model, err := buildWorldCreatureModel(loader, worldCreatureDefinition{path: spellEffectPath(name.path), scale: 1, displayScale: 1, modelScale: 1})
		if err != nil {
			continue
		}
		info, _ := model.UserData().(glueModelInfo)
		node := wrapWorldModel(model, world.WorldPosition{}, name.scale)
		node.SetVisible(false)
		scene.Add(node)
		v.active = append(v.active, &worldSpellEffect{node: node, info: info, unit: unit, place: place, until: now.Add(life), cast: caster, forCast: forCast})
	}
}

// finishCast takes the effects of a cast that has ended off its caster.
func (v *worldSpellVisuals) finishCast(caster uint64) {
	for _, effect := range v.active {
		if effect.forCast && effect.cast == caster {
			effect.until = time.Time{}
		}
	}
}

// unitPlace is where a unit stands: its position, facing and height.
type unitPlace func(guid uint64) (position [3]float32, facing float32, height float32, ok bool)

// update moves the effects to their units, plays their animation and removes
// the ones that ended.
func (v *worldSpellVisuals) update(scene *core.Node, now time.Time, elapsed float64, where unitPlace) {
	live := v.active[:0]
	for _, effect := range v.active {
		position, facing, height, ok := where(effect.unit)
		if !ok || !now.Before(effect.until) {
			scene.Remove(effect.node)
			continue
		}
		offset := float32(0)
		switch effect.place {
		case 0: // head
			offset = height * 0.95
		case 1, 3, 4: // chest and hands
			offset = height * 0.6
		case 5, 6: // weapons
			offset = height * 0.55
		}
		effect.node.SetVisible(true)
		effect.node.SetPosition(position[0], position[1], position[2]+offset)
		effect.node.SetRotation(0, 0, facing)
		if effect.info.animation != nil {
			effect.info.animation.Update(elapsed)
		}
		if effect.info.particles != nil {
			effect.info.particles.Update(elapsed)
		}
		live = append(live, effect)
	}
	v.active = live
}

// handle reacts to a spell packet: the start of a cast shows its precast kit on
// the caster, its end the cast kit there and the impact kit on whom it hit.
func (v *worldSpellVisuals) handle(scene *core.Node, loader *ui.Loader, packet world.Packet, now time.Time) {
	switch packet.Opcode {
	case world.SpellStart:
		cast, err := world.ParseSpellCastHeader(packet.Body)
		if err != nil {
			return
		}
		v.load(loader)
		if visual, ok := v.visuals[v.spells[cast.SpellID]]; ok {
			life := time.Duration(cast.TimeMS)*time.Millisecond + 200*time.Millisecond
			v.play(scene, loader, visual.precast, cast.Caster, life, cast.Caster, true, now)
		}
	case world.SpellGo:
		cast, hits, err := world.ParseSpellGoHits(packet.Body)
		if err != nil {
			return
		}
		v.load(loader)
		visual, ok := v.visuals[v.spells[cast.SpellID]]
		if !ok {
			return
		}
		v.finishCast(cast.Caster)
		v.play(scene, loader, visual.cast, cast.Caster, spellCastEffectLife, cast.Caster, false, now)
		if len(hits) == 0 {
			hits = []uint64{cast.Caster}
		}
		for _, target := range hits {
			v.play(scene, loader, visual.impact, target, spellImpactEffectLife, cast.Caster, false, now)
		}
	}
}
