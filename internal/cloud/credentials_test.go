package cloud

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadCredentials(t *testing.T) {
	// Use temp dir to avoid touching real ~/.vibeserve
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	creds := &Credentials{
		Token: "test-token-123",
		Email: "test@example.com",
		Plan:  "pro",
	}

	if err := SaveCredentials(creds); err != nil {
		t.Fatal(err)
	}

	loaded := LoadCredentials()
	if loaded == nil {
		t.Fatal("should load saved credentials")
	}
	if loaded.Token != "test-token-123" {
		t.Errorf("token: got %q, want %q", loaded.Token, "test-token-123")
	}
	if loaded.Email != "test@example.com" {
		t.Errorf("email: got %q, want %q", loaded.Email, "test@example.com")
	}
	if loaded.Plan != "pro" {
		t.Errorf("plan: got %q, want %q", loaded.Plan, "pro")
	}

	// Verify file permissions
	info, err := os.Stat(filepath.Join(tmpDir, ".vibeserve", "credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file permissions: got %o, want 0600", info.Mode().Perm())
	}
}

func TestDeleteCredentials(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	SaveCredentials(&Credentials{Token: "x", Email: "a@b.com", Plan: "free"})

	if !IsLoggedIn() {
		t.Error("should be logged in after save")
	}

	DeleteCredentials()

	if IsLoggedIn() {
		t.Error("should not be logged in after delete")
	}
}

func TestLoadCredentials_NotLoggedIn(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	if LoadCredentials() != nil {
		t.Error("should return nil when no credentials file")
	}
	if IsLoggedIn() {
		t.Error("should not be logged in")
	}
}
