// Package cli provides interactive CLI commands for managing users and the database.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"go-uptime/internal/cliutil"
	"go-uptime/internal/forms"
	"go-uptime/models"

	"github.com/rs/zerolog/log"
	"golang.org/x/term"
	"gorm.io/gorm"
)

// UsersSeed creates an admin/admin user after confirmation.
func UsersSeed(db *gorm.DB) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("WARNING: Creating user with login \"admin\" and password \"admin\" is insecure and dangerous in production.")
	if !confirmYes(reader, "Do you want to continue? [y/N]: ") {
		fmt.Println("Aborted.")
		return
	}

	existing, err := models.FindUserByUsername(db, "admin")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to check admin")
	}
	if existing != nil {
		fmt.Println("User 'admin' already exists")
		return
	}

	input := forms.CreateUserInput{Username: "admin", Password: "admin", ConfirmPassword: "admin"}
	// UsersSeed intentionally bypasses validation to allow the documented insecure admin/admin credentials.
	user, err := models.CreateUser(db, input.Username, input.Password)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create admin user")
	}
	fmt.Printf("Created user: id=%d username=%s\n", user.ID, user.Username)
}

// UsersCreate interactively creates a new user.
func UsersCreate(db *gorm.DB) {
	reader := bufio.NewReader(os.Stdin)

	username := readLine(reader, "Login: ")
	password := readPassword("Password: ")
	confirm := readPassword("Confirm password: ")

	input := forms.CreateUserInput{Username: username, Password: password, ConfirmPassword: confirm}
	if err := input.Validate(); err != nil {
		log.Fatal().Str("error", forms.FormatValidationError(err)).Msg("invalid input")
	}

	user, err := models.CreateUser(db, input.Username, input.Password)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create user")
	}

	fmt.Printf("Created user: id=%d username=%s\n", user.ID, user.Username)
}

// UsersShow prints all users as a table.
func UsersShow(db *gorm.DB) {
	var users []models.User
	if err := db.Order("id asc").Find(&users).Error; err != nil {
		log.Fatal().Err(err).Msg("failed to fetch users")
	}

	if len(users) == 0 {
		fmt.Println("No users found.")
		return
	}

	headers := []string{"ID", "Username", "Created At"}
	rows := make([][]string, 0, len(users))
	for _, user := range users {
		rows = append(rows, []string{
			fmt.Sprintf("%d", user.ID),
			user.Username,
			user.CreatedAt.Format("2006-01-02 15:04"),
		})
	}
	cliutil.PrintTable(headers, rows)
}

// UsersDeleteAll deletes all users after confirmation.
func UsersDeleteAll(db *gorm.DB) {
	reader := bufio.NewReader(os.Stdin)

	if !confirmYes(reader, "This will permanently delete ALL users. Are you sure? [y/N]: ") {
		fmt.Println("Aborted.")
		return
	}

	result := db.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&models.User{})
	if result.Error != nil {
		log.Fatal().Err(result.Error).Msg("failed to delete users")
	}
	fmt.Printf("Deleted %d user(s).\n", result.RowsAffected)
}

func confirmYes(reader *bufio.Reader, prompt string) bool {
	answer := strings.ToLower(readLine(reader, prompt))
	return answer == "y" || answer == "yes"
}

func readLine(reader *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	line, err := reader.ReadString('\n')
	if err != nil {
		log.Fatal().Err(err).Msg("failed to read input")
	}
	return strings.TrimSpace(line)
}

func readPassword(prompt string) string {
	fmt.Print(prompt)
	bytePassword, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to read input")
	}
	return strings.TrimSpace(string(bytePassword))
}
