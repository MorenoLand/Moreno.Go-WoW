package ui

import (
	"encoding/binary"
	"math"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// The barber shop: a chair lets the character change its hair style, hair
// color, facial hair and skin color for a price by level.
const (
	barberEnablePacket = 0x0427
	barberResultPacket = 0x0428
	barberAlterOpcode  = 0x0426

	barberTypeHair   = 0
	barberTypeFacial = 2
	barberTypeSkin   = 3
	// barberColorCost is the part of the base price a color change costs.
	barberColorCost = 0.5
	// barberBytesField is PLAYER_BYTES: skin, face, hair style and hair color.
	barberBytesField = 153
)

// BarberStyle is a row of BarberShopStyle.dbc.
type BarberStyle struct {
	ID, Type          uint32
	Name, Description string
	CostMod           float32
	Race, Sex, Data   uint32
}

// BarberTables are what the barber shop reads from the client's tables.
type BarberTables struct {
	Styles []BarberStyle
	// HairColors is how many colors a hair style has, by race, sex and style.
	HairColors map[[3]uint32]int
	// Costs is the base price by level (from level 1).
	Costs []float32
}

type barberState struct {
	tables BarberTables
	open   bool
	// hair, facial and skin are the places in the lists of the style chosen.
	hair, facial, skin int
	color              int
}

// SetBarberTables installs the barber shop's tables.
func (rt *Runtime) SetBarberTables(tables BarberTables) { rt.barber.tables = tables }

// barberLists are the styles a character may choose, by kind.
func (rt *Runtime) barberLists() (hair, facial, skin []BarberStyle) {
	info := rt.unitInfo("player")
	if info == nil {
		return
	}
	race, sex := uint32(info.RaceID), uint32(info.Sex-2)
	for _, style := range rt.barber.tables.Styles {
		if style.Race != race || (style.Sex != sex && style.Sex != 0xffffffff) {
			continue
		}
		switch style.Type {
		case barberTypeHair:
			hair = append(hair, style)
		case barberTypeFacial:
			facial = append(facial, style)
		case barberTypeSkin:
			skin = append(skin, style)
		}
	}
	return
}

// barberCurrent are the looks the character has now.
func (rt *Runtime) barberCurrent() (skin, hair, color, facial uint32) {
	bytes, bytes2 := rt.field(barberBytesField), rt.field(playerBytes2Field)
	return bytes & 0xff, bytes >> 16 & 0xff, bytes >> 24, bytes2 & 0xff
}

func barberFind(list []BarberStyle, data uint32) int {
	for i, style := range list {
		if style.Data == data {
			return i
		}
	}
	return 0
}

// barberColors is how many hair colors the chosen hair style has.
func (rt *Runtime) barberColors() int {
	info := rt.unitInfo("player")
	hair, _, _ := rt.barberLists()
	if info == nil || rt.barber.hair >= len(hair) {
		return 0
	}
	return rt.barber.tables.HairColors[[3]uint32{uint32(info.RaceID), uint32(info.Sex - 2), hair[rt.barber.hair].Data}]
}

// barberReset chooses the looks the character has now.
func (rt *Runtime) barberReset() {
	hair, facial, skin := rt.barberLists()
	currentSkin, currentHair, currentColor, currentFacial := rt.barberCurrent()
	b := &rt.barber
	b.hair, b.facial, b.skin = barberFind(hair, currentHair), barberFind(facial, currentFacial), barberFind(skin, currentSkin)
	b.color = int(currentColor)
}

func (rt *Runtime) barberBase() float64 {
	info := rt.unitInfo("player")
	costs := rt.barber.tables.Costs
	if info == nil || len(costs) == 0 {
		return 0
	}
	level := info.Level - 1
	if level < 0 {
		level = 0
	}
	if level >= len(costs) {
		level = len(costs) - 1
	}
	return float64(costs[level])
}

// barberTotal is the price of the looks chosen.
func (rt *Runtime) barberTotal() int {
	if !rt.barber.open {
		return 0
	}
	hair, facial, skin := rt.barberLists()
	currentSkin, currentHair, currentColor, currentFacial := rt.barberCurrent()
	b := &rt.barber
	base := int(math.Round(rt.barberBase()))
	total := 0
	switch {
	case b.hair < len(hair) && currentHair != hair[b.hair].Data:
		total = int(math.Round(float64(base) * float64(hair[b.hair].CostMod)))
	case uint32(b.color) != currentColor:
		total = int(math.Round(float64(base) * barberColorCost))
	}
	if b.facial < len(facial) && currentFacial != facial[b.facial].Data {
		total += int(math.Round(float64(base) * float64(facial[b.facial].CostMod)))
	}
	if b.skin < len(skin) && currentSkin != skin[b.skin].Data {
		total += int(math.Round(float64(base) * float64(skin[b.skin].CostMod)))
	}
	return total
}

func (rt *Runtime) handleBarberPacket(packet world.Packet) bool {
	switch packet.Opcode {
	case barberEnablePacket:
		rt.barber.open = true
		rt.barberReset()
		rt.FireEvent("BARBER_SHOP_OPEN")
	case barberResultPacket:
		result, err := world.NewReader(packet.Body, "SMSG_BARBER_SHOP_RESULT").U32()
		if err != nil {
			return true
		}
		switch result {
		case 0:
			rt.FireEvent("BARBER_SHOP_SUCCESS")
		case 1, 3:
			rt.ShowGameError(0x28)
		case 2:
			rt.ShowGameError(0x233)
		}
	default:
		return false
	}
	return true
}

func registerBarberAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("CanAlterSkin", func(L *lua.LState) int {
		_, _, skin := rt.barberLists()
		L.Push(lua.LBool(len(skin) > 0))
		return 1
	})
	reg("GetBarberShopStyleInfo", func(L *lua.LState) int {
		if !rt.barber.open {
			return 0
		}
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetBarberShopStyleInfo(type)")
			return 0
		}
		kind := L.CheckInt(1) - 1
		hair, facial, skin := rt.barberLists()
		currentSkin, currentHair, currentColor, currentFacial := rt.barberCurrent()
		b := &rt.barber
		var style *BarberStyle
		same := false
		switch kind {
		case 0:
			if b.hair < len(hair) {
				style, same = &hair[b.hair], hair[b.hair].Data == currentHair
			}
		case 1:
			cost := int(math.Round(rt.barberBase() * barberColorCost))
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			L.Push(lua.LNumber(cost))
			if uint32(b.color) == currentColor {
				L.Push(lua.LNumber(1))
			} else {
				L.Push(lua.LNil)
			}
			return 4
		case 2:
			if b.facial < len(facial) {
				style, same = &facial[b.facial], facial[b.facial].Data == currentFacial
			}
		case 3:
			if b.skin < len(skin) {
				style, same = &skin[b.skin], skin[b.skin].Data == currentSkin
			}
		default:
			return 0
		}
		if style == nil {
			return 0
		}
		L.Push(lua.LString(style.Name))
		L.Push(lua.LString(style.Description))
		L.Push(lua.LNumber(int(math.Round(rt.barberBase() * float64(style.CostMod)))))
		if same {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 4
	})
	reg("GetBarberShopTotalCost", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.barberTotal()))
		return 1
	})
	reg("SetNextBarberShopStyle", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: SetNextBarberShopStyle(type[, backward])")
			return 0
		}
		if !rt.barber.open {
			return 0
		}
		kind := L.CheckInt(1) - 1
		step := 1
		if L.ToBool(2) {
			step = -1
		}
		hair, facial, skin := rt.barberLists()
		b := &rt.barber
		wrap := func(index, size int) int {
			if size == 0 {
				return 0
			}
			return ((index+step)%size + size) % size
		}
		switch kind {
		case 0:
			b.hair = wrap(b.hair, len(hair))
			if colors := rt.barberColors(); colors > 0 && b.color >= colors {
				b.color = 0
			}
		case 1:
			if colors := rt.barberColors(); colors > 0 {
				b.color = wrap(b.color, colors)
			}
		case 2:
			b.facial = wrap(b.facial, len(facial))
		case 3:
			if len(skin) > 0 {
				b.skin = wrap(b.skin, len(skin))
			}
		}
		return 0
	})
	reg("BarberShopReset", func(L *lua.LState) int {
		if rt.barber.open {
			rt.barberReset()
		}
		return 1
	})
	reg("CancelBarberShop", func(L *lua.LState) int {
		if rt.barber.open {
			rt.barber.open = false
			rt.FireEvent("BARBER_SHOP_CLOSE")
		}
		return 0
	})
	reg("ApplyBarberShopStyle", func(L *lua.LState) int {
		if !rt.barber.open {
			return 0
		}
		hair, facial, skin := rt.barberLists()
		b := &rt.barber
		currentSkin, currentHair, currentColor, currentFacial := rt.barberCurrent()
		if b.hair >= len(hair) || b.facial >= len(facial) {
			return 0
		}
		changed := currentHair != hair[b.hair].Data || uint32(b.color) != currentColor || currentFacial != facial[b.facial].Data ||
			(len(skin) > 0 && b.skin < len(skin) && currentSkin != skin[b.skin].Data)
		if !changed {
			return 0
		}
		if uint32(rt.inventory.money) < uint32(rt.barberTotal()) {
			rt.ShowGameError(0x28)
			return 0
		}
		body := binary.LittleEndian.AppendUint32(nil, hair[b.hair].ID)
		body = binary.LittleEndian.AppendUint32(body, uint32(b.color))
		body = binary.LittleEndian.AppendUint32(body, facial[b.facial].ID)
		skinID := uint32(0)
		if len(skin) > 0 && b.skin < len(skin) {
			skinID = skin[b.skin].ID
		}
		body = binary.LittleEndian.AppendUint32(body, skinID)
		rt.send(barberAlterOpcode, body)
		return 0
	})
}
