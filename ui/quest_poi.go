package ui

import (
	"encoding/binary"
	"sort"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Quest points of interest: where on the map a quest wants the character.
// The server tells them in answer to a query of the quests in the log.
const (
	questPOIQuery    = 0x01E3
	questPOIResponse = 0x01E3
)

type questPOIPoint struct{ x, y int32 }

type questPOIBlob struct {
	index     uint32
	objective int32
	mapID     uint32
	areaID    uint32
	floor     uint32
	points    []questPOIPoint
}

type questPOIState struct {
	blobs     map[uint32][]questPOIBlob
	requested []uint32
	// visible are the quests that have a point on the page shown.
	visible []uint32
}

// requestQuestPOI asks for the points of the quests in the log when they
// changed.
func (rt *Runtime) requestQuestPOI() {
	var ids []uint32
	for _, slot := range rt.quests.slots {
		if slot.ID != 0 {
			ids = append(ids, slot.ID)
		}
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	same := len(ids) == len(rt.poi.requested)
	for i := 0; same && i < len(ids); i++ {
		same = ids[i] == rt.poi.requested[i]
	}
	if same {
		return
	}
	rt.poi.requested = ids
	if len(ids) == 0 {
		return
	}
	body := binary.LittleEndian.AppendUint32(nil, uint32(len(ids)))
	for _, id := range ids {
		body = binary.LittleEndian.AppendUint32(body, id)
	}
	rt.send(questPOIQuery, body)
}

func (rt *Runtime) handleQuestPOIPacket(packet world.Packet) bool {
	if packet.Opcode != questPOIResponse {
		return false
	}
	c := &calRead{r: world.NewReader(packet.Body, "SMSG_QUEST_POI_QUERY_RESPONSE")}
	if rt.poi.blobs == nil {
		rt.poi.blobs = map[uint32][]questPOIBlob{}
	}
	for count, i := c.u32(), uint32(0); i < count && !c.bad; i++ {
		quest := c.u32()
		var blobs []questPOIBlob
		for n, j := c.u32(), uint32(0); j < n && !c.bad; j++ {
			blob := questPOIBlob{index: c.u32(), objective: int32(c.u32()), mapID: c.u32(), areaID: c.u32(), floor: c.u32()}
			c.u32()
			c.u32()
			for points, k := c.u32(), uint32(0); k < points && !c.bad && k < 4096; k++ {
				blob.points = append(blob.points, questPOIPoint{int32(c.u32()), int32(c.u32())})
			}
			blobs = append(blobs, blob)
		}
		if !c.bad {
			rt.poi.blobs[quest] = blobs
		}
	}
	rt.FireEvent("QUEST_POI_UPDATE")
	return true
}

// questPOIOn is the blob of a quest that stands on the map page shown, the
// one its objective points to when the quest is not done and the one that
// hands it in when it is.
func (rt *Runtime) questPOIOn(quest uint32, page *MapPage) (*questPOIBlob, bool) {
	complete := false
	for _, slot := range rt.quests.slots {
		if slot.ID == quest {
			complete = rt.questComplete(slot)
		}
	}
	var best *questPOIBlob
	blobs := rt.poi.blobs[quest]
	for i := range blobs {
		blob := &blobs[i]
		if page == nil || blob.mapID != page.MapID || (rt.worldMap.zone != 0 && blob.areaID != page.ID) || len(blob.points) == 0 {
			continue
		}
		if (blob.objective == -1) == complete {
			return blob, complete
		}
		if best == nil {
			best = blob
		}
	}
	return best, complete
}

func (blob *questPOIBlob) centre() (float32, float32) {
	var x, y float64
	for _, point := range blob.points {
		x += float64(point.x)
		y += float64(point.y)
	}
	n := float64(len(blob.points))
	return float32(x / n), float32(y / n)
}

// questPOIQuests lists the quests of the log that have a point on the page.
func (rt *Runtime) questPOIQuests() []uint32 {
	page := rt.worldMap.currentPage()
	var list []uint32
	for _, row := range rt.questRows() {
		if row.slot < 0 {
			continue
		}
		id := rt.quests.slots[row.slot].ID
		if blob, _ := rt.questPOIOn(id, page); blob != nil {
			list = append(list, id)
		}
	}
	return list
}

func registerQuestPOIAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("QuestMapUpdateAllQuests", func(L *lua.LState) int {
		rt.poi.visible = rt.questPOIQuests()
		L.Push(lua.LNumber(len(rt.poi.visible)))
		return 1
	})
	reg("QuestPOIUpdateIcons", func(L *lua.LState) int {
		rt.poi.visible = rt.questPOIQuests()
		return 0
	})
	reg("QuestPOIGetQuestIDByVisibleIndex", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: QuestPOIGetQuestIDByVisibleIndex(index)")
			return 0
		}
		index := L.CheckInt(1) - 1
		if index < 0 || index >= len(rt.poi.visible) {
			return 0
		}
		id := rt.poi.visible[index]
		logIndex := 0
		for i, row := range rt.questRows() {
			if row.slot >= 0 && rt.quests.slots[row.slot].ID == id {
				logIndex = i + 1
			}
		}
		return pushAll(L, lua.LNumber(id), lua.LNumber(logIndex))
	})
	reg("GetQuestWorldMapAreaID", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetQuestWorldMapAreaID(questID)")
			return 0
		}
		blobs := rt.poi.blobs[uint32(L.CheckInt(1))]
		if len(blobs) == 0 {
			return pushAll(L, lua.LNumber(0), lua.LNumber(0))
		}
		return pushAll(L, lua.LNumber(blobs[0].areaID), lua.LNumber(blobs[0].floor))
	})
	reg("QuestPOIGetIconInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: QuestPOIGetIconInfo(questID)")
			return 0
		}
		page := rt.worldMap.currentPage()
		blob, complete := rt.questPOIOn(uint32(L.CheckInt(1)), page)
		if blob == nil || page == nil || !page.hasBounds() {
			return 0
		}
		x, y := blob.centre()
		u, v := page.project(x, y)
		if u < 0 || u > 1 || v < 0 || v > 1 {
			return 0
		}
		return pushAll(L, lua.LBool(complete), lua.LNumber(u), lua.LNumber(v), lua.LNumber(blob.objective))
	})
}
