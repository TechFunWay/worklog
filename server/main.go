package main

import (
	"smallgo/server/config"
	"smallgo/server/server"

	// Blank-import apps so their init() registers routes with the app registry.
	// Add your own apps here.
	_ "smallgo/server/qrcode"
	_ "smallgo/server/worklog"
)

func main() {
	// Local .env files are a development convenience only: values already
	// supplied by Docker, fnOS or the process environment always take priority.
	config.LoadDotEnv(".env", "server/.env")
	config.Parse()
	server.Start()
}
