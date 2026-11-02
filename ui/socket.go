package ui

import (
	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Enchantment slots the sockets use, and the socket colours.
const (
	enchantSlotSocket1 = 2
	socketCount        = 3
	gemMeta            = 1
)

var socketColourNames = [...]string{"Meta", "Red", "Yellow", "Blue"}

// SocketTables are the tables the socketing window reads.
type SocketTables struct {
	// GemTypes is the colours of each GemProperties.dbc row.
	GemTypes map[uint32]uint32
	// EnchantGems is the gem each SpellItemEnchantment.dbc row stands for and
	// EnchantNames the text of each.
	EnchantGems  map[uint32]uint32
	EnchantNames map[uint32]string
}

// socketSlot is a gem put in a socket of the item, waiting to be accepted.
type socketSlot struct {
	guid     uint64
	entry    uint32
	bag, pos int
}

type socketState struct {
	tables SocketTables
	item   uint64
	gems   [socketCount]socketSlot
	// pickup puts an item on the cursor again.
	pickup func(bag, slot int, guid uint64, entry uint32)
}

// SetSocketTables gives the interface the tables the socketing window reads.
func (rt *Runtime) SetSocketTables(tables SocketTables) { rt.sockets.tables = tables }

func (rt *Runtime) socketItem() (*ItemInstance, *world.ItemTemplate) {
	item := rt.inventory.items[rt.sockets.item]
	if item == nil {
		return nil, nil
	}
	return item, rt.itemTemplateIfKnown(item.Entry)
}

func socketColours(template *world.ItemTemplate, index int) uint32 {
	if template == nil || index < 0 || index >= len(template.Sockets) {
		return 0
	}
	return template.Sockets[index].Color
}

// socketCountOf is how many sockets an item has.
func socketCountOf(template *world.ItemTemplate) int {
	count := 0
	if template != nil {
		for _, socket := range template.Sockets {
			if socket.Color != 0 {
				count++
			}
		}
	}
	return count
}

// gemMatches reports whether a gem fits the colours of a socket: a socket with
// no colour takes any gem.
func (rt *Runtime) gemMatches(template, gem *world.ItemTemplate, index int) bool {
	if template == nil || gem == nil {
		return false
	}
	socket := socketColours(template, index)
	if socket == 0 {
		return true
	}
	return rt.sockets.tables.GemTypes[gem.GemProperties]&socket&0xF != 0
}

// existingGem is the gem item socketed in an item's socket.
func (rt *Runtime) existingGem(item *ItemInstance, index int) uint32 {
	if index < 0 || index >= socketCount {
		return 0
	}
	return rt.sockets.tables.EnchantGems[item.Enchants[enchantSlotSocket1+index]]
}

func (rt *Runtime) closeSockets() {
	s := &rt.sockets
	if s.item == 0 {
		return
	}
	s.item = 0
	s.gems = [socketCount]socketSlot{}
	rt.FireEvent("SOCKET_INFO_CLOSE")
}

func (rt *Runtime) openSockets(item *ItemInstance) {
	s := &rt.sockets
	s.item = item.GUID
	s.gems = [socketCount]socketSlot{}
	if socketCountOf(rt.itemTemplateIfKnown(item.Entry)) > 0 {
		rt.FireEvent("SOCKET_INFO_UPDATE")
		return
	}
	s.item = 0
	rt.FireEvent("SOCKET_INFO_CLOSE")
}

func (rt *Runtime) socketIconLines(L *lua.LState, gem *world.ItemTemplate, match bool) int {
	L.Push(lua.LString(gem.Name))
	L.Push(lua.LString(rt.itemIcon(gem.Entry)))
	if match {
		L.Push(lua.LNumber(1))
	} else {
		L.Push(lua.LNil)
	}
	return 3
}

func registerSocketAPI(rt *Runtime) {
	L := rt.L
	s := &rt.sockets
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	empty := func(L *lua.LState) int {
		for range 3 {
			L.Push(lua.LNil)
		}
		return 3
	}
	reg("SocketInventoryItem", func(L *lua.LState) int {
		if item := rt.equippedItem(L.CheckInt(1)); item != nil && socketCountOf(rt.itemTemplateIfKnown(item.Entry)) > 0 {
			rt.openSockets(item)
		}
		return 0
	})
	reg("SocketContainerItem", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: SocketContainerItem(index, slot)")
		}
		if item := rt.bagItem(L.CheckInt(1), L.CheckInt(2)); item != nil && socketCountOf(rt.itemTemplateIfKnown(item.Entry)) > 0 {
			rt.openSockets(item)
		}
		return 0
	})
	reg("GetNumSockets", func(L *lua.LState) int {
		_, template := rt.socketItem()
		L.Push(lua.LNumber(socketCountOf(template)))
		return 1
	})
	reg("GetSocketTypes", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetSocketType(index)")
		}
		_, template := rt.socketItem()
		if template == nil {
			return 0
		}
		mask, count := socketColours(template, L.CheckInt(1)-1), 0
		for bit, name := range socketColourNames {
			if mask&(1<<bit) != 0 {
				L.Push(lua.LString(name))
				count++
			}
		}
		if count == 0 {
			L.Push(lua.LString("Socket"))
			count = 1
		}
		return count
	})
	reg("GetSocketItemInfo", func(L *lua.LState) int {
		item, template := rt.socketItem()
		if item == nil || template == nil {
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			L.Push(lua.LNumber(0))
			return 3
		}
		L.Push(lua.LString(template.Name))
		L.Push(lua.LString(rt.itemIcon(item.Entry)))
		L.Push(lua.LNumber(template.Quality))
		return 3
	})
	reg("GetExistingSocketInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetExistingSocketInfo(index)")
		}
		index := L.CheckInt(1) - 1
		item, template := rt.socketItem()
		if item == nil || index < 0 || index >= socketCount {
			return empty(L)
		}
		gem := rt.itemTemplate(rt.existingGem(item, index))
		if gem == nil {
			return empty(L)
		}
		return rt.socketIconLines(L, gem, rt.gemMatches(template, gem, index))
	})
	reg("GetExistingSocketLink", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetExistingSocketLink(index)")
		}
		item, _ := rt.socketItem()
		index := L.CheckInt(1) - 1
		if item == nil || index < 0 || index >= socketCount {
			return 0
		}
		if entry := rt.existingGem(item, index); entry != 0 {
			if link := rt.itemLinkFor(entry, 0); link != "" {
				L.Push(lua.LString(link))
				return 1
			}
		}
		return 0
	})
	newGem := func(L *lua.LState, usage string) (*world.ItemTemplate, int) {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("%s", usage)
		}
		index := L.CheckInt(1) - 1
		if index < 0 || index >= socketCount || s.gems[index].guid == 0 {
			return nil, index
		}
		return rt.itemTemplate(s.gems[index].entry), index
	}
	reg("GetNewSocketInfo", func(L *lua.LState) int {
		gem, index := newGem(L, "Usage: GetNewSocketInfo(index)")
		_, template := rt.socketItem()
		if gem == nil {
			return empty(L)
		}
		return rt.socketIconLines(L, gem, rt.gemMatches(template, gem, index))
	})
	reg("GetNewSocketLink", func(L *lua.LState) int {
		gem, _ := newGem(L, "Usage: GetNewSocketLink(index)")
		if gem == nil {
			return 0
		}
		if link := rt.itemLinkFor(gem.Entry, 0); link != "" {
			L.Push(lua.LString(link))
			return 1
		}
		return 0
	})
	reg("ClickSocketButton", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: ClickSocketButton(index)")
		}
		index := L.CheckInt(1) - 1
		if index < 0 || index >= socketCount || s.item == 0 {
			return 0
		}
		_, template := rt.socketItem()
		held := rt.cursorItemInstance()
		var incoming socketSlot
		if held != nil {
			gem := rt.itemTemplateIfKnown(held.Entry)
			if gem == nil || gem.GemProperties == 0 {
				return 0
			}
			// A meta socket takes only a meta gem and the other sockets none.
			if socket, kind := socketColours(template, index), s.tables.GemTypes[gem.GemProperties]; kind != 0 && socket&gemMeta != kind&gemMeta {
				if socket&gemMeta != 0 {
					rt.ShowGameError(0x22A)
				} else {
					rt.ShowGameError(0x22B)
				}
				return 0
			}
			cursor := rt.inventory.cursor
			incoming = socketSlot{guid: held.GUID, entry: held.Entry, bag: cursor.bag, pos: cursor.slot}
		}
		previous := s.gems[index]
		s.gems[index] = incoming
		if held != nil && rt.clearItemCursor != nil {
			rt.clearItemCursor()
		}
		rt.FireEvent("SOCKET_INFO_UPDATE")
		if previous.guid != 0 && s.pickup != nil {
			s.pickup(previous.bag, previous.pos, previous.guid, previous.entry)
		}
		return 0
	})
	reg("AcceptSockets", func(L *lua.LState) int {
		if s.item == 0 {
			return 0
		}
		var gems [socketCount]uint64
		any := false
		for index := range gems {
			gems[index] = s.gems[index].guid
			any = any || gems[index] != 0
		}
		if any {
			rt.send(world.ClientSocketGems, world.BuildSocketGems(s.item, gems))
		}
		return 0
	})
	reg("CloseSocketInfo", func(L *lua.LState) int {
		rt.closeSockets()
		return 0
	})
}
