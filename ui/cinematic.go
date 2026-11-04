package ui

import (
	"time"

	lua "github.com/yuin/gopher-lua"
)

// cinematicStopDelay is how long the client waits after StopCinematic before it
// says the cinematic is over.
const cinematicStopDelay = 250 * time.Millisecond

// registerCinematicAPI defines StopCinematic, which ends the cinematic after a
// short delay, and the calls the movie code makes when nothing is playing.
func registerCinematicAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("StopCinematic", L.NewFunction(func(L *lua.LState) int {
		rt.cinematicStopAt = time.Now().Add(cinematicStopDelay)
		return 0
	}))
	noop := L.NewFunction(func(L *lua.LState) int { return 0 })
	L.SetGlobal("GameMovieFinished", noop)
	L.SetGlobal("DetectWowMouse", noop)
}

// tickCinematic raises CINEMATIC_STOP once the delay after StopCinematic is up.
func (rt *Runtime) tickCinematic() {
	if rt.cinematicStopAt.IsZero() || time.Now().Before(rt.cinematicStopAt) {
		return
	}
	rt.cinematicStopAt = time.Time{}
	rt.FireEvent("CINEMATIC_STOP")
}
