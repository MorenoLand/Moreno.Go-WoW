package render

import (
	"fmt"
	"sort"

	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/graphic"
)

// drawnBreakdown says what the last frame's draws were: how many came from the
// map (terrain, map objects, doodads), from units, from game objects, and what
// else (names, particles, effects).
func drawnBreakdown(drawn []*graphic.Graphic, scene *core.Node, worldModel *core.Node, entities map[uint64]*worldEntity) string {
	owner := make(map[*core.Node]string, len(entities))
	for _, entity := range entities {
		if entity.node == nil {
			continue
		}
		switch entity.objectType {
		case world.ObjectTypeGameObject:
			owner[entity.node] = "objects"
		case world.ObjectTypePlayer:
			owner[entity.node] = "players"
		default:
			owner[entity.node] = "units"
		}
	}
	counts := map[string]int{}
	for _, g := range drawn {
		top := g.GetNode()
		for {
			parent, ok := top.Parent().(core.INode)
			if !ok || parent == nil || parent.GetNode() == scene {
				break
			}
			top = parent.GetNode()
		}
		name := "other"
		switch {
		case top == worldModel:
			shader := ""
			if materials := g.Materials(); len(materials) > 0 {
				shader = materials[0].IMaterial().GetMaterial().Shader()
			}
			switch shader {
			case "morenowow_world_terrain":
				name = "terrain"
			case "morenowow_world_wmo", "morenowow_world_wmo_alpha_key":
				name = "buildings"
			case "morenowow_world_liquid":
				name = "water"
			case "morenowow_particle":
				name = "particles"
			default:
				name = "doodads"
			}
		case owner[top] != "":
			name = owner[top]
		}
		counts[name] += len(g.Materials())
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)
	text := "draws"
	for _, name := range names {
		text += fmt.Sprintf(" | %s %d", name, counts[name])
	}
	return text
}
