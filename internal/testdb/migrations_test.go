package testdb

import "testing"

// TestValidateTestDatabaseName checks that tests cannot target application or system databases.
func TestValidateTestDatabaseName(t *testing.T) {
	tests := []struct {
		name            string
		testName        string
		applicationName string
		wantError       bool
	}{
		{name: "separate test database", testName: "go-uptime-test", applicationName: "gouptime"},
		{name: "empty name", applicationName: "gouptime", wantError: true},
		{name: "same as application database", testName: "gouptime", applicationName: "gouptime", wantError: true},
		{name: "case-insensitive application match", testName: "GOUptime", applicationName: "gouptime", wantError: true},
		{name: "postgres system database", testName: "postgres", applicationName: "gouptime", wantError: true},
		{name: "template database", testName: "template1", applicationName: "gouptime", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTestDatabaseName(tt.testName, tt.applicationName)
			if (err != nil) != tt.wantError {
				t.Fatalf("ValidateTestDatabaseName(%q, %q) error = %v, wantError %v", tt.testName, tt.applicationName, err, tt.wantError)
			}
		})
	}
}
