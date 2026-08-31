package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func discoverAddOns(dataPath string) []AddonInfo {
	roots := []string{filepath.Join(dataPath, "Interface", "AddOns"), filepath.Join(filepath.Dir(dataPath), "Interface", "AddOns"), filepath.Join(dataPath, "enUS", "Interface", "AddOns")}
	seen := make(map[string]bool)
	var addons []AddonInfo
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || seen[strings.ToLower(entry.Name())] {
				continue
			}
			tocPath := ""
			files, err := os.ReadDir(filepath.Join(root, entry.Name()))
			if err != nil {
				continue
			}
			for _, file := range files {
				if !file.IsDir() && strings.EqualFold(filepath.Ext(file.Name()), ".toc") {
					tocPath = filepath.Join(root, entry.Name(), file.Name())
					break
				}
			}
			if tocPath == "" {
				continue
			}
			data, err := os.ReadFile(tocPath)
			if err != nil {
				continue
			}
			addon := AddonInfo{Name: entry.Name(), Title: entry.Name(), Loadable: true, Security: "INSECURE", Enabled: true}
			interfaceVersion := 0
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
				if !strings.HasPrefix(line, "##") {
					continue
				}
				separator := strings.IndexByte(line, ':')
				if separator < 0 {
					continue
				}
				key := strings.ToLower(strings.TrimSpace(line[2:separator]))
				value := strings.TrimSpace(line[separator+1:])
				switch key {
				case "title":
					if value != "" {
						addon.Title = value
					}
				case "notes":
					addon.Notes = value
				case "url", "x-website":
					addon.URL = value
				case "interface":
					if fields := strings.Fields(value); len(fields) > 0 {
						interfaceVersion, _ = strconv.Atoi(fields[0])
					}
				}
			}
			if interfaceVersion != 0 && interfaceVersion != buildTOC {
				addon.Loadable = false
				addon.Reason = "INTERFACE_VERSION"
			}
			addons = append(addons, addon)
			seen[strings.ToLower(entry.Name())] = true
		}
	}
	return addons
}
