package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"go-uptime/database"
	"go-uptime/internal/cliutil"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// ClearTable clears the specified table after confirmation.
func ClearTable(db *gorm.DB, table string) {
	reader := bufio.NewReader(os.Stdin)
	if !confirmYes(reader, fmt.Sprintf("Clear table %q? [y/N]: ", table)) {
		fmt.Println("Aborted.")
		return
	}
	if err := database.ClearTable(db, table); err != nil {
		log.Fatal().Err(err).Str("table", table).Msg("failed to clear table")
	}
	fmt.Printf("Table %q cleared.\n", table)
}

// ClearAllTables clears all tables after confirmation.
func ClearAllTables(db *gorm.DB) {
	reader := bufio.NewReader(os.Stdin)
	if !confirmYes(reader, "This will clear ALL tables. Are you sure? [y/N]: ") {
		fmt.Println("Aborted.")
		return
	}
	if err := database.ClearAllTables(db); err != nil {
		log.Fatal().Err(err).Msg("failed to clear all tables")
	}
	fmt.Println("All tables cleared.")
}

// DropTable drops the specified table after confirmation.
func DropTable(db *gorm.DB, table string) {
	reader := bufio.NewReader(os.Stdin)
	if !confirmYes(reader, fmt.Sprintf("Drop table %q? [y/N]: ", table)) {
		fmt.Println("Aborted.")
		return
	}
	if err := database.DropTable(db, table); err != nil {
		log.Fatal().Err(err).Str("table", table).Msg("failed to drop table")
	}
	fmt.Printf("Table %q dropped.\n", table)
}

// DropAllTables drops all tables after confirmation.
func DropAllTables(db *gorm.DB) {
	reader := bufio.NewReader(os.Stdin)
	if !confirmYes(reader, "This will DROP ALL tables. Are you sure? [y/N]: ") {
		fmt.Println("Aborted.")
		return
	}
	if err := database.DropAllTables(db); err != nil {
		log.Fatal().Err(err).Msg("failed to drop all tables")
	}
	fmt.Println("All tables dropped.")
}

// ExecuteSQL runs an SQL query and prints the result as a table.
func ExecuteSQL(db *gorm.DB, query string) {
	columns, rows, affected, err := database.ExecuteSQL(db, query)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to execute sql")
	}
	if columns != nil {
		cliutil.PrintTable(columns, rows)
		return
	}
	fmt.Printf("Query executed. Rows affected: %d\n", affected)
}

// ShowTables prints the list of tables (helper command).
func ShowTables(db *gorm.DB) {
	tables, err := database.ListTables(db)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to list tables")
	}
	if len(tables) == 0 {
		fmt.Println("No tables found.")
		return
	}
	rows := make([][]string, 0, len(tables))
	for _, t := range tables {
		rows = append(rows, []string{t})
	}
	cliutil.PrintTable([]string{"Table"}, rows)
}

// ReadSQLFromArgs joins arguments into a single SQL command.
func ReadSQLFromArgs(args []string) string {
	return strings.Join(args, " ")
}
