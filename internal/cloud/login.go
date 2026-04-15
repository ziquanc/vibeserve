package cloud

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/google/uuid"
)

// PlatformURL is the base URL for vibeserve.dev.
// Override for local dev.
var PlatformURL = "http://localhost:3000"

// Login opens the browser for device auth and polls for the token.
func Login() (*Credentials, error) {
	code := uuid.New().String()
	url := fmt.Sprintf("%s/auth/device?code=%s", PlatformURL, code)

	fmt.Printf("\n  Opening browser for login...\n")
	fmt.Printf("  If it doesn't open, visit:\n  %s\n\n", url)
	openBrowser(url)

	fmt.Printf("  Waiting for login")
	for i := 0; i < 150; i++ { // 5 min timeout
		time.Sleep(2 * time.Second)
		fmt.Print(".")

		resp, err := http.Get(fmt.Sprintf("%s/api/auth/device/poll?code=%s", PlatformURL, code))
		if err != nil {
			continue
		}

		var result struct {
			Status string `json:"status"`
			Token  string `json:"token"`
			Email  string `json:"email"`
			Plan   string `json:"plan"`
		}
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		if result.Status == "pending" || result.Status == "" {
			continue
		}

		if result.Status == "authorized" {
			creds := &Credentials{
				Token: result.Token,
				Email: result.Email,
				Plan:  result.Plan,
			}
			if err := SaveCredentials(creds); err != nil {
				return nil, fmt.Errorf("failed to save credentials: %w", err)
			}
			fmt.Printf("\n\n  Logged in as %s (%s plan)\n\n", result.Email, result.Plan)
			return creds, nil
		}
	}

	return nil, fmt.Errorf("login timed out — please try again")
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	cmd.Start()
}
