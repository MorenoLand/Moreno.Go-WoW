package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// gameError is an error of the client's numbered list: the global string that
// words it, where it shows (0 a chat line, 1 the info display, 2 the error
// display), the kind of chat line and the sound it plays.
type gameError struct {
	name  string
	kind  int
	chat  int
	sound int
}

var (
	gameErrorOnce   sync.Once
	gameErrorByName map[string]int
)

func gameErrorIndex(name string) (int, bool) {
	gameErrorOnce.Do(func() {
		gameErrorByName = make(map[string]int, len(gameErrors))
		for index, entry := range gameErrors {
			if entry.name != "" {
				if _, taken := gameErrorByName[entry.name]; !taken {
					gameErrorByName[entry.name] = index
				}
			}
		}
	})
	index, ok := gameErrorByName[name]
	return index, ok
}

// ShowGameError raises the numbered error of the client with the values its
// text takes.
func (rt *Runtime) ShowGameError(index int, args ...any) {
	if index < 0 || index >= len(gameErrors) {
		return
	}
	entry := gameErrors[index]
	if entry.name == "" {
		return
	}
	text, ok := rt.L.GetGlobal(entry.name).(lua.LString)
	if !ok || text == "" {
		return
	}
	line := string(text)
	if len(args) > 0 && strings.Contains(line, "%") {
		line = fmt.Sprintf(line, args...)
	}
	switch entry.kind {
	case 0:
		rt.ChatLine(world.ChatType(entry.chat).EventName(), line)
	case 1:
		rt.FireEvent("UI_INFO_MESSAGE", lua.LString(line))
	default:
		rt.FireEvent("UI_ERROR_MESSAGE", lua.LString(line))
	}
}
