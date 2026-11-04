package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	chatCacheVersion      = 5
	chatCacheAddedVersion = 13
)

// chatCacheFile is the file the chat windows and chat colours of the
// character are kept in.
func (rt *Runtime) chatCacheFile() string {
	folder := rt.characterFolder()
	if folder == "" {
		return ""
	}
	return filepath.Join(folder, "chat-cache.txt")
}

// windowZoneMask is the bit mask of the zone channels a window shows.
func windowZoneMask(w *chatWindow) uint32 {
	var mask uint32
	for _, channel := range w.channels {
		if channel.zone >= 1 && channel.zone <= 32 {
			mask |= 1 << (channel.zone - 1)
		}
	}
	return mask
}

// saveChatCache writes the chat windows and colours, keeping the file it
// replaces as a backup.
func (rt *Runtime) saveChatCache() {
	rt.writeChatCache(rt.chatCacheFile())
}

func (rt *Runtime) writeChatCache(path string) {
	if path == "" || !rt.chatCfg.ready {
		return
	}
	var out strings.Builder
	fmt.Fprintf(&out, "VERSION %d\n\n", chatCacheVersion)
	fmt.Fprintf(&out, "ADDEDVERSION %d\n\n", chatCacheAddedVersion)
	out.WriteString("CHANNELS\nEND\n\n")
	var all uint32
	for index := range rt.chatCfg.windows {
		all |= windowZoneMask(&rt.chatCfg.windows[index])
	}
	fmt.Fprintf(&out, "ZONECHANNELS %d\n\n", all)
	out.WriteString("COLORS\n")
	flag := func(on bool) string {
		if on {
			return "Y"
		}
		return "N"
	}
	for _, entry := range rt.chatCfg.colors {
		fmt.Fprintf(&out, "%s %d %d %d %s\n", entry.name, entry.r, entry.g, entry.b, flag(rt.chatCfg.byClass[entry.name]))
	}
	for _, entry := range rt.chatCfg.channelColors {
		fmt.Fprintf(&out, "%s %d %d %d %s\n", entry.name, entry.r, entry.g, entry.b, flag(rt.chatCfg.byClass[entry.name]))
	}
	out.WriteString("END\n\n")
	for index := range rt.chatCfg.windows {
		w := &rt.chatCfg.windows[index]
		fmt.Fprintf(&out, "WINDOW %d\n", index+1)
		if w.name != "" {
			fmt.Fprintf(&out, "NAME %s\n", w.name)
		}
		fmt.Fprintf(&out, "SIZE %d\n", w.size)
		fmt.Fprintf(&out, "COLOR %d %d %d %d\n", w.r, w.g, w.b, w.a)
		fmt.Fprintf(&out, "LOCKED %d\n", boolInt(w.locked))
		fmt.Fprintf(&out, "UNINTERACTABLE %d\n", boolInt(w.uninteractable != 0))
		fmt.Fprintf(&out, "DOCKED %d\n", w.docked)
		fmt.Fprintf(&out, "SHOWN %d\n", boolInt(w.shown))
		if w.hasPosition {
			fmt.Fprintf(&out, "POSITION %s %f %f\n", w.point, w.x, w.y)
			fmt.Fprintf(&out, "DIMENSIONS %f %f\n\n", w.width, w.height)
		}
		out.WriteString("MESSAGES\n")
		for group, on := range w.messages {
			if on {
				out.WriteString(chatMessageGroups[group].name + "\n")
			}
		}
		out.WriteString("END\n\n")
		out.WriteString("CHANNELS\n")
		for _, channel := range w.channels {
			if channel.zone == 0 {
				out.WriteString(channel.name + "\n")
			}
		}
		out.WriteString("END\n\n")
		fmt.Fprintf(&out, "ZONECHANNELS %d\n\n", windowZoneMask(w))
		out.WriteString("END\n\n")
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if old, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(path+".old", old, 0o644)
	}
	_ = os.WriteFile(path, []byte(out.String()), 0o644)
}

// loadChatCache takes the chat windows and colours kept for the character;
// the defaults stay when there is no file.
func (rt *Runtime) loadChatCache() {
	rt.readChatCache(rt.chatCacheFile())
}

func (rt *Runtime) readChatCache(path string) {
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if !rt.chatCfg.ready {
		rt.resetChatWindows()
	}
	settings := &rt.chatCfg
	const (
		stateTop = iota
		stateChannels
		stateColors
		stateWindow
		stateWindowChannels
		stateWindowMessages
	)
	state, current := stateTop, -1
	seenMessages := map[int]bool{}
	seenChannels := map[int]bool{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		key := strings.ToUpper(fields[0])
		if state == stateTop || state == stateWindow {
			switch {
			case key == "CHANNELS" && state == stateTop:
				state = stateChannels
				continue
			case key == "COLORS" && state == stateTop:
				state = stateColors
				continue
			case key == "WINDOW" && len(fields) > 1:
				if number, _ := strconv.Atoi(fields[1]); number >= 1 && number <= chatWindowCount {
					state, current = stateWindow, number-1
				}
				continue
			}
		}
		if state == stateWindow && current >= 0 {
			w := &settings.windows[current]
			switch key {
			case "CHANNELS":
				if !seenChannels[current] {
					seenChannels[current] = true
					w.channels = w.channels[:0]
				}
				state = stateWindowChannels
			case "MESSAGES":
				if !seenMessages[current] {
					seenMessages[current] = true
					w.messages = [len(chatMessageGroups)]bool{}
				}
				state = stateWindowMessages
			case "NAME":
				w.name = strings.TrimSpace(line[len(fields[0]):])
			case "SIZE":
				w.size = atoiField(fields, 1)
			case "COLOR":
				w.r, w.g, w.b, w.a = byte(atoiField(fields, 1)), byte(atoiField(fields, 2)), byte(atoiField(fields, 3)), byte(atoiField(fields, 4))
			case "LOCKED":
				w.locked = atoiField(fields, 1) != 0
			case "UNINTERACTABLE":
				w.uninteractable = boolInt(atoiField(fields, 1) != 0)
			case "DOCKED":
				w.docked = atoiField(fields, 1)
			case "SHOWN":
				w.shown = atoiField(fields, 1) != 0
			case "POSITION":
				if len(fields) >= 4 {
					x, _ := strconv.ParseFloat(fields[2], 32)
					y, _ := strconv.ParseFloat(fields[3], 32)
					w.point, w.x, w.y, w.hasPosition = strings.ToUpper(fields[1]), float32(x), float32(y), true
				}
			case "DIMENSIONS":
				if len(fields) >= 3 {
					width, _ := strconv.ParseFloat(fields[1], 32)
					height, _ := strconv.ParseFloat(fields[2], 32)
					w.width, w.height = float32(width), float32(height)
				}
			case "ZONECHANNELS":
				mask := uint32(atoiField(fields, 1))
				for _, row := range rt.channels.rows {
					if row.ID >= 1 && row.ID <= 32 && mask&(1<<(row.ID-1)) != 0 {
						rt.addWindowChannel(current, row.Shortcut, row.ID)
					}
				}
			case "END":
				state = stateTop
			}
			continue
		}
		if key == "END" {
			switch state {
			case stateWindowChannels, stateWindowMessages:
				state = stateWindow
			default:
				state = stateTop
			}
			continue
		}
		switch state {
		case stateColors:
			if len(fields) < 4 {
				continue
			}
			r, g, b := byte(atoiField(fields, 1)), byte(atoiField(fields, 2)), byte(atoiField(fields, 3))
			for index := range settings.colors {
				if strings.EqualFold(settings.colors[index].name, fields[0]) {
					settings.colors[index].r, settings.colors[index].g, settings.colors[index].b = r, g, b
					rt.setChatByClass(settings.colors[index].name, fields)
				}
			}
			for index := range settings.channelColors {
				if strings.EqualFold(settings.channelColors[index].name, fields[0]) {
					settings.channelColors[index].r, settings.channelColors[index].g, settings.channelColors[index].b = r, g, b
					rt.setChatByClass(settings.channelColors[index].name, fields)
				}
			}
		case stateWindowMessages:
			for group, entry := range chatMessageGroups {
				if strings.EqualFold(entry.name, fields[0]) {
					settings.windows[current].messages[group] = true
				}
			}
		case stateWindowChannels:
			rt.addWindowChannel(current, line, 0)
		}
	}
}

// setChatByClass keeps the Y or N the colour line ends with.
func (rt *Runtime) setChatByClass(name string, fields []string) {
	if len(fields) < 5 || (fields[4] != "Y" && fields[4] != "N") {
		return
	}
	if rt.chatCfg.byClass == nil {
		rt.chatCfg.byClass = map[string]bool{}
	}
	rt.chatCfg.byClass[name] = fields[4] == "Y"
}

func atoiField(fields []string, index int) int {
	if index >= len(fields) {
		return 0
	}
	number, _ := strconv.Atoi(fields[index])
	return number
}
