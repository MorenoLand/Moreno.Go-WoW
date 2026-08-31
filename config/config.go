package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

type Options struct {
	AuthAddress    string `json:"auth_address"`
	Account        string `json:"account"`
	Locale         string `json:"locale"`
	Realm          string `json:"realm"`
	Character      string `json:"character"`
	DataPath       string `json:"data_path"`
	InterfacePath  string `json:"interface_path"`
	BackgroundPath string `json:"background_path"` // Path to a static background image (JPEG/PNG) to use as the login screen scene. Leave empty to use the gradient fallback.
	RememberMe     bool   `json:"remember_me"`
}

func Defaults() Options {
	return Options{
		AuthAddress: "127.0.0.1:3724",
		Locale:      "enUS",
	}
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
	}
	if options.InterfacePath == "" {
		options.InterfacePath = defaults.InterfacePath
	}
	return options, nil
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
