package network

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"moreno.warcraft/auth"
	"moreno.warcraft/world"
)

type Config struct {
	AuthAddress string
	Account     string
	Password    string
	Locale      string
	Realm       string
	Timeout     time.Duration
}

type Result struct {
	Realm      auth.Realm
	Characters []world.Character
}

func DefaultConfig() Config {
	return Config{AuthAddress: "127.0.0.1:3724", Locale: "enUS", Timeout: 8 * time.Second}
}

func ConfigFromEnvironment() Config {
	config := DefaultConfig()
	if value := os.Getenv("WOW_AUTH"); value != "" {
		config.AuthAddress = value
	}
	config.Account = os.Getenv("WOW_ACCOUNT")
	config.Password = os.Getenv("WOW_PASSWORD")
	if value := os.Getenv("WOW_LOCALE"); value != "" {
		config.Locale = value
	}
	config.Realm = os.Getenv("WOW_REALM")
	return config
}

func Login(config Config) (Result, error) {
	if config.Account == "" || config.Password == "" {
		return Result{}, fmt.Errorf("WOW_ACCOUNT and WOW_PASSWORD are required")
	}
	authHost, authPort, err := splitEndpoint(config.AuthAddress, auth.DefaultPort)
	if err != nil {
		return Result{}, err
	}
	loggedIn, err := auth.Login(authHost, authPort, config.Account, config.Password, config.Locale, config.Timeout)
	if err != nil {
		return Result{}, err
	}
	realm, err := selectRealm(loggedIn.Realms, config.Realm)
	if err != nil {
		return Result{}, err
	}
	worldHost, worldPort, err := world.SplitRealmAddress(realm.Address)
	if err != nil {
		return Result{}, err
	}
	address := net.JoinHostPort(worldHost, strconv.Itoa(int(worldPort)))
	connection, err := world.Open(address, config.Account, realm.ID, loggedIn.SessionKey, config.Timeout)
	if err != nil {
		return Result{}, err
	}
	defer connection.Close()
	characters, err := connection.Characters()
	if err != nil {
		return Result{}, err
	}
	return Result{Realm: realm, Characters: characters}, nil
}

func selectRealm(realms []auth.Realm, selector string) (auth.Realm, error) {
	selector = strings.TrimSpace(selector)
	if len(realms) == 0 {
		return auth.Realm{}, fmt.Errorf("auth server returned no realms")
	}
	if selector != "" {
		for _, realm := range realms {
			if strings.EqualFold(realm.Name, selector) || strings.EqualFold(realm.Address, selector) || strconv.Itoa(int(realm.ID)) == selector {
				return realm, nil
			}
		}
		return auth.Realm{}, fmt.Errorf("realm %q was not returned by the auth server", selector)
	}
	for _, realm := range realms {
		if !realm.IsOffline() && !realm.Locked {
			return realm, nil
		}
	}
	for _, realm := range realms {
		if !realm.IsOffline() {
			return realm, nil
		}
	}
	return realms[0], nil
}

func splitEndpoint(address string, defaultPort uint16) (string, uint16, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", 0, fmt.Errorf("auth address is empty")
	}
	if host, port, err := net.SplitHostPort(address); err == nil {
		value, err := strconv.ParseUint(port, 10, 16)
		if err != nil || value == 0 {
			return "", 0, fmt.Errorf("invalid auth port in %q", address)
		}
		return host, uint16(value), nil
	}
	if index := strings.LastIndexByte(address, ':'); index >= 0 && !strings.Contains(address[index+1:], ":") {
		if value, err := strconv.ParseUint(address[index+1:], 10, 16); err == nil && value != 0 {
			return address[:index], uint16(value), nil
		}
	}
	return strings.Trim(address, "[]"), defaultPort, nil
}
