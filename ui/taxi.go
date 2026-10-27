package ui

import (
	"encoding/binary"
	"sort"
	"strconv"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Taxi packets.
const (
	taxiShowNodes     uint16             = 0x01A9
	taxiNodeStatus    uint16             = 0x01AB
	taxiActivateReply uint16             = 0x01AE
	taxiNewPath       uint16             = 0x01AF
	clientTaxiExpress world.ClientOpcode = 0x0312
	clientTaxiPath    world.ClientOpcode = 0x01AD
	clientTaxiQuery   world.ClientOpcode = 0x01AC
	clientTaxiStatus  world.ClientOpcode = 0x01AA
	taxiMaskSize                         = 14
	// unitFlagTaxiFlight is UNIT_FLAG_TAXI_FLIGHT.
	unitFlagTaxiFlight = 0x00100000
)

// TaxiNode is a row of TaxiNodes.dbc.
type TaxiNode struct {
	ID         uint32
	Map        uint32
	X, Y, Z    float32
	Name       string
	AllianceOK bool
	HordeOK    bool
}

// TaxiPath is a row of TaxiPath.dbc: a flight between two nodes and its price.
type TaxiPath struct {
	From, To uint32
	Cost     uint32
}

// TaxiTables are the flight path data files.
type TaxiTables struct {
	Nodes map[uint32]TaxiNode
	Paths []TaxiPath
}

// taxiSlot is a node shown on the taxi map.
type taxiSlot struct {
	node    TaxiNode
	kind    string
	route   []uint32
	cost    uint32
	x, y    float64
	visible bool
}

type taxiState struct {
	tables  TaxiTables
	open    bool
	vendor  uint64
	current uint32
	mask    [taxiMaskSize]uint32
	slots   []taxiSlot
	// preview is the route drawn for the slot the mouse is over.
	preview int
	mapID   uint32
}

// SetTaxiTables gives the interface the flight path data.
func (rt *Runtime) SetTaxiTables(tables TaxiTables) { rt.taxi.tables = tables }

func taxiKnown(mask [taxiMaskSize]uint32, node uint32) bool {
	if node == 0 || int((node-1)/32) >= taxiMaskSize {
		return false
	}
	return mask[(node-1)/32]&(1<<((node-1)%32)) != 0
}

// taxiRoute finds the way from one node to another through the nodes the
// character knows: the fewest flights, then the cheapest.
func (rt *Runtime) taxiRoute(from, to uint32) ([]uint32, uint32, bool) {
	state := &rt.taxi
	type step struct {
		node  uint32
		hops  int
		cost  uint32
		route []uint32
	}
	best := map[uint32]step{from: {node: from, route: []uint32{from}}}
	queue := []uint32{from}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		here := best[current]
		for _, path := range state.tables.Paths {
			if path.From != current || !taxiKnown(state.mask, path.To) {
				continue
			}
			next := step{node: path.To, hops: here.hops + 1, cost: here.cost + path.Cost, route: append(append([]uint32(nil), here.route...), path.To)}
			if known, seen := best[path.To]; seen && (known.hops < next.hops || known.hops == next.hops && known.cost <= next.cost) {
				continue
			}
			best[path.To] = next
			queue = append(queue, path.To)
		}
	}
	found, ok := best[to]
	return found.route, found.cost, ok
}

// OpenTaxiMap shows the flight map of a flight master.
func (rt *Runtime) OpenTaxiMap(vendor uint64, current uint32, mask [taxiMaskSize]uint32) {
	state := &rt.taxi
	state.open, state.vendor, state.current, state.mask = true, vendor, current, mask
	state.preview = -1
	state.slots = state.slots[:0]
	here, ok := state.tables.Nodes[current]
	if !ok {
		return
	}
	state.mapID = here.Map
	team := rt.playerTeamHorde()
	var ids []uint32
	for id, node := range state.tables.Nodes {
		if node.Map != here.Map || (team && !node.HordeOK) || (!team && !node.AllianceOK) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	for _, id := range ids {
		node := state.tables.Nodes[id]
		slot := taxiSlot{node: node, kind: "NONE"}
		switch {
		case id == current:
			slot.kind, slot.visible = "CURRENT", true
		case taxiKnown(mask, id):
			if route, cost, ok := rt.taxiRoute(current, id); ok {
				slot.route, slot.cost, slot.visible = route, cost, true
				slot.kind = "DISTANT"
				if len(route) == 2 {
					slot.kind = "REACHABLE"
				}
			}
		}
		slot.x, slot.y = rt.taxiPosition(node)
		state.slots = append(state.slots, slot)
	}
	rt.FireEvent("TAXIMAP_OPENED")
}

// playerTeamHorde reports whether the character is of the Horde.
func (rt *Runtime) playerTeamHorde() bool {
	if info := rt.unitInfo("player"); info != nil {
		switch info.RaceID {
		case 2, 5, 6, 8, 10:
			return true
		}
	}
	return false
}

// taxiPosition places a node on the taxi map of its continent as fractions of
// the picture, measured from the left and from the bottom.
func (rt *Runtime) taxiPosition(node TaxiNode) (float64, float64) {
	catalog := rt.worldMap.catalog
	if catalog == nil {
		return 0, 0
	}
	for index := range catalog.Pages {
		page := &catalog.Pages[index]
		if page.MapID == node.Map && page.AreaID == 0 && page.hasBounds() {
			return float64(page.YMax-node.Y) / float64(page.YMax-page.YMin), float64(node.X-page.XMin) / float64(page.XMax-page.XMin)
		}
	}
	return 0, 0
}

// CloseTaxiMap hides the flight map.
func (rt *Runtime) CloseTaxiMap() {
	if rt.taxi.open {
		rt.taxi.open = false
		rt.FireEvent("TAXIMAP_CLOSED")
	}
}

var taxiReplyErrors = []string{"", "ERR_TAXIUNSPECIFIEDSERVERERROR", "ERR_TAXINOSUCHPATH", "ERR_TAXINOTENOUGHMONEY", "ERR_TAXITOOFARAWAY",
	"ERR_TAXINOVENDORNEARBY", "ERR_TAXINOTVISITED", "ERR_TAXIPLAYERBUSY", "ERR_TAXIPLAYERALREADYMOUNTED", "ERR_TAXIPLAYERSHAPESHIFTED",
	"ERR_TAXIPLAYERMOVING", "ERR_TAXISAMENODE", "ERR_TAXINOTSTANDING"}

// handleTaxiPacket takes the packets of flight masters.
func (rt *Runtime) handleTaxiPacket(packet world.Packet) bool {
	switch packet.Opcode {
	case taxiShowNodes:
		reader := world.NewReader(packet.Body, "SMSG_SHOWTAXINODES")
		if _, err := reader.U32(); err != nil {
			return true
		}
		guid, err := reader.U64()
		if err != nil {
			return true
		}
		current, err := reader.U32()
		if err != nil {
			return true
		}
		var mask [taxiMaskSize]uint32
		for index := range mask {
			if mask[index], err = reader.U32(); err != nil {
				break
			}
		}
		rt.OpenTaxiMap(guid, current, mask)
	case taxiActivateReply:
		if reply, err := world.NewReader(packet.Body, "SMSG_ACTIVATETAXIREPLY").U32(); err == nil && reply > 0 && int(reply) < len(taxiReplyErrors) {
			rt.ShowGlobalError(taxiReplyErrors[reply])
		} else if err == nil && reply == 0 {
			rt.CloseTaxiMap()
		}
	case taxiNewPath:
		rt.ShowInfoMessage(rt.globalString("ERR_NEWTAXIPATH", "New flight path discovered!"))
	case taxiNodeStatus:
	default:
		return false
	}
	return true
}

// registerTaxiAPI defines the calls of the flight map.
func registerTaxiAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	push := func(L *lua.LState, values ...lua.LValue) int {
		for _, value := range values {
			L.Push(value)
		}
		return len(values)
	}
	slotAt := func(L *lua.LState, index int) *taxiSlot {
		slot := L.CheckInt(index) - 1
		if slot < 0 || slot >= len(rt.taxi.slots) {
			return nil
		}
		return &rt.taxi.slots[slot]
	}
	reg("NumTaxiNodes", func(L *lua.LState) int { return push(L, lua.LNumber(len(rt.taxi.slots))) })
	reg("TaxiNodeName", func(L *lua.LState) int {
		if slot := slotAt(L, 1); slot != nil {
			return push(L, lua.LString(slot.node.Name))
		}
		return push(L, lua.LString("INVALID"))
	})
	reg("TaxiNodePosition", func(L *lua.LState) int {
		slot := slotAt(L, 1)
		if slot == nil {
			L.RaiseError("Invalid taxi node slot")
			return 0
		}
		return push(L, lua.LNumber(slot.x), lua.LNumber(slot.y))
	})
	reg("TaxiNodeGetType", func(L *lua.LState) int {
		if slot := slotAt(L, 1); slot != nil {
			return push(L, lua.LString(slot.kind))
		}
		return push(L, lua.LString("NONE"))
	})
	reg("TaxiNodeCost", func(L *lua.LState) int {
		slot := slotAt(L, 1)
		if slot == nil {
			L.RaiseError("Invalid taxi node slot")
			return 0
		}
		return push(L, lua.LNumber(slot.cost))
	})
	reg("TaxiNodeSetCurrent", func(L *lua.LState) int {
		rt.taxi.preview = -1
		if index := L.CheckInt(1) - 1; index >= 0 && index < len(rt.taxi.slots) && rt.taxi.slots[index].visible {
			rt.taxi.preview = index
		}
		return 0
	})
	reg("GetNumRoutes", func(L *lua.LState) int {
		slot := slotAt(L, 1)
		if slot == nil {
			return push(L, lua.LNumber(0))
		}
		if len(slot.route) == 0 {
			return push(L, lua.LNumber(1))
		}
		return push(L, lua.LNumber(len(slot.route)-1))
	})
	hop := func(destination bool, y bool) lua.LGFunction {
		return func(L *lua.LState) int {
			slot := slotAt(L, 1)
			hopIndex := L.CheckInt(2) - 1
			if slot == nil || hopIndex < 0 {
				return push(L, lua.LNumber(0))
			}
			var from, to TaxiNode
			if len(slot.route) == 0 {
				from, to = rt.taxi.tables.Nodes[rt.taxi.current], slot.node
			} else {
				if hopIndex+1 >= len(slot.route) {
					return push(L, lua.LNumber(0))
				}
				from, to = rt.taxi.tables.Nodes[slot.route[hopIndex]], rt.taxi.tables.Nodes[slot.route[hopIndex+1]]
			}
			node := from
			if destination {
				node = to
			}
			px, py := rt.taxiPosition(node)
			if y {
				return push(L, lua.LNumber(py))
			}
			return push(L, lua.LNumber(px))
		}
	}
	reg("TaxiGetSrcX", hop(false, false))
	reg("TaxiGetSrcY", hop(false, true))
	reg("TaxiGetDestX", hop(true, false))
	reg("TaxiGetDestY", hop(true, true))
	reg("TakeTaxiNode", func(L *lua.LState) int {
		slot := slotAt(L, 1)
		state := &rt.taxi
		if slot == nil || !slot.visible {
			return 0
		}
		if slot.node.ID == state.current {
			rt.ShowGlobalError("ERR_TAXISAMENODE")
			return 0
		}
		if info := rt.unitInfo("player"); info != nil && int(rt.inventory.money) < int(slot.cost) {
			rt.ShowGlobalError("ERR_NOT_ENOUGH_MONEY")
			return 0
		}
		if len(slot.route) < 2 {
			rt.ShowGlobalError("ERR_TAXINOSUCHPATH")
			return 0
		}
		if len(slot.route) == 2 {
			body := binary.LittleEndian.AppendUint64(nil, state.vendor)
			body = binary.LittleEndian.AppendUint32(body, slot.route[0])
			body = binary.LittleEndian.AppendUint32(body, slot.route[1])
			rt.send(clientTaxiPath, body)
			return 0
		}
		body := binary.LittleEndian.AppendUint64(nil, state.vendor)
		body = binary.LittleEndian.AppendUint32(body, uint32(len(slot.route)))
		for _, node := range slot.route {
			body = binary.LittleEndian.AppendUint32(body, node)
		}
		rt.send(clientTaxiExpress, body)
		return 0
	})
	reg("CloseTaxiMap", func(L *lua.LState) int { rt.CloseTaxiMap(); return 0 })
	reg("SetTaxiMap", func(L *lua.LState) int {
		if w := rt.widgetArg(L, 1); w != nil {
			w.textureFile = `Interface\TaxiFrame\TAXIMAP` + strconv.Itoa(int(rt.taxi.mapID))
		}
		return 0
	})
	reg("UnitOnTaxi", func(L *lua.LState) int {
		info := rt.unitInfo(L.CheckString(1))
		return push(L, luaFlag(info != nil && info.OnTaxi))
	})
	reg("TaxiGetNodeSlot", func(L *lua.LState) int {
		slot := slotAt(L, 1)
		if slot == nil {
			return push(L, lua.LNumber(0))
		}
		hopIndex := L.CheckInt(2) - 1
		if len(slot.route) == 0 || hopIndex < 0 || hopIndex >= len(slot.route) {
			return push(L, lua.LNumber(0))
		}
		node := slot.route[hopIndex]
		for index, candidate := range rt.taxi.slots {
			if candidate.node.ID == node {
				return push(L, lua.LNumber(index+1))
			}
		}
		return push(L, lua.LNumber(0))
	})
}
