package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// EmoteRow is a row of EmotesText.dbc: a text emote, the animation it plays and
// the lines said for each way it can be aimed.
type EmoteRow struct {
	ID    uint32
	Token string
	Emote uint32
	Texts [16]uint32
}

// emoteState holds the emote tables and the text emotes waiting for a name.
type emoteState struct {
	rows    []EmoteRow
	texts   map[uint32]string
	pending []pendingEmote
}

type pendingEmote struct {
	emote world.TextEmote
	since time.Time
}

// SetEmoteTables gives the interface EmotesText.dbc and EmotesTextData.dbc.
func (rt *Runtime) SetEmoteTables(rows []EmoteRow, texts map[uint32]string) {
	rt.emotes.rows, rt.emotes.texts = rows, texts
}

func (rt *Runtime) emoteRow(id uint32) *EmoteRow {
	for index := range rt.emotes.rows {
		if rt.emotes.rows[index].ID == id {
			return &rt.emotes.rows[index]
		}
	}
	return nil
}

// emoteLine is the line of a text emote for the way it was aimed: the bit 1
// says the doer is the character, bit 0 that the target is, bit 2 that there is
// no target, and the female lines sit eight places on.
func (rt *Runtime) emoteLine(row *EmoteRow, female bool, variant uint32) string {
	pick := func(index uint32) string {
		if index >= 16 {
			return ""
		}
		return rt.emotes.texts[row.Texts[index]]
	}
	base := uint32(0)
	if female {
		base = 8
	}
	if text := pick(base | variant); text != "" {
		return text
	}
	if text := pick(variant); text != "" {
		return text
	}
	variant = variant&^1 | 4
	if text := pick(base | variant); text != "" {
		return text
	}
	return pick(variant)
}

// showTextEmote writes the line of a text emote to the chat windows.
func (rt *Runtime) showTextEmote(emote world.TextEmote, name string) {
	row := rt.emoteRow(emote.Emote)
	if row == nil {
		return
	}
	player := rt.unitInfo("player")
	self := player != nil && player.GUID == emote.Sender
	targetSelf := player != nil && emote.Target != "" && strings.EqualFold(emote.Target, player.Name)
	variant := uint32(0)
	if self {
		variant = 2
	} else if targetSelf {
		variant = 1
	}
	if emote.Target == "" {
		variant |= 4
	}
	female := false
	if info := rt.unitByGUID(emote.Sender); info != nil {
		female = info.Sex == 3
	}
	line := rt.emoteLine(row, female, variant)
	if line == "" {
		return
	}
	switch {
	case !self && !targetSelf:
		line = fmt.Sprintf(line, name, emote.Target)
	case !self:
		line = fmt.Sprintf(line, name)
	case !targetSelf:
		line = fmt.Sprintf(line, emote.Target)
	default:
		line = strings.ReplaceAll(line, "%s", "")
	}
	rt.FireEvent("CHAT_MSG_TEXT_EMOTE",
		lua.LString(line), lua.LString(name), lua.LString(""), lua.LString(""), lua.LString(""), lua.LString(""),
		lua.LNumber(0), lua.LNumber(0), lua.LString(""), lua.LNumber(0), lua.LNumber(0), guidText(emote.Sender), lua.LNumber(0))
}

// unitByGUID finds a unit the interface keeps by its guid.
func (rt *Runtime) unitByGUID(guid uint64) *UnitInfo {
	for _, info := range rt.units {
		if info != nil && info.Exists && info.GUID == guid {
			return info
		}
	}
	return nil
}

// receiveTextEmote handles SMSG_TEXT_EMOTE, holding it until the name of the
// player who did it is known.
func (rt *Runtime) receiveTextEmote(emote world.TextEmote) {
	if name := rt.emoterName(emote.Sender); name != "" {
		rt.showTextEmote(emote, name)
		return
	}
	rt.emotes.pending = append(rt.emotes.pending, pendingEmote{emote: emote, since: time.Now()})
	if rt.chat.requestName != nil {
		rt.chat.requestName(emote.Sender)
	}
}

func (rt *Runtime) emoterName(guid uint64) string {
	if name := rt.nameOf(guid); name != "" {
		return name
	}
	if info := rt.unitByGUID(guid); info != nil {
		return info.Name
	}
	return ""
}

// flushEmotes shows the text emotes whose doers have a name now.
func (rt *Runtime) flushEmotes() {
	if len(rt.emotes.pending) == 0 {
		return
	}
	waiting := rt.emotes.pending[:0:0]
	for _, entry := range rt.emotes.pending {
		if name := rt.emoterName(entry.emote.Sender); name != "" {
			rt.showTextEmote(entry.emote, name)
		} else if time.Since(entry.since) < 5*time.Second {
			waiting = append(waiting, entry)
		}
	}
	rt.emotes.pending = waiting
}

// registerEmoteAPI defines DoEmote.
func registerEmoteAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("DoEmote", L.NewFunction(func(L *lua.LState) int {
		token := L.CheckString(1)
		var row *EmoteRow
		for index := range rt.emotes.rows {
			if strings.EqualFold(rt.emotes.rows[index].Token, token) {
				row = &rt.emotes.rows[index]
			}
		}
		if row == nil {
			return 0
		}
		unit := "target"
		if L.GetTop() >= 2 && L.Get(2).Type() == lua.LTString {
			if name := L.Get(2).String(); name != "" && !strings.HasPrefix(name, "%") {
				unit = name
			}
		}
		target := uint64(0)
		if info := rt.unitByNameOrToken(unit); info != nil {
			target = info.GUID
		}
		if player := rt.unitInfo("player"); player != nil && target == player.GUID {
			target = 0
		}
		rt.send(world.ClientTextEmote, world.BuildTextEmote(row.ID, 0xFFFFFFFF, target))
		return 0
	}))
}
