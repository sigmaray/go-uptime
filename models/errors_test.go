package models

import (
	"errors"
	"testing"
)

func TestIsUniqueViolation(t *testing.T) {
	if IsUniqueViolation(nil) {
		t.Fatal("IsUniqueViolation(nil) = true, want false")
	}
	if IsUniqueViolation(errors.New("other")) {
		t.Fatal("IsUniqueViolation(generic) = true, want false")
	}
}

func TestMonitorURLUniqueConstraint(t *testing.T) {
	db := openTestDB(t)
	resetUptimeStatTables(t, db)

	first := MonitorURL{Name: "first", URL: "https://unique-constraint.example.com"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("create first monitor: %v", err)
	}

	dup := MonitorURL{Name: "second", URL: "https://unique-constraint.example.com"}
	err := db.Create(&dup).Error
	if err == nil {
		t.Fatal("expected unique violation when creating duplicate URL")
	}
	if !IsUniqueViolation(err) {
		t.Fatalf("IsUniqueViolation() = false for %v", err)
	}

	other := MonitorURL{Name: "other", URL: "https://other-unique.example.com"}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("create other monitor: %v", err)
	}

	other.URL = first.URL
	err = db.Save(&other).Error
	if err == nil {
		t.Fatal("expected unique violation when updating to an existing URL")
	}
	if !IsUniqueViolation(err) {
		t.Fatalf("IsUniqueViolation() = false for update error %v", err)
	}
}
