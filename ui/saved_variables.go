package ui

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// savedVariables keeps the variables the interface and the add-ons ask to have
// kept between runs, in the files of the WTF folder.
type savedVariables struct {
	root, account, realm, character string
	// blizzard and blizzardCharacter are the variables the interface scripts
	// registered, for the account and for the character.
	blizzard, blizzardCharacter []string
	// addons are the variables each add-on names in its table of contents.
	addons map[string]addonVariables
}

type addonVariables struct{ account, character []string }

// SetWTFRoot says where the WTF folder is.
func (rt *Runtime) SetWTFRoot(root string) { rt.saved.root = root }

// SetWTFAccount says which account the files belong to.
func (rt *Runtime) SetWTFAccount(account string) {
	rt.saved.account = strings.ToUpper(strings.TrimSpace(account))
}

// accountFolder is the folder of the account.
func (rt *Runtime) accountFolder() string {
	if rt.saved.root == "" || rt.saved.account == "" {
		return ""
	}
	return filepath.Join(rt.saved.root, "Account", rt.saved.account)
}

// characterFolder is the folder of the character.
func (rt *Runtime) characterFolder() string {
	account := rt.accountFolder()
	realm, name := rt.currentRealm(), rt.playerName()
	if account == "" || realm == "" || name == "" {
		return ""
	}
	return filepath.Join(account, realm, name)
}

// variableNameOK is whether a name can be written as a variable.
func variableNameOK(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_') {
			return false
		}
	}
	return true
}

func splitVariableNames(value string) []string {
	var names []string
	for _, name := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		if variableNameOK(name) {
			names = append(names, name)
		}
	}
	return names
}

// runVariableFile runs a file of saved variables, which assigns them.
func (rt *Runtime) runVariableFile(path string) {
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	rt.Execute(string(data), "@"+path)
}

// loadAddOnVariables names the variables of an add-on from its table of
// contents and loads the files that hold them.
func (rt *Runtime) loadAddOnVariables(addon string, toc []byte) {
	variables := addonVariables{
		account:   splitVariableNames(addonTOCField(toc, "SavedVariables")),
		character: splitVariableNames(addonTOCField(toc, "SavedVariablesPerCharacter")),
	}
	if len(variables.account) == 0 && len(variables.character) == 0 {
		return
	}
	if rt.saved.addons == nil {
		rt.saved.addons = make(map[string]addonVariables)
	}
	rt.saved.addons[addon] = variables
	if len(variables.account) > 0 {
		if folder := rt.accountFolder(); folder != "" {
			rt.runVariableFile(filepath.Join(folder, "SavedVariables", addon+".lua"))
		}
	}
	if len(variables.character) > 0 {
		if folder := rt.characterFolder(); folder != "" {
			rt.runVariableFile(filepath.Join(folder, "SavedVariables", addon+".lua"))
		}
	}
}

// loadBlizzardVariables loads what the interface scripts registered to keep.
func (rt *Runtime) loadBlizzardVariables() {
	if folder := rt.accountFolder(); folder != "" {
		rt.runVariableFile(filepath.Join(folder, "SavedVariables.lua"))
	}
	if folder := rt.characterFolder(); folder != "" {
		rt.runVariableFile(filepath.Join(folder, "SavedVariables.lua"))
	}
}

// saveVariables writes every kept variable to its file, keeping the file it
// replaces as a backup.
func (rt *Runtime) saveVariables() {
	rt.saveCVarFiles()
	rt.saveChatCache()
	rt.saveLayoutFile()
	write := func(path string, names []string) {
		if path == "" {
			return
		}
		if len(names) == 0 {
			_ = os.Remove(path)
			return
		}
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if old, err := os.ReadFile(path); err == nil {
			_ = os.WriteFile(path+".bak", old, 0o644)
		}
		var out strings.Builder
		for _, name := range names {
			out.WriteString("\r\n")
			out.WriteString(name)
			out.WriteString(" = ")
			writeLuaValue(&out, rt.L.GetGlobal(name), 0, map[*lua.LTable]bool{})
			out.WriteString("\r\n")
		}
		_ = os.WriteFile(path, []byte(out.String()), 0o644)
	}
	account, character := rt.accountFolder(), rt.characterFolder()
	if account != "" {
		write(filepath.Join(account, "SavedVariables.lua"), rt.saved.blizzard)
	}
	if character != "" {
		write(filepath.Join(character, "SavedVariables.lua"), rt.saved.blizzardCharacter)
	}
	names := make([]string, 0, len(rt.saved.addons))
	for addon := range rt.saved.addons {
		names = append(names, addon)
	}
	sort.Strings(names)
	for _, addon := range names {
		variables := rt.saved.addons[addon]
		if account != "" {
			write(filepath.Join(account, "SavedVariables", addon+".lua"), variables.account)
		}
		if character != "" {
			write(filepath.Join(character, "SavedVariables", addon+".lua"), variables.character)
		}
	}
}

// writeLuaValue writes a value the way the client keeps variables: tables with
// the numbered entries first, each marked with its number, then the others.
func writeLuaValue(out *strings.Builder, value lua.LValue, depth int, seen map[*lua.LTable]bool) {
	switch v := value.(type) {
	case *lua.LNilType:
		out.WriteString("nil")
	case lua.LBool:
		if v {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case lua.LNumber:
		number := float64(v)
		switch {
		case math.IsNaN(number):
			out.WriteString("nil --[[ nan ]]")
		case math.IsInf(number, 0):
			out.WriteString("nil --[[ inf ]]")
		default:
			out.WriteString(strconv.FormatFloat(number, 'g', 16, 64))
		}
	case lua.LString:
		writeLuaString(out, string(v))
	case *lua.LTable:
		if seen[v] {
			out.WriteString("nil --[[ skipped recursive table ]]")
			return
		}
		seen[v] = true
		indent := strings.Repeat("\t", depth)
		inner := strings.Repeat("\t", depth+1)
		out.WriteString("{\r\n")
		length := v.Len()
		for index := 1; index <= length; index++ {
			out.WriteString(inner)
			writeLuaValue(out, v.RawGetInt(index), depth+1, seen)
			fmt.Fprintf(out, ", -- [%d]\r\n", index)
		}
		type entry struct{ key, value lua.LValue }
		var entries []entry
		v.ForEach(func(key, item lua.LValue) {
			if number, ok := key.(lua.LNumber); ok {
				whole := float64(int(number)) == float64(number)
				if whole && int(number) >= 1 && int(number) <= length {
					return
				}
			}
			entries = append(entries, entry{key, item})
		})
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].key.String() < entries[j].key.String() })
		for _, item := range entries {
			switch item.key.(type) {
			case lua.LString, lua.LBool, lua.LNumber:
				out.WriteString(inner + "[")
				writeLuaValue(out, item.key, depth+1, seen)
				out.WriteString("] = ")
				writeLuaValue(out, item.value, depth+1, seen)
				out.WriteString(",\r\n")
			default:
				out.WriteString(inner + "--[[ skipped entry with " + item.key.Type().String() + " key ]]\r\n")
			}
		}
		out.WriteString(indent + "}")
		delete(seen, v)
	case *lua.LFunction:
		out.WriteString("nil --[[ skipped inline function ]]")
	default:
		out.WriteString("nil --[[ skipped " + value.Type().String() + " ]]")
	}
}

func writeLuaString(out *strings.Builder, text string) {
	out.WriteByte('"')
	for index := 0; index < len(text); index++ {
		switch c := text[index]; c {
		case 0:
			out.WriteString("\\000")
		case '\n':
			out.WriteString("\\n")
		case '\r':
			out.WriteString("\\r")
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteByte(c)
		default:
			out.WriteByte(c)
		}
	}
	out.WriteByte('"')
}

func registerSavedVariablesAPI(rt *Runtime) {
	L := rt.L
	register := func(name string, character bool) {
		L.SetGlobal(name, L.NewFunction(func(L *lua.LState) int {
			if L.Get(1).Type() != lua.LTString {
				L.RaiseError("Usage: %s(\"variable\")", name)
				return 0
			}
			variable := L.CheckString(1)
			if !variableNameOK(variable) {
				return 0
			}
			list := &rt.saved.blizzard
			if character {
				list = &rt.saved.blizzardCharacter
			}
			for _, kept := range *list {
				if kept == variable {
					return 0
				}
			}
			*list = append(*list, variable)
			return 0
		}))
	}
	register("RegisterForSave", false)
	register("RegisterForSavePerCharacter", true)
}

// currentRealm is the name of the realm the character plays on.
func (rt *Runtime) currentRealm() string {
	if rt.realmName != "" {
		return rt.realmName
	}
	return rt.Glue.ServerName
}
