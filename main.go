// Package main — точка входа приложения go-uptime.
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

	// Передаём embed.FS миграций в cmd — server и db-goose-migrate используют один источник.
	cmd.Init(embedMigrations)
	cmd.Execute()
}

func loadEnv() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		// .env отсутствует — нормально; другие ошибки чтения логируем.
		log.Warn().Err(err).Msg("failed to load .env file")
	}
}
