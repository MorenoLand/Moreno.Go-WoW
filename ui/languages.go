package ui

import lua "github.com/yuin/gopher-lua"

// Language is a row of Languages.dbc.
type Language struct {
	ID   uint32
	Name string
}

// languageSkills is the skill line each language is learned through.
var languageSkills = map[uint32]uint32{
	1: 109, 2: 113, 3: 115, 6: 111, 7: 98, 8: 139, 9: 140, 10: 137, 11: 138, 12: 141, 13: 313, 14: 315, 33: 673, 35: 759,
}

// SetLanguages installs the languages of the client's tables.
func (rt *Runtime) SetLanguages(languages []Language) { rt.languages = languages }

// knownLanguages lists the languages the character has the skill of.
func (rt *Runtime) knownLanguages() []Language {
	var known []Language
	for _, language := range rt.languages {
		skill, ok := languageSkills[language.ID]
		if !ok {
			continue
		}
		for _, entry := range rt.skills.entries {
			if entry.ID == skill && entry.Rank > 0 {
				known = append(known, language)
				break
			}
		}
	}
	return known
}

func registerLanguageAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("GetNumLanguages", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.knownLanguages())))
		return 1
	}))
	L.SetGlobal("GetLanguageByIndex", L.NewFunction(func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetLanguageByIndex(index)")
		}
		known := rt.knownLanguages()
		if index := L.CheckInt(1); index >= 1 && index <= len(known) {
			L.Push(lua.LString(known[index-1].Name))
			return 1
		}
		return 0
	}))
}
