package ui

import lua "github.com/yuin/gopher-lua"

// SetObjectInfo gives the interface the kind and the numbers of a game object.
func (rt *Runtime) SetObjectInfo(source func(guid uint64) (uint8, [24]uint32, bool)) {
	rt.objectInfo = source
}

// The high words of the guids of items and game objects.
const (
	guidHighItem       = 0x4000
	guidHighGameObject = 0xF110
	guidHighTransport  = 0xF120
)

// The kinds of game object that carry the paper of a text, and where the number
// of the paper is among their numbers.
var objectMaterialIndex = map[uint8]int{2: 2, 9: 2, 10: 9}

// questBackgroundMaterial is the paper the quest window is drawn on: the paper
// of the item or the game object that gave the quest.
func (rt *Runtime) questBackgroundMaterial() (string, bool) {
	giver := rt.quests.dialog.giver
	var material uint32
	switch giver >> 48 {
	case guidHighItem:
		item := rt.inventory.items[giver]
		if item == nil {
			return "", false
		}
		template := rt.itemTemplate(item.Entry)
		if template == nil {
			return "", false
		}
		material = template.PageMaterial
	case guidHighGameObject, guidHighTransport:
		if rt.objectInfo == nil {
			return "", false
		}
		kind, data, ok := rt.objectInfo(giver)
		index, has := objectMaterialIndex[kind]
		if !ok || !has {
			return "", false
		}
		material = data[index]
	default:
		return "", false
	}
	name, ok := rt.itemText.materials[material]
	return name, ok && material > 0
}

func registerQuestMaterialAPI(rt *Runtime) {
	rt.L.SetGlobal("GetQuestBackgroundMaterial", rt.L.NewFunction(func(L *lua.LState) int {
		if name, ok := rt.questBackgroundMaterial(); ok {
			L.Push(lua.LString(name))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	}))
}
