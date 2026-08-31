package render

import (
	"encoding/binary"

	"github.com/MorenoLand/Moreno.WoW/ui"
)

func loadAreaNames(loader *ui.Loader) map[uint32]string {
	if loader == nil {
		return nil
	}
	data, err := loader.ReadFile(`DBFilesClient\AreaTable.dbc`)
	if err != nil {
		return nil
	}
	return parseAreaNames(data)
}

func parseAreaNames(data []byte) map[uint32]string {
	result := make(map[uint32]string)
	if len(data) < 20 || string(data[:4]) != "WDBC" {
		return result
	}
	records := int(binary.LittleEndian.Uint32(data[4:8]))
	fields := int(binary.LittleEndian.Uint32(data[8:12]))
	stride := int(binary.LittleEndian.Uint32(data[12:16]))
	stringSize := int(binary.LittleEndian.Uint32(data[16:20]))
	if records < 0 || fields <= 11 || stride < fields*4 || stringSize < 1 || 20+records*stride < 20 || 20+records*stride+stringSize > len(data) {
		return result
	}
	stringStart := 20 + records*stride
	readString := func(offset uint32) string {
		start := int(offset)
		if start < 0 || start >= stringSize {
			return ""
		}
		end := start
		for end < stringSize && data[stringStart+end] != 0 {
			end++
		}
		if end == start {
			return ""
		}
		return string(data[stringStart+start : stringStart+end])
	}
	for record := 0; record < records; record++ {
		base := 20 + record*stride
		id := binary.LittleEndian.Uint32(data[base : base+4])
		if id == 0 {
			continue
		}
		if name := readString(binary.LittleEndian.Uint32(data[base+11*4 : base+12*4])); name != "" {
			result[id] = name
		}
	}
	return result
}
