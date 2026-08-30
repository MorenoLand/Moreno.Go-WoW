package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/MorenoLand/Moreno.WoW/config"
	"github.com/MorenoLand/Moreno.WoW/network"
	"github.com/MorenoLand/Moreno.WoW/render"
)

func main() {
	configPath, err := config.Path()
	if err != nil {
		log.Fatalf("locating MorenoWoW config: %v", err)
	}
	options, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("loading %s: %v", configPath, err)
	}
	if value := os.Getenv("WOW_AUTH"); value != "" {
		options.AuthAddress = value
	}
	if value := os.Getenv("WOW_ACCOUNT"); value != "" {
		options.Account = value
	}
	if value := os.Getenv("WOW_LOCALE"); value != "" {
		options.Locale = value
	}
	if value := os.Getenv("WOW_REALM"); value != "" {
		options.Realm = value
	}
	if value := os.Getenv("WOW_DATA"); value != "" {
		options.DataPath = value
	}
	password := os.Getenv("WOW_PASSWORD")
	debug := false
	timeout := network.DefaultConfig().Timeout
	flag.StringVar(&options.AuthAddress, "auth", options.AuthAddress, "auth server address")
	flag.StringVar(&options.Account, "account", options.Account, "account name")
	flag.StringVar(&password, "password", password, "account password")
	flag.StringVar(&options.Locale, "locale", options.Locale, "client locale")
	flag.StringVar(&options.Realm, "realm", options.Realm, "realm name, address, or id")
	flag.StringVar(&options.DataPath, "data", options.DataPath, "Warcraft data directory")
	flag.DurationVar(&timeout, "timeout", timeout, "network operation timeout")
	flag.BoolVar(&debug, "debug", false, "write session status to the terminal")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: MorenoWoW [options]")
		flag.VisitAll(func(option *flag.Flag) { fmt.Fprintf(os.Stderr, "  --%s\t%s\n", option.Name, option.Usage) })
	}
	flag.Parse()
	if err := config.Save(configPath, options); err != nil {
		log.Printf("saving %s: %v", configPath, err)
	}
	render.Run(network.Config{AuthAddress: options.AuthAddress, Account: options.Account, Password: password, Locale: options.Locale, Realm: options.Realm, Timeout: timeout, Debug: debug}, options.DataPath, options.InterfacePath, options.Character, configPath, debug)
}
