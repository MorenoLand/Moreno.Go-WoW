package ui

import (
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Mirror timers: the exhaustion, breath and feign death bars.
const (
	mirrorTimerStart = 0x01D9
	mirrorTimerPause = 0x01DA
	mirrorTimerStop  = 0x01DB
	mirrorTimerCount = 3
)

var mirrorTimerNames = [mirrorTimerCount]string{"EXHAUSTION", "BREATH", "FEIGNDEATH"}

func mirrorTimerName(kind uint32) string {
	if kind < mirrorTimerCount {
		return mirrorTimerNames[kind]
	}
	return "UNKNOWN"
}

// mirrorTimer is a timer the server started: its value in milliseconds when
// it began, how fast it counts and when it began.
type mirrorTimer struct {
	active  bool
	value   int32
	maximum int32
	scale   int32
	paused  uint8
	spell   uint32
	began   time.Time
}

// label is the text of the bar: the name of the spell behind it, or the
// timer's own label.
func (rt *Runtime) mirrorTimerLabel(kind, spell uint32) string {
	if info, ok := rt.gameplay.Catalog[spell]; ok && spell != 0 {
		return info.Name
	}
	return rt.globalString(mirrorTimerName(kind)+"_LABEL", "")
}

func (rt *Runtime) handleMirrorTimerPacket(packet world.Packet) bool {
	switch packet.Opcode {
	case mirrorTimerStart:
		reader := world.NewReader(packet.Body, "mirror timer start")
		var fields [4]uint32
		for index := range fields {
			value, err := reader.U32()
			if err != nil {
				return true
			}
			fields[index] = value
		}
		paused, err := reader.U8()
		if err != nil {
			return true
		}
		spell, err := reader.U32()
		if err != nil {
			return true
		}
		kind := fields[0]
		rt.FireEvent("MIRROR_TIMER_START", lua.LString(mirrorTimerName(kind)), lua.LNumber(int32(fields[1])), lua.LNumber(int32(fields[2])),
			lua.LNumber(int32(fields[3])), lua.LNumber(paused), lua.LString(rt.mirrorTimerLabel(kind, spell)))
		if kind < mirrorTimerCount {
			rt.mirrors[kind] = mirrorTimer{active: true, value: int32(fields[1]), maximum: int32(fields[2]),
				scale: int32(fields[3]), paused: paused, spell: spell, began: time.Now()}
		}
		return true
	case mirrorTimerPause:
		reader := world.NewReader(packet.Body, "mirror timer pause")
		kind, err := reader.U32()
		if err != nil {
			return true
		}
		paused, err := reader.U8()
		if err != nil {
			return true
		}
		rt.FireEvent("MIRROR_TIMER_PAUSE", lua.LString(mirrorTimerName(kind)), lua.LNumber(paused))
		return true
	case mirrorTimerStop:
		reader := world.NewReader(packet.Body, "mirror timer stop")
		kind, err := reader.U32()
		if err != nil {
			return true
		}
		rt.FireEvent("MIRROR_TIMER_STOP", lua.LString(mirrorTimerName(kind)))
		if kind < mirrorTimerCount {
			rt.mirrors[kind] = mirrorTimer{}
		}
		return true
	}
	return false
}

func registerMirrorTimerAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("GetMirrorTimerInfo", L.NewFunction(func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetMirrorTimerInfo(id)")
		}
		index := L.CheckInt(1) - 1
		if index < 0 || index >= mirrorTimerCount {
			L.RaiseError("Usage: GetMirrorTimerInfo(id)")
		}
		timer := rt.mirrors[index]
		if !timer.active {
			L.Push(lua.LString("UNKNOWN"))
			for range 4 {
				L.Push(lua.LNumber(0))
			}
			L.Push(lua.LString(""))
			return 6
		}
		L.Push(lua.LString(mirrorTimerName(uint32(index))))
		L.Push(lua.LNumber(timer.value))
		L.Push(lua.LNumber(timer.maximum))
		L.Push(lua.LNumber(timer.scale))
		L.Push(lua.LNumber(timer.paused))
		L.Push(lua.LString(rt.mirrorTimerLabel(uint32(index), timer.spell)))
		return 6
	}))
	L.SetGlobal("GetMirrorTimerProgress", L.NewFunction(func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError("Usage: GetMirrorTimerProgress(\"timer\")")
		}
		name := L.CheckString(1)
		kind := uint32(mirrorTimerCount)
		for index, candidate := range mirrorTimerNames {
			if name == candidate {
				kind = uint32(index)
			}
		}
		if kind >= mirrorTimerCount {
			L.RaiseError("Usage: GetMirrorTimerProgress(\"timer\")")
		}
		timer := rt.mirrors[kind]
		elapsed := int32(time.Since(timer.began) / time.Millisecond)
		L.Push(lua.LNumber(elapsed*timer.scale + timer.value))
		return 1
	}))
}
