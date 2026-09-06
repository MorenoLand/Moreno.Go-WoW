package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RealmlistPath returns the native loose-file path Data\<locale>\realmlist.wtf.
// dataPath is the client's Data directory (as used by Moreno and WOW_TEST_DATA).
func RealmlistPath(dataPath, locale string) string {
	dataPath = strings.TrimSpace(dataPath)
	locale = strings.TrimSpace(locale)
	if dataPath == "" || locale == "" {
		return ""
	}
	return filepath.Join(dataPath, locale, "realmlist.wtf")
}

// ConfigWTFPath returns <install>\WTF\Config.wtf when dataPath is the Data directory.
func ConfigWTFPath(dataPath string) string {
	dataPath = strings.TrimSpace(dataPath)
	if dataPath == "" {
		return ""
	}
	install := filepath.Dir(dataPath)
	if install == "" || install == "." || install == string(filepath.Separator) {
		return ""
	}
	return filepath.Join(install, "WTF", "Config.wtf")
}

// ParseRealmlistWTF parses native realmlist.wtf console lines.
// Supported forms match the original client file:
//
//	set realmlist <host>
//	SET realmlist <host>
//	set realmlist "host"
//
// The last valid set realmlist wins. Empty lines and # or // comments are ignored.
// Returns ("", false, nil) when no directive is present.
func ParseRealmlistWTF(content string) (string, bool, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	found := false
	host := ""
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		lower := strings.ToLower(line)
		const prefix = "set realmlist"
		if !strings.HasPrefix(lower, prefix) {
			continue
		}
		rest := strings.TrimSpace(line[len(prefix):])
		host = unquoteWTFToken(rest)
		if host == "" {
			return "", false, fmt.Errorf("realmlist.wtf:%d: set realmlist missing host", lineNo)
		}
		found = true
	}
	if err := scanner.Err(); err != nil {
		return "", false, err
	}
	return host, found, nil
}

// ParseConfigWTFRealmList reads SET realmList "host" directives from Config.wtf.
func ParseConfigWTFRealmList(content string) (string, bool, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	found := false
	host := ""
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		lower := strings.ToLower(line)
		if !strings.HasPrefix(lower, "set ") {
			continue
		}
		rest := strings.TrimSpace(line[4:])
		nameEnd := strings.IndexAny(rest, " \t")
		if nameEnd < 0 {
			continue
		}
		name := rest[:nameEnd]
		if !strings.EqualFold(name, "realmList") && !strings.EqualFold(name, "realmlist") {
			continue
		}
		value := strings.TrimSpace(rest[nameEnd+1:])
		host = unquoteWTFToken(value)
		if host == "" {
			return "", false, fmt.Errorf("Config.wtf:%d: realmList missing value", lineNo)
		}
		found = true
	}
	if err := scanner.Err(); err != nil {
		return "", false, err
	}
	return host, found, nil
}

func unquoteWTFToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if value[0] == '"' || value[0] == '\'' {
		quote := value[0]
		end := strings.IndexByte(value[1:], quote)
		if end >= 0 {
			return strings.TrimSpace(value[1 : 1+end])
		}
		value = strings.TrimSpace(value[1:])
	}
	if fields := strings.Fields(value); len(fields) > 0 {
		token := fields[0]
		if strings.HasPrefix(token, "//") || strings.HasPrefix(token, "#") {
			return ""
		}
		return token
	}
	return ""
}

// LoadRealmlistAuthAddress loads the auth host the way the original client does:
// prefer the loose Data\<locale>\realmlist.wtf file (not an MPQ entry), else
// fall back to install\WTF\Config.wtf SET realmList when present.
// Returns ok=false when neither source yields a host (caller keeps prior auth).
func LoadRealmlistAuthAddress(dataPath, locale string) (string, bool, error) {
	path := RealmlistPath(dataPath, locale)
	if path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			host, found, parseErr := ParseRealmlistWTF(string(data))
			if parseErr != nil {
				return "", false, fmt.Errorf("%s: %w", path, parseErr)
			}
			if found {
				return host, true, nil
			}
		} else if !os.IsNotExist(err) {
			return "", false, err
		}
	}
	cfgPath := ConfigWTFPath(dataPath)
	if cfgPath == "" {
		return "", false, nil
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	host, found, parseErr := ParseConfigWTFRealmList(string(data))
	if parseErr != nil {
		return "", false, fmt.Errorf("%s: %w", cfgPath, parseErr)
	}
	return host, found, nil
}
