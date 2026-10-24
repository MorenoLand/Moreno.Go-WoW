package ui

import (
	"time"

	lua "github.com/yuin/gopher-lua"
)

// PLAYER_FLAGS of the 3.3.5a player object, and the flag of a released spirit.
const (
	playerFlagsField = 150
	playerFlagGhost  = 0x10
)

// DeathHost is what dying and coming back ask of the connection.
type DeathHost interface {
	RepopRequest() error
	ReclaimCorpse() error
	ResurrectResponse(guid uint64, accept bool) error
}

type deathState struct {
	// reclaimAt is when the corpse can be reclaimed again.
	reclaimAt time.Time
	// corpse is the GUID of the character's corpse, 0 when it has none.
	corpse uint64
	// offerer, offererName and the flags describe a pending resurrection offer.
	offerer     uint64
	offererName string
	sickness    bool
	timer       bool
	// corpseNear is set while the recover-corpse offer is showing.
	corpseNear bool
}

// SetCorpseReclaimDelay records how long until the corpse can be reclaimed.
func (rt *Runtime) SetCorpseReclaimDelay(delay time.Duration) {
	rt.death.reclaimAt = time.Now().Add(delay)
}

// SetCorpse records the character's corpse.
func (rt *Runtime) SetCorpse(guid uint64) { rt.death.corpse = guid }

// OfferResurrection shows the request another player sent.
func (rt *Runtime) OfferResurrection(guid uint64, name string, sickness, timer bool) {
	rt.death.offerer, rt.death.offererName, rt.death.sickness, rt.death.timer = guid, name, sickness, timer
	rt.FireEvent("RESURRECT_REQUEST", lua.LString(name))
}

// CancelResurrection takes a pending offer back.
func (rt *Runtime) CancelResurrection() {
	if rt.death.offerer != 0 {
		rt.death = deathState{reclaimAt: rt.death.reclaimAt, corpse: rt.death.corpse, corpseNear: rt.death.corpseNear}
		rt.FireEvent("RESURRECT_REQUEST_CANCEL")
	}
}

// SetCorpseNear shows the offer to recover the corpse while a ghost stands
// within reach of it, and takes it back when the ghost walks away.
func (rt *Runtime) SetCorpseNear(near bool) {
	if rt == nil || rt.death.corpseNear == near {
		return
	}
	rt.death.corpseNear = near
	if near {
		rt.Execute(`StaticPopup_Show("RECOVER_CORPSE")`, "@corpse")
	} else {
		rt.Execute(`StaticPopup_Hide("RECOVER_CORPSE")`, "@corpse")
	}
}

func (rt *Runtime) deathHost() DeathHost {
	host, _ := rt.Host.(DeathHost)
	return host
}

// registerDeathAPI defines the calls dying and coming back make.
func registerDeathAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("RepopMe", func(L *lua.LState) int {
		if host := rt.deathHost(); host != nil {
			_ = host.RepopRequest()
		}
		return 0
	})
	reg("RetrieveCorpse", func(L *lua.LState) int {
		if host := rt.deathHost(); host != nil {
			_ = host.ReclaimCorpse()
		}
		return 0
	})
	reg("GetCorpseRecoveryDelay", func(L *lua.LState) int {
		remaining := time.Until(rt.death.reclaimAt).Seconds()
		if remaining < 0 {
			remaining = 0
		}
		L.Push(lua.LNumber(int(remaining + 0.999)))
		return 1
	})
	reg("HasSoulstone", func(L *lua.LState) int { L.Push(lua.LNil); return 1 })
	reg("UseSoulstone", func(L *lua.LState) int { return 0 })
	reg("GetReleaseTimeRemaining", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("ResurrectGetOfferer", func(L *lua.LState) int {
		L.Push(lua.LString(rt.death.offererName))
		return 1
	})
	reg("ResurrectHasSickness", func(L *lua.LState) int { L.Push(lua.LBool(rt.death.sickness)); return 1 })
	reg("ResurrectHasTimer", func(L *lua.LState) int { L.Push(lua.LBool(rt.death.timer)); return 1 })
	respond := func(accept bool) lua.LGFunction {
		return func(L *lua.LState) int {
			if host := rt.deathHost(); host != nil && rt.death.offerer != 0 {
				_ = host.ResurrectResponse(rt.death.offerer, accept)
			}
			rt.death.offerer, rt.death.offererName = 0, ""
			return 0
		}
	}
	reg("AcceptResurrect", respond(true))
	reg("DeclineResurrect", respond(false))
}

// PlayerIsGhost reports whether the character has released its spirit.
func (rt *Runtime) PlayerIsGhost() bool {
	info := rt.unitInfo("player")
	return info != nil && info.Ghost
}

// PlayerPowerType is the kind of power the character uses (0 mana, 1 rage...).
func (rt *Runtime) PlayerPowerType() int {
	if info := rt.unitInfo("player"); info != nil {
		return info.PowerType
	}
	return -1
}
