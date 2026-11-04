package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// bindingFile is the file of the key bindings of the account (set 1) or of the
// character (set 2).
func (rt *Runtime) bindingFile(set int) string {
	folder := rt.accountFolder()
	if set == 2 {
		folder = rt.characterFolder()
	}
	if folder == "" {
		return ""
	}
	return filepath.Join(folder, "bindings-cache.wtf")
}

// writeBindings keeps the keys of the bindings in the file of a set, and the
// file it replaces as a backup.
func (rt *Runtime) writeBindings(set int) {
	path := rt.bindingFile(set)
	if path == "" || rt.bindings == nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if old, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(strings.TrimSuffix(path, ".wtf")+".old", old, 0o644)
	}
	var out strings.Builder
	out.WriteString("BINDINGMODE 0\r\n")
	keys := make([]string, 0, len(rt.bindings.keys))
	for key := range rt.bindings.keys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if command := rt.bindings.keys[key]; command != "" {
			fmt.Fprintf(&out, "bind %s %s\r\n", key, command)
		}
	}
	actions := make([]string, 0, len(rt.bindings.modified))
	for action := range rt.bindings.modified {
		actions = append(actions, action)
	}
	sort.Strings(actions)
	for _, action := range actions {
		fmt.Fprintf(&out, "modifiedclick %s %s\r\n", action, rt.bindings.modified[action])
	}
	_ = os.WriteFile(path, []byte(out.String()), 0o644)
}

// readBindings takes the keys of a set from its file over the defaults.
func (rt *Runtime) readBindings(set int) bool {
	path := rt.bindingFile(set)
	if path == "" || rt.bindings == nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	keys := map[string]string{}
	modified := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		switch {
		case len(fields) >= 3 && fields[0] == "bind":
			keys[strings.ToUpper(fields[1])] = fields[2]
		case len(fields) >= 3 && fields[0] == "modifiedclick":
			modified[fields[1]] = strings.ToUpper(fields[2])
		}
	}
	rt.bindings.keys = keys
	for action, key := range modified {
		rt.bindings.modified[action] = key
	}
	return true
}

// loadBindingFiles takes the saved keys: the character's if it has any, the
// account's otherwise.
func (rt *Runtime) loadBindingFiles() {
	if !rt.readBindings(2) {
		rt.readBindings(1)
	}
	rt.FireEvent("UPDATE_BINDINGS")
}

func registerBindingFilesAPI(rt *Runtime) {
	rt.L.SetGlobal("SaveBindings", rt.L.NewFunction(func(L *lua.LState) int {
		set := L.OptInt(1, 1)
		if set == 1 || set == 2 {
			rt.writeBindings(set)
		}
		return 0
	}))
	rt.L.SetGlobal("LoadBindings", rt.L.NewFunction(func(L *lua.LState) int {
		set := L.OptInt(1, 1)
		if set == 1 || set == 2 {
			rt.readBindings(set)
			rt.FireEvent("UPDATE_BINDINGS")
		}
		return 0
	}))
}
