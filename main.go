package main

import (
	"flag"

	"moreno.warcraft/network"
	"moreno.warcraft/render"
)

func main() {
	config := network.ConfigFromEnvironment()
	flag.StringVar(&config.AuthAddress, "auth", config.AuthAddress, "auth server address")
	flag.StringVar(&config.Account, "account", config.Account, "account name")
	flag.StringVar(&config.Password, "password", config.Password, "account password")
	flag.StringVar(&config.Locale, "locale", config.Locale, "client locale")
	flag.StringVar(&config.Realm, "realm", config.Realm, "realm name, address, or id")
	flag.DurationVar(&config.Timeout, "timeout", config.Timeout, "network operation timeout")
	flag.Parse()
	render.Run(config)
}
