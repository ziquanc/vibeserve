package cloud

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Credentials struct {
	Token string `json:"token"`
	Email string `json:"email"`
	Plan  string `json:"plan"`
}

func credentialsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".vibeserve")
}

func credentialsPath() string {
	return filepath.Join(credentialsDir(), "credentials.json")
}

func LoadCredentials() *Credentials {
	data, err := os.ReadFile(credentialsPath())
	if err != nil {
		return nil
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil
	}
	if creds.Token == "" {
		return nil
	}
	return &creds
}

func SaveCredentials(creds *Credentials) error {
	if err := os.MkdirAll(credentialsDir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(credentialsPath(), data, 0o600)
}

func DeleteCredentials() error {
	return os.Remove(credentialsPath())
}

func IsLoggedIn() bool {
	return LoadCredentials() != nil
}
