package ui

import (
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// WorldControlHost is what the interface asks of the world for the calls that
// move the character, turn the camera and pick targets.
type WorldControlHost interface {
	// Movement presses or releases a movement command (MOVEFORWARD and so on).
	Movement(command string, down bool)
	// MouseAction presses or releases a mouse button over the world (0 left,
	// 1 right).
	MouseAction(button int, down bool)
	// CameraZoom moves the camera in (positive) or out by yards.
	CameraZoom(yards float64)
	// CameraYaw turns the camera by degrees.
	CameraYaw(degrees float64)
	// SetCameraView moves the camera to the distance of a saved view.
	SetCameraView(distance float64) float64
	// CameraDistance is how far the camera is from the character.
	CameraDistance() float64
	// TargetNearest targets the nearest unit of a kind ("enemy", "enemyplayer",
	// "friend", "friendplayer", "party", "raid"); reverse goes the other way.
	TargetNearest(kind string, reverse bool)
	// TargetLast targets the last unit of a kind that was targeted.
	TargetLast(kind string)
	// Follow follows a unit until moved by hand; zero stops following.
	Follow(guid uint64)
	// Interact does what a right click on a unit does.
	Interact(guid uint64)
	// TargetOf is the guid a unit has targeted.
	TargetOf(guid uint64) uint64
	// Falling reports whether the character is falling.
	Falling() bool
	// MoveState reports whether the character swims, flies and stands indoors.
	MoveState() (swimming, flying, indoors bool)
	// Framerate is the frames drawn per second.
	Framerate() float64
}

// savedViews are the camera distances of the five views, the first being the
// closest.
var defaultCameraViews = [5]float64{0, 5.5, 10, 15, 20}

func (rt *Runtime) controls() WorldControlHost {
	host, _ := rt.Host.(WorldControlHost)
	return host
}

// registerWorldActionAPI defines the calls that move, look and target.
func registerWorldActionAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	push := func(L *lua.LState, values ...lua.LValue) int {
		for _, value := range values {
			L.Push(value)
		}
		return len(values)
	}
	move := func(command string, down bool) lua.LGFunction {
		return func(L *lua.LState) int {
			if host := rt.controls(); host != nil {
				host.Movement(command, down)
			}
			return 0
		}
	}
	for _, entry := range []struct{ name, command string }{
		{"MoveForward", "MOVEFORWARD"}, {"MoveBackward", "MOVEBACKWARD"}, {"TurnLeft", "TURNLEFT"}, {"TurnRight", "TURNRIGHT"},
		{"StrafeLeft", "STRAFELEFT"}, {"StrafeRight", "STRAFERIGHT"}, {"PitchUp", "PITCHUP"}, {"PitchDown", "PITCHDOWN"},
	} {
		reg(entry.name+"Start", move(entry.command, true))
		reg(entry.name+"Stop", move(entry.command, false))
	}
	reg("JumpOrAscendStart", move("JUMP", true))
	reg("AscendStop", move("JUMP", false))
	reg("SitStandOrDescendStart", move("SITORSTAND", true))
	reg("DescendStop", move("SITORSTAND", false))
	reg("MoveAndSteerStart", move("MOVEFORWARD", true))
	reg("MoveAndSteerStop", move("MOVEFORWARD", false))
	toggle := func(command string) lua.LGFunction {
		return func(L *lua.LState) int {
			if host := rt.controls(); host != nil {
				host.Movement(command, true)
				host.Movement(command, false)
			}
			return 0
		}
	}
	reg("ToggleAutoRun", toggle("TOGGLEAUTORUN"))
	reg("ToggleRun", toggle("TOGGLERUN"))
	mouse := func(button int, down bool) lua.LGFunction {
		return func(L *lua.LState) int {
			if host := rt.controls(); host != nil {
				host.MouseAction(button, down)
			}
			return 0
		}
	}
	reg("CameraOrSelectOrMoveStart", mouse(0, true))
	reg("CameraOrSelectOrMoveStop", mouse(0, false))
	reg("TurnOrActionStart", mouse(1, true))
	reg("TurnOrActionStop", mouse(1, false))

	// The camera.
	zoom := func(sign float64) lua.LGFunction {
		return func(L *lua.LState) int {
			if host := rt.controls(); host != nil {
				host.CameraZoom(sign * float64(L.OptNumber(1, 1)))
			}
			return 0
		}
	}
	reg("CameraZoomIn", zoom(1))
	reg("CameraZoomOut", zoom(-1))
	reg("VehicleCameraZoomIn", zoom(1))
	reg("VehicleCameraZoomOut", zoom(-1))
	reg("FlipCameraYaw", func(L *lua.LState) int {
		if host := rt.controls(); host != nil {
			host.CameraYaw(float64(L.CheckNumber(1)))
		}
		return 0
	})
	reg("SetView", func(L *lua.LState) int {
		view := L.CheckInt(1)
		if view < 1 || view > len(rt.views) {
			return 0
		}
		if host := rt.controls(); host != nil {
			rt.viewIndex = view - 1
			host.SetCameraView(rt.views[view-1])
		}
		return 0
	})
	reg("SaveView", func(L *lua.LState) int {
		view := L.CheckInt(1)
		if host := rt.controls(); host != nil && view >= 1 && view <= len(rt.views) {
			rt.views[view-1] = host.CameraDistance()
		}
		return 0
	})
	reg("ResetView", func(L *lua.LState) int {
		view := L.CheckInt(1)
		if view >= 1 && view <= len(rt.views) {
			rt.views[view-1] = defaultCameraViews[view-1]
		}
		return 0
	})
	cycleView := func(step int) lua.LGFunction {
		return func(L *lua.LState) int {
			rt.viewIndex = (rt.viewIndex + step + len(rt.views)) % len(rt.views)
			if host := rt.controls(); host != nil {
				host.SetCameraView(rt.views[rt.viewIndex])
			}
			return 0
		}
	}
	reg("NextView", cycleView(1))
	reg("PrevView", cycleView(-1))

	// Targets.
	nearest := func(kind string, reverse bool) lua.LGFunction {
		return func(L *lua.LState) int {
			if host := rt.controls(); host != nil {
				host.TargetNearest(kind, reverse || (L.Get(1) != lua.LNil && L.Get(1) != lua.LFalse))
			}
			return 0
		}
	}
	reg("TargetNearestEnemyPlayer", nearest("enemyplayer", false))
	reg("TargetNearestFriend", nearest("friend", false))
	reg("TargetNearestFriendPlayer", nearest("friendplayer", false))
	reg("TargetNearestPartyMember", nearest("party", false))
	reg("TargetNearestRaidMember", nearest("raid", false))
	last := func(kind string) lua.LGFunction {
		return func(L *lua.LState) int {
			if host := rt.controls(); host != nil {
				host.TargetLast(kind)
			}
			return 0
		}
	}
	reg("TargetLastTarget", last("target"))
	reg("TargetLastEnemy", last("enemy"))
	reg("TargetLastFriend", last("friend"))
	reg("AssistUnit", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, "target"))
		if info == nil || info.GUID == 0 {
			return 0
		}
		if host := rt.controls(); host != nil {
			if guid := host.TargetOf(info.GUID); guid != 0 {
				rt.send(clientSetSelection, bytesOfGUID(guid))
			}
		}
		return 0
	})
	reg("FocusUnit", func(L *lua.LState) int {
		unit := L.OptString(1, "target")
		if info := rt.unitInfo(unit); info != nil && info.Exists {
			copy := *info
			rt.SetUnit("focus", copy)
			rt.focusGUID = info.GUID
			rt.FireEvent("PLAYER_FOCUS_CHANGED")
		}
		return 0
	})
	reg("ClearFocus", func(L *lua.LState) int {
		if rt.unitInfo("focus") != nil {
			rt.ClearUnit("focus")
			rt.focusGUID = 0
			rt.FireEvent("PLAYER_FOCUS_CHANGED")
		}
		return 0
	})
	reg("FollowUnit", func(L *lua.LState) int {
		info := rt.unitInfo(L.CheckString(1))
		if info == nil || info.GUID == 0 {
			return 0
		}
		if host := rt.controls(); host != nil {
			host.Follow(info.GUID)
			rt.FireEvent("AUTOFOLLOW_BEGIN", lua.LString(info.Name))
		}
		return 0
	})
	reg("InteractUnit", func(L *lua.LState) int {
		info := rt.unitInfo(L.CheckString(1))
		if info == nil || info.GUID == 0 {
			return 0
		}
		if host := rt.controls(); host != nil {
			host.Interact(info.GUID)
		}
		return 0
	})
	reg("IsFalling", func(L *lua.LState) int {
		host := rt.controls()
		return push(L, luaFlag(host != nil && host.Falling()))
	})
	reg("GetFramerate", func(L *lua.LState) int {
		if host := rt.controls(); host != nil {
			return push(L, lua.LNumber(host.Framerate()))
		}
		return push(L, lua.LNumber(0))
	})
	held := func(mask uint8) lua.LGFunction {
		return func(L *lua.LState) int { return push(L, luaFlag(rt.mods&mask != 0)) }
	}
	reg("IsShiftKeyDown", held(modLeftShift|modRightShift))
	reg("IsLeftShiftKeyDown", held(modLeftShift))
	reg("IsRightShiftKeyDown", held(modRightShift))
	reg("IsControlKeyDown", held(modLeftCtrl|modRightCtrl))
	reg("IsLeftControlKeyDown", held(modLeftCtrl))
	reg("IsRightControlKeyDown", held(modRightCtrl))
	reg("IsAltKeyDown", held(modLeftAlt|modRightAlt))
	reg("IsLeftAltKeyDown", held(modLeftAlt))
	reg("IsRightAltKeyDown", held(modRightAlt))
	reg("IsModifierKeyDown", held(0x3F))
	reg("GetModifiedClick", func(L *lua.LState) int {
		if binding, ok := rt.modifiedClicks()[L.CheckString(1)]; ok {
			return push(L, lua.LString(binding))
		}
		return push(L, lua.LNil)
	})
	reg("SetModifiedClick", func(L *lua.LState) int {
		action, binding := L.CheckString(1), strings.ToUpper(L.CheckString(2))
		clicks := rt.modifiedClicks()
		if _, known := clicks[action]; !known || !validClickBinding(binding) {
			L.RaiseError("SetModifiedClick(): Unknown action (%s) or binding (%s)", action, binding)
			return 0
		}
		clicks[action] = binding
		return 0
	})
	reg("IsModifiedClick", func(L *lua.LState) int {
		binding, ok := rt.modifiedClicks()[L.OptString(1, "")]
		return push(L, luaFlag(ok && rt.clickBindingHeld(binding)))
	})
	reg("Dismount", func(L *lua.LState) int {
		if host, ok := rt.Host.(interface{ CancelMount() error }); ok {
			_ = host.CancelMount()
		}
		return 0
	})
	reg("SetOptOutOfLoot", func(L *lua.LState) int {
		optOut := byte(0)
		if L.Get(1) != lua.LNil && L.Get(1) != lua.LFalse && L.Get(1) != lua.LNumber(0) {
			optOut = 1
		}
		rt.send(clientOptOutOfLoot, []byte{optOut, 0, 0, 0})
		return 0
	})
	reg("SetPVP", func(L *lua.LState) int {
		rt.send(clientTogglePVP, nil)
		return 0
	})
	reg("TogglePVP", func(L *lua.LState) int {
		rt.send(clientTogglePVP, nil)
		return 0
	})
	reg("RunScript", func(L *lua.LState) int {
		rt.Execute(L.CheckString(1), "@RunScript")
		return 0
	})
	reg("ReloadUI", func(L *lua.LState) int {
		rt.FireEvent("PLAYER_LOGOUT")
		return 0
	})
	reg("GetRealmName", func(L *lua.LState) int { return push(L, lua.LString(rt.realmName)) })
	reg("GetRealZoneText", func(L *lua.LState) int {
		if catalog := rt.worldMap.catalog; catalog != nil && rt.worldMap.hasPlayer {
			for _, page := range catalog.Pages {
				if page.MapID == rt.worldMap.playerMap && page.AreaID == 0 {
					return push(L, lua.LString(catalog.MapNames[page.MapID]))
				}
			}
		}
		return push(L, lua.LString(rt.mapName(rt.worldMap.playerMap)))
	})
	reg("GetExpansionLevel", func(L *lua.LState) int { return push(L, lua.LNumber(2)) })
	reg("IsXPUserDisabled", func(L *lua.LState) int { return push(L, lua.LFalse) })
	reg("HasFullControl", func(L *lua.LState) int { return push(L, lua.LBool(!rt.controlLost)) })
	reg("IsOutOfBounds", func(L *lua.LState) int { return push(L, lua.LFalse) })
	reg("SecureCmdOptionParse", func(L *lua.LState) int {
		result, target := rt.parseCmdOptions(L.CheckString(1))
		if result == "" {
			return push(L, lua.LNil)
		}
		if target != "" {
			return push(L, lua.LString(result), lua.LString(target))
		}
		return push(L, lua.LString(result))
	})
}

// Client opcodes the calls above send.
const (
	clientSetSelection world.ClientOpcode = 0x013D
	clientOptOutOfLoot world.ClientOpcode = 0x0409
	clientTogglePVP    world.ClientOpcode = 0x0253
)

// bytesOfGUID is the body of the requests that carry one guid.
func bytesOfGUID(guid uint64) []byte { return world.BuildGuid(guid) }

// Modifier key bits, as the client keeps them: left and right shift,
// control and alt.
const (
	modLeftShift  = 1 << 0
	modRightShift = 1 << 1
	modLeftCtrl   = 1 << 2
	modRightCtrl  = 1 << 3
	modLeftAlt    = 1 << 4
	modRightAlt   = 1 << 5
)

// SetModifierState gives the interface which modifier keys are held.
func (rt *Runtime) SetModifierState(mask uint8) { rt.mods = mask }

// modifierDown reports whether SHIFT, CTRL or ALT is held.
func (rt *Runtime) modifierDown(name string) bool {
	switch strings.ToUpper(name) {
	case "SHIFT":
		return rt.mods&(modLeftShift|modRightShift) != 0
	case "CTRL":
		return rt.mods&(modLeftCtrl|modRightCtrl) != 0
	case "ALT":
		return rt.mods&(modLeftAlt|modRightAlt) != 0
	}
	return false
}

// FocusGUID is the guid of the unit that is the focus.
func (rt *Runtime) FocusGUID() uint64 { return rt.focusGUID }

// modifiedClicks are the bindings of the modified clicks: the action each one
// makes (a chat link, a split stack) and the keys that hold it.
func (rt *Runtime) modifiedClicks() map[string]string {
	if rt.modClicks == nil {
		rt.modClicks = make(map[string]string)
	}
	if len(rt.modClicks) == 0 && rt.bindings != nil {
		for action, binding := range rt.bindings.modified {
			rt.modClicks[action] = binding
		}
	}
	return rt.modClicks
}

// validClickBinding reports whether text names modifiers and maybe a mouse
// button ("SHIFT", "ALT-CTRL", "SHIFT-BUTTON1", "NONE").
func validClickBinding(text string) bool {
	if text == "NONE" {
		return true
	}
	for _, part := range strings.Split(text, "-") {
		switch {
		case part == "SHIFT", part == "CTRL", part == "ALT":
		case strings.HasPrefix(part, "BUTTON") && len(part) > 6:
		default:
			return false
		}
	}
	return true
}

// clickBindingHeld reports whether the keys of a click binding are held and, when
// it names a mouse button, that the click was made with it.
func (rt *Runtime) clickBindingHeld(binding string) bool {
	if binding == "" || binding == "NONE" {
		return false
	}
	for _, part := range strings.Split(binding, "-") {
		switch {
		case part == "SHIFT" || part == "CTRL" || part == "ALT":
			if !rt.modifierDown(part) {
				return false
			}
		case strings.HasPrefix(part, "BUTTON"):
			names := map[string]string{"BUTTON1": "LeftButton", "BUTTON2": "RightButton", "BUTTON3": "MiddleButton", "BUTTON4": "Button4", "BUTTON5": "Button5"}
			if rt.lastButton != names[part] {
				return false
			}
		}
	}
	return true
}
