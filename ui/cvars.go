package ui

import (
	"strconv"
	"strings"
)

// cvarMeta holds engine defaults and bounds confirmed from Wow.exe 12340
// (Ghidra RegisterCVar sites FUN_0076a630 / FUN_0078e400 / FUN_00401b60)
// plus GlueXML OptionsPanel fallback min/max when the client exposes them
// through GetCVarMin/GetCVarMax (farclip 177/1277 at DAT_009f5798/009f57d8).
type cvarMeta struct {
	Default string
	Min     string // empty => GetCVarMin returns nil
	Max     string
}

var optionsCVarMeta = map[string]cvarMeta{
	// Resolution / gx (FUN_0076a630)
	"gxvsync":              {Default: "1"},
	"gxtriplebuffer":       {Default: "0"},
	"gxcursor":             {Default: "1"},
	"gxfixlag":             {Default: "0"},
	"gxwindow":             {Default: "1"},
	"gxmaximize":           {Default: "0"},
	"windowresizelock":     {Default: "0"},
	"gxresolution":         {Default: "1024x768"},
	"gxrefresh":            {Default: "75"},
	"gxapi":                {Default: "D3D9"},
	"gxcolorbits":          {Default: "32"},
	"gxdepthbits":          {Default: "24"},
	"gxmultisample":        {Default: "1"},
	"gxmultisamplequality": {Default: "0.0"},
	"gxstereoenabled":      {Default: "0"},
	"gxstereoconvergence":  {Default: "1.0", Min: "0.2", Max: "50"},
	"gxstereoseparation":   {Default: "0", Min: "0", Max: "100"},
	"widescreen":           {Default: "1"},
	"videooptionsversion":  {Default: "0"},

	// Gamma (FUN_00401b60): Gamma default DAT_009e1340 "1.0", DesktopGamma "0"
	"gamma":        {Default: "1.0", Min: "-0.5", Max: "0.5"},
	"desktopgamma": {Default: "0"},

	// Effects (FUN_0078e400)
	"farclip":             {Default: "350", Min: "177", Max: "1277"},
	"nearclip":            {Default: "0.2"},
	"particledensity":     {Default: "1.0", Min: "0.1", Max: "1.0"},
	"environmentdetail":   {Default: "1.0", Min: "0.5", Max: "1.5"},
	"groundeffectdensity": {Default: "16", Min: "16", Max: "64"},
	"groundeffectdist":    {Default: "70.0", Min: "70", Max: "140"},
	"basemip":             {Default: "0", Min: "0", Max: "1"},
	"extshadowquality":    {Default: "0", Min: "0", Max: "4"},
	"specular":            {Default: "0"},
	"projectedtextures":   {Default: "0"},
	"mapshadows":          {Default: "1"},
	"shadowlevel":         {Default: "1"},
	"waterlod":            {Default: "0"},
	"spelleffectlevel":    {Default: "9"},
	"farclipoverride":     {Default: "0"},
	"occlusion":           {Default: "1"},
	"objectfade":          {Default: "1"},
	"hwpcf":               {Default: "1"},

	// Additional EffectsPanelOptions CVars (registered elsewhere / FrameXML)
	"texturefilteringmode":  {Default: "1", Min: "0", Max: "5"},
	"weatherdensity":        {Default: "2", Min: "0", Max: "3"},
	"componenttexturelevel": {Default: "8", Min: "8", Max: "9"},
	"ffxglow":               {Default: "1"},
	"ffxdeath":              {Default: "1"},

	// Sound (GlueXML SoundPanelOptions + existing audio host path)
	"sound_enableallsound":            {Default: "1"},
	"sound_enablemusic":               {Default: "1"},
	"sound_enablesfx":                 {Default: "1"},
	"sound_enableambience":            {Default: "1"},
	"sound_enableerrorspeech":         {Default: "1"},
	"sound_enableemotesounds":         {Default: "1"},
	"sound_enablepetsounds":           {Default: "1"},
	"sound_zonemusicnodelay":          {Default: "0"},
	"sound_enablesoundwhengameisinbg": {Default: "0"},
	"sound_enablereverb":              {Default: "0"},
	"sound_enablesoftwarehrtf":        {Default: "0"},
	"sound_enabledspeffects":          {Default: "1"},
	"sound_enablehardware":            {Default: "0"},
	"sound_mastervolume":              {Default: "1.0", Min: "0", Max: "1"},
	"sound_sfxvolume":                 {Default: "1.0", Min: "0", Max: "1"},
	"sound_musicvolume":               {Default: "0.4", Min: "0", Max: "1"},
	"sound_ambiencevolume":            {Default: "0.6", Min: "0", Max: "1"},
	"sound_numchannels":               {Default: "32", Min: "32", Max: "64"},
	"sound_outputquality":             {Default: "1", Min: "0", Max: "2"},
	"sound_outputdriverindex":         {Default: "0"},
	"sound_outputdrivername":          {Default: "System Default"},
	"sound_listeneratcharacter":       {Default: "1"},
}

func lookupCVarMeta(name string) (cvarMeta, bool) {
	meta, ok := optionsCVarMeta[strings.ToLower(name)]
	return meta, ok
}

func defaultCVarValue(name string) (string, bool) {
	if meta, ok := lookupCVarMeta(name); ok {
		return meta.Default, true
	}
	return "", false
}

func cvarMinValue(name string) (float64, bool) {
	meta, ok := lookupCVarMeta(name)
	if !ok || meta.Min == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(meta.Min, 64)
	return v, err == nil
}

func cvarMaxValue(name string) (float64, bool) {
	meta, ok := lookupCVarMeta(name)
	if !ok || meta.Max == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(meta.Max, 64)
	return v, err == nil
}

func seedOptionsCVarDefaults(rt *Runtime) {
	if rt == nil {
		return
	}
	for name, meta := range optionsCVarMeta {
		rt.SetCVarDefault(name, meta.Default)
		if _, ok := rt.GetCVar(name); !ok {
			rt.SetCVar(name, meta.Default)
		}
	}
}

func restoreVideoCVars(rt *Runtime, group string) {
	if rt == nil {
		return
	}
	names := videoCVarNames(group)
	for _, name := range names {
		if meta, ok := lookupCVarMeta(name); ok {
			setCVarValue(rt, name, meta.Default)
		}
	}
}

func videoCVarNames(group string) []string {
	switch strings.ToLower(group) {
	case "resolution":
		return []string{
			"gxVSync", "gxTripleBuffer", "gxCursor", "gxFixLag", "gxWindow", "gxMaximize",
			"windowResizeLock", "desktopGamma", "gamma", "gxResolution", "gxRefresh",
			"gxMultisample", "gxMultisampleQuality",
		}
	case "effects":
		return []string{
			"farclip", "particleDensity", "environmentDetail", "groundEffectDensity",
			"groundEffectDist", "BaseMip", "extShadowQuality", "textureFilteringMode",
			"weatherDensity", "componentTextureLevel", "specular", "ffxGlow", "ffxDeath",
			"projectedTextures",
		}
	case "stereo":
		return []string{"gxStereoEnabled", "gxStereoConvergence", "gxStereoSeparation", "gxCursor"}
	default:
		return nil
	}
}
