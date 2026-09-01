package network

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.WoW/auth"
	"github.com/MorenoLand/Moreno.WoW/world"
)

type Config struct {
	AuthAddress string
	Account     string
	Password    string
	Locale      string
	Realm       string
	Timeout     time.Duration
	Debug       bool
}

type Session struct {
	Realm      auth.Realm
	Characters []world.Character
	connection *world.Connection
}

type Authenticated struct {
	SessionKey [auth.SessionKeyLen]byte
	Realms     []auth.Realm
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

func Login(config Config) (*Session, error) {
	authenticated, err := Authenticate(config)
	if err != nil {
		return nil, err
	}
	realm, err := selectRealm(authenticated.Realms, config.Realm)
	if err != nil {
		return nil, err
	}
	return OpenRealm(authenticated, config.Account, realm, config.Timeout, config.Debug)
}

func Authenticate(config Config) (Authenticated, error) {
	if config.Account == "" || config.Password == "" {
		return Authenticated{}, fmt.Errorf("account and password are required")
	}
	authHost, authPort, err := splitEndpoint(config.AuthAddress, auth.DefaultPort)
	if err != nil {
		return Authenticated{}, err
	}
	debugf(config, "auth: connecting to %s:%d", authHost, authPort)
	loggedIn, err := auth.Login(authHost, authPort, config.Account, config.Password, config.Locale, config.Timeout)
	if err != nil {
		return Authenticated{}, err
	}
	debugf(config, "auth: received %d realm(s)", len(loggedIn.Realms))
	return Authenticated{SessionKey: loggedIn.SessionKey, Realms: loggedIn.Realms}, nil
}

func OpenRealm(authenticated Authenticated, account string, realm auth.Realm, timeout time.Duration, debug bool) (*Session, error) {
	worldHost, worldPort, err := world.SplitRealmAddress(realm.Address)
	if err != nil {
		return nil, err
	}
	debugf(Config{Debug: debug}, "world: selected realm %q at %s", realm.Name, realm.Address)
	address := net.JoinHostPort(worldHost, strconv.Itoa(int(worldPort)))
	connection, err := world.Open(address, account, realm.ID, authenticated.SessionKey, timeout)
	if err != nil {
		return nil, err
	}
	characters, err := connection.Characters()
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	debugf(Config{Debug: debug}, "world: received %d character(s)", len(characters))
	return &Session{Realm: realm, Characters: characters, connection: connection}, nil
}

func (s *Session) Close() error {
	if s == nil || s.connection == nil {
		return nil
	}
	return s.connection.Close()
}

func (s *Session) EnterWorld(index int) (world.WorldPosition, error) {
	if s == nil || s.connection == nil {
		return world.WorldPosition{}, fmt.Errorf("world session is closed")
	}
	if index < 0 || index >= len(s.Characters) {
		return world.WorldPosition{}, fmt.Errorf("character index %d is out of range", index)
	}
	return s.connection.EnterWorld(s.Characters[index].GUID)
}

func (s *Session) StartWorldPackets() <-chan world.PacketEvent {
	if s == nil || s.connection == nil {
		return nil
	}
	return s.connection.StartPackets()
}

func debugf(config Config, format string, args ...interface{}) {
	if config.Debug {
		log.Printf(format, args...)
	}
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
	if strings.HasPrefix(address, "[") {
		close := strings.IndexByte(address, ']')
		if close < 0 {
			return "", 0, fmt.Errorf("invalid bracketed auth address %q", address)
		}
		if len(address) == close+1 {
			return address[1:close], defaultPort, nil
		}
		if address[close+1] != ':' {
			return "", 0, fmt.Errorf("invalid auth address %q", address)
		}
		value, err := strconv.ParseUint(address[close+2:], 10, 16)
		if err != nil || value == 0 {
			return "", 0, fmt.Errorf("invalid auth port in %q", address)
		}
		return address[1:close], uint16(value), nil
	}
	if strings.Count(address, ":") == 1 {
		index := strings.LastIndexByte(address, ':')
		value, err := strconv.ParseUint(address[index+1:], 10, 16)
		if err != nil || value == 0 {
			return "", 0, fmt.Errorf("invalid auth port in %q", address)
		}
		return address[:index], uint16(value), nil
	}
	return strings.Trim(address, "[]"), defaultPort, nil
}
