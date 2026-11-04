package ui

import (
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// miniArrowNames are the two models that show the player on the small map of a
// battlefield.
var miniArrowNames = []string{"PlayerMiniArrowFrame", "PlayerMiniArrowEffectFrame"}

// miniArrowSize is how big they are next to the arrows of the world map: the
// client scales them 1.1111 against 1.6667.
const miniArrowSize = 32 * (1.1111111640930176 / 1.6666666269302368)

// registerMiniWorldMapArrow defines the calls behind the arrow on the small
// map, which work like the ones of the world map.
func registerMiniWorldMapArrow(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	arrows := func() []*widget {
		var frames []*widget
		for _, name := range miniArrowNames {
			if frame := rt.widgets[name]; frame != nil {
				frames = append(frames, frame)
			}
		}
		return frames
	}
	reg("CreateMiniWorldMapArrowFrame", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTUserData {
			L.RaiseError("Usage: CreateMiniWorldMapArrowFrame(parent)")
			return 0
		}
		parent := rt.widgetArg(L, 1)
		if parent == nil {
			L.RaiseError("CreateMiniWorldMapArrowFrame(): Wrong object type, expected frame")
			return 0
		}
		for _, name := range miniArrowNames {
			if rt.widgets[name] != nil {
				continue
			}
			frame := newWidget(kindFrame, name)
			frame.typeName = "Model"
			frame.parent = parent
			frame.shown = false
			frame.modelFile = `Interface\Minimap\MinimapArrow.mdx`
			frame.width, frame.height = miniArrowSize, miniArrowSize
			frame.frameLevel = 200
			arrow := newWidget(kindTexture, "")
			arrow.parent = frame
			arrow.textureFile = `Interface\Minimap\MinimapArrow.blp`
			arrow.layerLevel = layerArtwork
			arrow.points = []anchorPoint{{point: "TOPLEFT", relativePoint: "TOPLEFT"}, {point: "BOTTOMRIGHT", relativePoint: "BOTTOMRIGHT"}}
			addWidgetChild(frame, arrow)
			rt.mapArrowTextures = append(rt.mapArrowTextures, arrow)
			frame.frameStrata = parent.frameStrata
			addWidgetChild(parent, frame)
			rt.register(frame)
		}
		return 0
	})
	reg("ShowMiniWorldMapArrowFrame", func(L *lua.LState) int {
		frames := arrows()
		if len(frames) != len(miniArrowNames) {
			return 0
		}
		show := L.GetTop() < 1 || lua.LVAsBool(L.Get(1))
		for _, frame := range frames {
			frame.shown = show
		}
		return 0
	})
	reg("PositionMiniWorldMapArrowFrame", func(L *lua.LState) int {
		frames := arrows()
		if len(frames) != len(miniArrowNames) {
			return 0
		}
		if L.Get(1).Type() != lua.LTString || L.Get(2).Type() != lua.LTString {
			L.RaiseError(`Usage: PositionMiniWorldMapArrowFrame("point" "frame" [, relativePoint] [, offsetX, offsetY])`)
			return 0
		}
		point := strings.ToUpper(L.CheckString(1))
		if layoutPointIndex(point) < 0 {
			L.RaiseError("Unknown frame point")
			return 0
		}
		relativeTo := L.CheckString(2)
		if rt.widgets[relativeTo] == nil {
			L.RaiseError("Couldn't find frame named '%s'", relativeTo)
			return 0
		}
		if relativeTo == frames[0].name {
			L.RaiseError("Error: %s is anchored to itself", relativeTo)
			return 0
		}
		relativePoint, x, y := point, 0.0, 0.0
		if L.Get(3).Type() == lua.LTString {
			relativePoint = strings.ToUpper(L.CheckString(3))
			if layoutPointIndex(relativePoint) < 0 {
				L.RaiseError("Unknown frame point")
				return 0
			}
			if L.Get(4).Type() == lua.LTNumber && L.Get(5).Type() == lua.LTNumber {
				x, y = float64(L.CheckNumber(4)), float64(L.CheckNumber(5))
			}
		}
		for _, frame := range frames {
			frame.points = []anchorPoint{{point: point, relativeTo: relativeTo, relativePoint: relativePoint, x: x, y: y}}
		}
		return 0
	})
	reg("InitWorldMapPing", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTUserData {
			L.RaiseError("Usage: InitWorldMapPing(parent)")
			return 0
		}
		if rt.widgetArg(L, 1) == nil {
			L.RaiseError("InitWorldMapPing(): Wrong object type, expected frame")
		}
		return 0
	})
}
