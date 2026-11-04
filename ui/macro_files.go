package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	macroIconFolder = `Interface\Icons\`
	// macroFirstID is the number the first macro of a file gets.
	macroFirstID = 0x1000000 + 1
)

// macroFile is the file of the macros of the account or of the character.
func (rt *Runtime) macroFile(character bool) string {
	folder := rt.accountFolder()
	if character {
		folder = rt.characterFolder()
	}
	if folder == "" {
		return ""
	}
	return filepath.Join(folder, "macros-cache.txt")
}

// readMacroFile reads the macros a file holds: a MACRO line with the number,
// the name in quotes and the icon, the lines of the text, then END.
func readMacroFile(path string) []Macro {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var macros []Macro
	var current *Macro
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case current == nil && strings.HasPrefix(line, "MACRO "):
			fields := strings.SplitN(strings.TrimPrefix(line, "MACRO "), " ", 2)
			if len(fields) < 2 {
				continue
			}
			rest := fields[1]
			open := strings.IndexByte(rest, '"')
			closing := strings.LastIndexByte(rest, '"')
			if open < 0 || closing <= open {
				continue
			}
			icon := strings.TrimSpace(rest[closing+1:])
			macros = append(macros, Macro{Name: rest[open+1 : closing], Icon: macroIconFolder + icon})
			current, lines = &macros[len(macros)-1], nil
		case current != nil && line == "END":
			current.Body = strings.Join(lines, "\n")
			current = nil
		case current != nil:
			lines = append(lines, line)
		}
	}
	return macros
}

// writeMacroFile writes macros the way the client keeps them, keeping the file
// it replaces as a backup.
func writeMacroFile(path string, macros []Macro) {
	if path == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if old, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(strings.TrimSuffix(path, ".txt")+".old", old, 0o644)
	}
	var out strings.Builder
	for index, macro := range macros {
		icon := strings.TrimPrefix(macro.Icon, macroIconFolder)
		fmt.Fprintf(&out, "MACRO %d \"%s\" %s\r\n", macroFirstID+index, macro.Name, icon)
		for _, line := range strings.Split(strings.ReplaceAll(macro.Body, "\r", ""), "\n") {
			if line != "" {
				out.WriteString(line + "\r\n")
			}
		}
		out.WriteString("END\r\n")
	}
	_ = os.WriteFile(path, []byte(out.String()), 0o644)
}
