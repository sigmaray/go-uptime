// Package main is the go-uptime application entrypoint.
package main

import (
	"embed"
	"os"

	"go-uptime/cmd"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	loadEnv()

	cmd.Init(embedMigrations)
	cmd.Execute()
}

func loadEnv() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Warn().Err(err).Msg("failed to load .env file")
	}
}
