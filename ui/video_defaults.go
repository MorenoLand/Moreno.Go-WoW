package ui

import lua "github.com/yuin/gopher-lua"

// videoDefaultCVars are the variables each of the video defaults buttons puts
// back: the screen setup, the quality of the effects and the stereo view.
var videoDefaultCVars = map[string][]string{
	"resolution": {
		"gxVSync", "gxTripleBuffer", "gxFixLag", "gxWindow", "gxMaximize",
		"gxResolution", "gxRefresh", "gxMultisample", "gxMultisampleQuality",
		"desktopGamma", "gamma", "windowResizeLock",
	},
	"effects": {
		"farclip", "shadowLevel", "MaxLights", "specular", "waterLOD",
		"particleDensity", "baseMip", "groundEffectDensity",
		"extShadowQuality", "ffxDeath", "ffxGlow",
	},
	"stereo": {"gxStereoEnabled", "gxStereoConvergence", "gxStereoSeparation"},
}

// registerVideoDefaultsAPI defines the calls behind the defaults buttons of the
// video options and the restart of the graphics.
func registerVideoDefaultsAPI(rt *Runtime) {
	L := rt.L
	reg := func(name, group string) {
		L.SetGlobal(name, L.NewFunction(func(L *lua.LState) int {
			for _, name := range videoDefaultCVars[group] {
				if meta, ok := lookupCVarMeta(name); ok {
					setCVarValue(rt, name, meta.Default)
				}
			}
			return 0
		}))
	}
	reg("RestoreVideoResolutionDefaults", "resolution")
	reg("RestoreVideoEffectsDefaults", "effects")
	reg("RestoreVideoStereoDefaults", "stereo")
	L.SetGlobal("RestartGx", L.NewFunction(func(L *lua.LState) int {
		if rt.Host != nil {
			rt.Host.ConsoleExec("gxRestart")
		}
		return 0
	}))
}
