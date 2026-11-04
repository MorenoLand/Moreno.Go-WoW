package ui

import (
	"os"
	"path/filepath"
	"strings"
)

// savedCVar is a variable the client keeps between runs.
type savedCVar struct {
	name    string
	kind    uint32
	initial string
}

const (
	cvarKindAccount   = 0x10
	cvarKindCharacter = 0x20
)

// cvarFile is the file of the variables of the account or of the character.
func (rt *Runtime) cvarFile(kind uint32) string {
	folder := rt.accountFolder()
	if kind == cvarKindCharacter {
		folder = rt.characterFolder()
	}
	if folder == "" {
		return ""
	}
	return filepath.Join(folder, "config-cache.wtf")
}

// loadCVarFiles takes the variables kept for the account and the character.
func (rt *Runtime) loadCVarFiles() {
	for _, kind := range []uint32{cvarKindAccount, cvarKindCharacter} {
		path := rt.cvarFile(kind)
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "SET ") {
				continue
			}
			rest := strings.TrimSpace(line[4:])
			space := strings.IndexByte(rest, ' ')
			if space < 0 {
				continue
			}
			rt.SetCVar(rest[:space], strings.Trim(strings.TrimSpace(rest[space+1:]), `"`))
		}
	}
}

// saveCVarFiles writes the variables that are not what they started as, keeping
// the file it replaces as a backup.
func (rt *Runtime) saveCVarFiles() {
	for _, kind := range []uint32{cvarKindAccount, cvarKindCharacter} {
		path := rt.cvarFile(kind)
		if path == "" {
			continue
		}
		var out strings.Builder
		for _, saved := range savedCVars {
			if saved.kind != kind {
				continue
			}
			if value, ok := rt.GetCVar(saved.name); ok && !strings.EqualFold(value, saved.initial) {
				out.WriteString("SET " + saved.name + " \"" + value + "\"\n")
			}
		}
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if old, err := os.ReadFile(path); err == nil {
			_ = os.WriteFile(strings.TrimSuffix(path, ".wtf")+".old", old, 0o644)
		}
		_ = os.WriteFile(path, []byte(out.String()), 0o644)
	}
}
