package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zalando/go-keyring"
)

type Options struct {
	AuthAddress    string       `json:"auth_address"`
	Account        string       `json:"account"`
	Locale         string       `json:"locale"`
	Realm          string       `json:"realm"`
	Character      string       `json:"character"`
	DataPath       string       `json:"data_path"`
	InterfacePath  string       `json:"interface_path"`
	BackgroundPath string       `json:"background_path"` // Path to a static background image (JPEG/PNG) to use as the login screen scene. Leave empty to use the gradient fallback.
	RememberMe     bool         `json:"remember_me"`
	Audio          AudioOptions `json:"audio"`
}

type AudioOptions struct {
	EnableAll      bool    `json:"enable_all"`
	EnableMusic    bool    `json:"enable_music"`
	EnableSFX      bool    `json:"enable_sfx"`
	EnableAmbience bool    `json:"enable_ambience"`
	MasterVolume   float64 `json:"master_volume"`
	MusicVolume    float64 `json:"music_volume"`
	SFXVolume      float64 `json:"sfx_volume"`
	AmbienceVolume float64 `json:"ambience_volume"`
}

func Defaults() Options {
	return Options{
		AuthAddress: "127.0.0.1:3724",
		Locale:      "enUS",
		Audio:       DefaultAudioOptions(),
	}
}

func DefaultAudioOptions() AudioOptions {
	return AudioOptions{EnableAll: true, EnableMusic: true, EnableSFX: true, EnableAmbience: true, MasterVolume: 1, MusicVolume: 1, SFXVolume: 1, AmbienceVolume: 1}
}

func (options *AudioOptions) SetCVar(name, value string) bool {
	if options == nil {
		return false
	}
	value = strings.TrimSpace(value)
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "sound_enableallsound":
		options.EnableAll = value == "1" || strings.EqualFold(value, "true")
	case "sound_enablemusic":
		options.EnableMusic = value == "1" || strings.EqualFold(value, "true")
	case "sound_enablesfx":
		options.EnableSFX = value == "1" || strings.EqualFold(value, "true")
	case "sound_enableambience":
		options.EnableAmbience = value == "1" || strings.EqualFold(value, "true")
	case "sound_mastervolume":
		volume, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false
		}
		options.MasterVolume = clampVolume(volume)
	case "sound_musicvolume":
		volume, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false
		}
		options.MusicVolume = clampVolume(volume)
	case "sound_sfxvolume":
		volume, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false
		}
		options.SFXVolume = clampVolume(volume)
	case "sound_ambiencevolume":
		volume, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false
		}
		options.AmbienceVolume = clampVolume(volume)
	default:
		return false
	}
	return true
}

func (options AudioOptions) CVars() map[string]string {
	return map[string]string{
		"Sound_EnableAllSound": boolCVar(options.EnableAll),
		"Sound_EnableMusic":    boolCVar(options.EnableMusic),
		"Sound_EnableSFX":      boolCVar(options.EnableSFX),
		"Sound_EnableAmbience": boolCVar(options.EnableAmbience),
		"Sound_MasterVolume":   strconv.FormatFloat(clampVolume(options.MasterVolume), 'f', -1, 64),
		"Sound_MusicVolume":    strconv.FormatFloat(clampVolume(options.MusicVolume), 'f', -1, 64),
		"Sound_SFXVolume":      strconv.FormatFloat(clampVolume(options.SFXVolume), 'f', -1, 64),
		"Sound_AmbienceVolume": strconv.FormatFloat(clampVolume(options.AmbienceVolume), 'f', -1, 64),
	}
}

func boolCVar(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func clampVolume(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func Path() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "MorenoWoW", "config.json"), nil
}

func Load(path string) (Options, error) {
	options := Defaults()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return options, nil
	}
	if err != nil {
		return options, err
	}
	if err := json.Unmarshal(data, &options); err != nil {
		return options, err
	}
	defaults := Defaults()
	if options.AuthAddress == "" {
		options.AuthAddress = defaults.AuthAddress
	}
	if options.Locale == "" {
		options.Locale = defaults.Locale
	}
	if options.DataPath == "" {
		options.DataPath = defaults.DataPath
	} else {
		options.DataPath = repairDataPath(options.DataPath)
	}
	if options.InterfacePath == "" {
		options.InterfacePath = defaults.InterfacePath
	}
	return options, nil
}

func repairDataPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path
	}
	parent := filepath.Dir(path)
	prefix := strings.ToLower(filepath.Base(path))
	entries, err := os.ReadDir(parent)
	if err != nil {
		return path
	}
	var match string
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(strings.ToLower(entry.Name()), prefix) {
			continue
		}
		candidate := filepath.Join(parent, entry.Name(), "Data")
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			if match != "" {
				return path
			}
			match = candidate
		}
	}
	if match != "" {
		return match
	}
	return path
}

func Save(path string, options Options) error {
	data, err := json.MarshalIndent(options, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func LoadPassword(account, server string) (string, error) {
	key, err := credentialKey(account, server)
	if err != nil {
		return "", err
	}
	return keyring.Get("MorenoWoW", key)
}

func SavePassword(account, server, password string) error {
	key, err := credentialKey(account, server)
	if err != nil {
		return err
	}
	return keyring.Set("MorenoWoW", key, password)
}

func DeletePassword(account, server string) error {
	key, err := credentialKey(account, server)
	if err != nil {
		return err
	}
	err = keyring.Delete("MorenoWoW", key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

func credentialKey(account, server string) (string, error) {
	account = strings.TrimSpace(account)
	server = strings.TrimSpace(server)
	if account == "" || server == "" {
		return "", fmt.Errorf("account and server are required")
	}
	return strings.ToUpper(account) + "@" + strings.ToLower(server), nil
}
