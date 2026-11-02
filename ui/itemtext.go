package ui

import (
	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// itemTextFlagUnknown is the item flag that says the item was read before.
const itemTextRead = 0x2

// pageSource is what a readable thing says of its text: where the first page
// is, the material of the paper and its name.
type pageSource struct {
	page     uint32
	material uint32
	name     string
	item     *ItemInstance
	bag      uint8
	slot     uint8
}

// itemTextState is the book, letter or sign the character reads.
type itemTextState struct {
	open      bool
	guid      uint64
	source    pageSource
	pages     []uint32 // the id of each page read, the one after the last it knows
	texts     map[uint32]string
	next      map[uint32]uint32
	asked     map[uint32]bool
	current   int
	text      string
	began     bool // the reader has been told a text opens
	readSent  bool // the server was asked to let the character read the item
	materials map[uint32]string
}

// SetPageMaterials gives the interface the names of the papers of
// PageTextMaterial.dbc.
func (rt *Runtime) SetPageMaterials(materials map[uint32]string) { rt.itemText.materials = materials }

// SetGameObjectText gives the interface a way to learn what a game object reads:
// the first page, the material and its name.
func (rt *Runtime) SetGameObjectText(lookup func(guid uint64) (page, material uint32, name string, ok bool)) {
	rt.objectText = lookup
}

// handleItemTextPacket takes the packets of readable things.
func (rt *Runtime) handleItemTextPacket(packet world.Packet) bool {
	s := &rt.itemText
	switch packet.Opcode {
	case world.PageTextQueryResponse:
		page, err := world.ParsePageText(packet.Body)
		if err != nil {
			return true
		}
		if s.texts == nil {
			s.texts, s.next = map[uint32]string{}, map[uint32]uint32{}
		}
		s.texts[page.ID], s.next[page.ID] = page.Text, page.Next
		if s.open && s.current < len(s.pages) && s.pages[s.current] == page.ID {
			if s.began {
				rt.showPage()
			} else {
				rt.beginReading()
			}
		}
	case world.ReadItemOKPacket:
		r := world.NewReader(packet.Body, "SMSG_READ_ITEM_OK")
		if guid, err := r.U64(); err == nil && s.open && guid == s.guid {
			rt.showPage()
		}
	case world.ReadItemFailedPacket:
		r := world.NewReader(packet.Body, "SMSG_READ_ITEM_FAILED")
		if guid, err := r.U64(); err == nil && s.open && guid == s.guid {
			rt.closeItemText()
		}
	case world.GameObjectPageTextPack:
		r := world.NewReader(packet.Body, "SMSG_GAMEOBJECT_PAGETEXT")
		guid, err := r.U64()
		if err != nil || rt.objectText == nil {
			return true
		}
		if page, material, name, ok := rt.objectText(guid); ok {
			rt.openItemText(guid, pageSource{page: page, material: material, name: name}, false)
		}
	default:
		return false
	}
	return true
}

// openItemText starts reading. Opening the thing that is open closes it.
func (rt *Runtime) openItemText(guid uint64, source pageSource, toggle bool) {
	s := &rt.itemText
	if toggle && s.open && s.guid == guid {
		rt.closeItemText()
		return
	}
	if s.open && s.guid != guid {
		s.open = false
	}
	s.open, s.guid, s.source = true, guid, source
	s.pages, s.current, s.text = []uint32{source.page}, 0, ""
	if s.texts == nil {
		s.texts, s.next, s.asked = map[uint32]string{}, map[uint32]uint32{}, map[uint32]bool{}
	}
	if source.page == 0 {
		return
	}
	if _, cached := s.texts[source.page]; !cached {
		rt.askPage(source.page)
		return
	}
	rt.beginReading()
}

// beginReading tells the reader a text opens and shows its first page, once the
// server lets the character read an item that was not read before.
func (rt *Runtime) beginReading() {
	s := &rt.itemText
	if !s.began {
		s.began = true
		rt.FireEvent("ITEM_TEXT_BEGIN")
	}
	if s.source.item != nil && s.source.item.Flags&itemTextRead == 0 && !s.readSent {
		s.readSent = true
		rt.send(world.ClientReadItem, world.BuildReadItem(s.source.bag, s.source.slot))
		return
	}
	rt.showPage()
}

func (rt *Runtime) askPage(page uint32) {
	s := &rt.itemText
	if s.asked == nil {
		s.asked = map[uint32]bool{}
	}
	if s.asked[page] {
		return
	}
	s.asked[page] = true
	rt.send(world.ClientPageTextQuery, world.BuildPageTextQuery(page, s.guid))
}

// showPage puts the page the reader is on in front of the reader.
func (rt *Runtime) showPage() {
	s := &rt.itemText
	if !s.open || s.current >= len(s.pages) {
		return
	}
	page := s.pages[s.current]
	text, cached := s.texts[page]
	if !cached {
		rt.askPage(page)
		return
	}
	if s.current+1 >= len(s.pages) {
		s.pages = append(s.pages, s.next[page])
	} else {
		s.pages[s.current+1] = s.next[page]
	}
	if !s.began {
		s.began = true
		rt.FireEvent("ITEM_TEXT_BEGIN")
	}
	s.text = rt.expandNPCText(text)
	rt.FireEvent("ITEM_TEXT_READY")
}

func (rt *Runtime) closeItemText() {
	s := &rt.itemText
	if !s.open {
		return
	}
	*s = itemTextState{texts: s.texts, next: s.next, asked: s.asked, materials: s.materials}
	rt.FireEvent("ITEM_TEXT_CLOSED")
}

// readItem opens the text of a book or letter in the bags.
func (rt *Runtime) readItem(item *ItemInstance, bag, slot uint8, page uint32) {
	template := rt.itemTemplateIfKnown(item.Entry)
	if template == nil {
		return
	}
	rt.openItemText(item.GUID, pageSource{page: page, material: template.PageMaterial, name: template.Name, item: item, bag: bag, slot: slot}, true)
}

func registerItemTextAPI(rt *Runtime) {
	L := rt.L
	s := &rt.itemText
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("ItemTextGetPage", func(L *lua.LState) int {
		L.Push(lua.LNumber(s.current + 1))
		return 1
	})
	reg("ItemTextGetItem", func(L *lua.LState) int {
		L.Push(lua.LString(s.source.name))
		return 1
	})
	reg("ItemTextGetText", func(L *lua.LState) int {
		L.Push(lua.LString(s.text))
		return 1
	})
	reg("ItemTextGetMaterial", func(L *lua.LState) int {
		if name, ok := s.materials[s.source.material]; ok && s.source.material > 0 {
			L.Push(lua.LString(name))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("ItemTextGetCreator", func(L *lua.LState) int {
		L.Push(lua.LNil)
		return 1
	})
	reg("ItemTextHasNextPage", func(L *lua.LState) int {
		if s.open && s.current+1 < len(s.pages) && s.pages[s.current+1] != 0 {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("ItemTextNextPage", func(L *lua.LState) int {
		if s.open && s.current+1 < len(s.pages) && s.pages[s.current+1] > 0 {
			s.current++
			rt.showPage()
		}
		return 0
	})
	reg("ItemTextPrevPage", func(L *lua.LState) int {
		if s.open && s.current > 0 {
			s.current--
			rt.showPage()
		}
		return 0
	})
	reg("CloseItemText", func(L *lua.LState) int {
		rt.closeItemText()
		return 0
	})
}
