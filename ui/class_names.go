package ui

import (
	"encoding/binary"
)

// chrClassNames reads the class names of ChrClasses.dbc, by the token the
// scripts know a class by: for men and for women.
func (rt *Runtime) chrClassNames() (male, female map[string]string) {
	male, female = map[string]string{}, map[string]string{}
	if rt.engine == nil || rt.engine.AssetLoader == nil {
		return
	}
	data, err := rt.engine.AssetLoader.ReadFile(`DBFilesClient\ChrClasses.dbc`)
	if err != nil || len(data) < 20 || string(data[:4]) != "WDBC" {
		return
	}
	records := int(binary.LittleEndian.Uint32(data[4:8]))
	fields := int(binary.LittleEndian.Uint32(data[8:12]))
	stride := int(binary.LittleEndian.Uint32(data[12:16]))
	stringSize := int(binary.LittleEndian.Uint32(data[16:20]))
	stringStart := 20 + records*stride
	if fields < 56 || stride < fields*4 || stringStart+stringSize > len(data) {
		return
	}
	text := func(offset uint32) string {
		start := int(offset)
		if start <= 0 || start >= stringSize {
			return ""
		}
		end := start
		for end < stringSize && data[stringStart+end] != 0 {
			end++
		}
		return string(data[stringStart+start : stringStart+end])
	}
	for record := 0; record < records; record++ {
		base := 20 + record*stride
		field := func(column int) uint32 { return binary.LittleEndian.Uint32(data[base+column*4:]) }
		token := text(field(55))
		if token == "" {
			continue
		}
		name := text(field(4))
		maleName, femaleName := text(field(38)), text(field(21))
		if maleName == "" {
			maleName = name
		}
		if femaleName == "" {
			femaleName = name
		}
		male[token], female[token] = maleName, femaleName
	}
	return
}
