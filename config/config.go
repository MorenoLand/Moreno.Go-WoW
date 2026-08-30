package config

import (
	"encoding/json"
	"errors"
	"os"
)

type Options struct {
	AuthAddress string `json:"auth_address"`
	Account     string `json:"account"`
	Locale      string `json:"locale"`
	Realm       string `json:"realm"`
	DataPath    string `json:"data_path"`
}

func Defaults() Options { return Options{AuthAddress: "127.0.0.1:3724", Locale: "enUS"} }

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
	return options, nil
}

func Save(path string, options Options) error {
	data, err := json.MarshalIndent(options, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0600)
}
