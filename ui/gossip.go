package ui

import (
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// NPCHost is what talking to creatures asks of the connection.
type NPCHost interface {
	// InteractNPC opens whatever a creature offers: its menu, quests, wares or
	// training, by the NPC flags it carries.
	InteractNPC(guid uint64, flags uint32) error
	GossipSelect(guid uint64, menu, option uint32, code string, coded bool) error
	QueryNPCText(textID uint32, guid uint64) error
}

// Creature NPC flags (UNIT_NPC_FLAGS).
const (
	NPCFlagGossip       = 0x00000001
	NPCFlagQuestGiver   = 0x00000002
	NPCFlagTrainer      = 0x00000010
	NPCFlagVendor       = 0x00000080
	NPCFlagRepair       = 0x00001000
	NPCFlagFlightMaster = 0x00002000
	NPCFlagSpiritHealer = 0x00004000
	NPCFlagInnkeeper    = 0x00010000
	NPCFlagBanker       = 0x00020000
)

type gossipState struct {
	open bool
	menu world.GossipMenu
	text string
	// textFor is the text id the answer is awaited for.
	textFor uint32
	cache   map[uint32]string
}

func (rt *Runtime) npcHost() NPCHost {
	host, _ := rt.Host.(NPCHost)
	return host
}

// InteractUnit talks to a creature by its NPC flags, as a right click does.
func (rt *Runtime) InteractUnit(guid uint64, flags uint32) {
	if host := rt.npcHost(); host != nil {
		_ = host.InteractNPC(guid, flags)
	}
}

// ShowGossip opens the creature's menu.
func (rt *Runtime) ShowGossip(menu world.GossipMenu) {
	rt.gossip.open = true
	rt.gossip.menu = menu
	rt.gossip.text = rt.gossip.cache[menu.TextID]
	rt.gossip.textFor = menu.TextID
	if _, known := rt.gossip.cache[menu.TextID]; !known {
		if host := rt.npcHost(); host != nil {
			_ = host.QueryNPCText(menu.TextID, menu.GUID)
		}
	}
	rt.FireEvent("GOSSIP_SHOW")
}

// SetNPCText records the greeting the server gave for a text id.
func (rt *Runtime) SetNPCText(text world.NPCText) {
	if rt.gossip.cache == nil {
		rt.gossip.cache = make(map[uint32]string)
	}
	greeting := text.Male
	if info := rt.unitInfo("player"); info != nil && info.Sex == 3 && text.Female != "" {
		greeting = text.Female
	}
	rt.gossip.cache[text.ID] = greeting
	if rt.gossip.open && rt.gossip.textFor == text.ID {
		rt.gossip.text = greeting
		rt.FireEvent("GOSSIP_SHOW")
	}
}

// CloseGossipWindow closes the menu.
func (rt *Runtime) CloseGossipWindow() {
	if !rt.gossip.open {
		return
	}
	rt.gossip.open = false
	rt.FireEvent("GOSSIP_CLOSED")
}

// gossipOptionType names the kind of a menu line by its icon, as the interface
// chooses the picture beside it.
func gossipOptionType(icon uint8) string {
	switch icon {
	case 1:
		return "vendor"
	case 2:
		return "taxi"
	case 3:
		return "trainer"
	case 4, 5:
		return "binder"
	case 6:
		return "banker"
	case 7:
		return "petition"
	case 8:
		return "tabard"
	case 9:
		return "battlemaster"
	}
	return "gossip"
}

// expandNPCText replaces the markers a creature's text carries: $N the
// player's name, $R race, $C class, $B a line break, $g male:female;.
func (rt *Runtime) expandNPCText(text string) string {
	info := rt.unitInfo("player")
	name, race, class, female := "", "", "", false
	if info != nil {
		name, race, class, female = info.Name, rt.unitRaceText(info), rt.unitClassText(info), info.Sex == 3
	}
	var out strings.Builder
	for index := 0; index < len(text); index++ {
		if text[index] != '$' || index+1 >= len(text) {
			out.WriteByte(text[index])
			continue
		}
		marker := text[index+1]
		switch marker {
		case 'N', 'n':
			out.WriteString(name)
		case 'R', 'r':
			out.WriteString(race)
		case 'C', 'c':
			out.WriteString(class)
		case 'B', 'b':
			out.WriteString("\n")
		case 'G', 'g':
			end := strings.IndexByte(text[index+2:], ';')
			if end < 0 {
				out.WriteByte('$')
				continue
			}
			parts := strings.SplitN(text[index+2:index+2+end], ":", 2)
			choice := parts[0]
			if female && len(parts) == 2 {
				choice = parts[1]
			}
			out.WriteString(choice)
			index += end + 1
		default:
			out.WriteByte('$')
			continue
		}
		index++
	}
	return out.String()
}

// registerGossipAPI defines the calls the gossip window makes.
func registerGossipAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetGossipText", func(L *lua.LState) int {
		L.Push(lua.LString(rt.expandNPCText(rt.gossip.text)))
		return 1
	})
	reg("GetNumGossipOptions", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.gossip.menu.Options)))
		return 1
	})
	reg("GetGossipOptions", func(L *lua.LState) int {
		for _, option := range rt.gossip.menu.Options {
			L.Push(lua.LString(option.Text))
			L.Push(lua.LString(gossipOptionType(option.Icon)))
		}
		return len(rt.gossip.menu.Options) * 2
	})
	reg("SelectGossipOption", func(L *lua.LState) int {
		index := L.CheckInt(1)
		menu := rt.gossip.menu
		if index < 1 || index > len(menu.Options) {
			return 0
		}
		option := menu.Options[index-1]
		if host := rt.npcHost(); host != nil {
			_ = host.GossipSelect(menu.GUID, menu.MenuID, option.ID, L.OptString(2, ""), option.Coded)
		}
		return 0
	})
	reg("CloseGossip", func(L *lua.LState) int {
		rt.CloseGossipWindow()
		return 0
	})
	reg("GetNumGossipAvailableQuests", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.gossipQuests(false))))
		return 1
	})
	reg("GetNumGossipActiveQuests", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.gossipQuests(true))))
		return 1
	})
	// Available quests give title, level, trivial, daily, repeatable; active
	// ones title, level, trivial, complete.
	pushQuests := func(active bool) lua.LGFunction {
		return func(L *lua.LState) int {
			quests := rt.gossipQuests(active)
			for _, quest := range quests {
				L.Push(lua.LString(quest.Title))
				L.Push(lua.LNumber(quest.Level))
				L.Push(lua.LBool(false))
				if active {
					L.Push(lua.LBool(quest.Icon == questIconComplete))
				} else {
					L.Push(lua.LBool(false))
					L.Push(lua.LBool(quest.Repeatable))
				}
			}
			per := 5
			if active {
				per = 4
			}
			return len(quests) * per
		}
	}
	reg("GetGossipAvailableQuests", pushQuests(false))
	reg("GetGossipActiveQuests", pushQuests(true))
	reg("SelectGossipAvailableQuest", func(L *lua.LState) int {
		rt.selectGossipQuest(L.CheckInt(1), false)
		return 0
	})
	reg("SelectGossipActiveQuest", func(L *lua.LState) int {
		rt.selectGossipQuest(L.CheckInt(1), true)
		return 0
	})
}
