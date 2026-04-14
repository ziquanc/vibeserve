package runtime

import (
	"fmt"

	tengo "github.com/d5/tengo/v2"
	"github.com/vibeserve/vibeserve/internal/engine"
)

// RequestContext holds parsed HTTP request data passed into a Tengo script.
type RequestContext struct {
	PathParams  map[string]string
	QueryParams map[string]string
	Body        map[string]any
	Headers     map[string]string // lowercase keys
	Method      string
}

// ResponseCapture collects the response produced by a script.
type ResponseCapture struct {
	StatusCode int
	Body       any
	Headers    map[string]string
	Written    bool
}

// Runtime executes Tengo scripts with the Vibe Standard Library injected.
type Runtime struct {
	store     engine.DataStore
	bus       *engine.Bus
	jwtSecret string
}

// New creates a Runtime backed by the given DataStore and event Bus.
// An optional jwtSecret enables JWT-based auth token generation and verification.
func New(store engine.DataStore, bus *engine.Bus, jwtSecret ...string) *Runtime {
	secret := ""
	if len(jwtSecret) > 0 {
		secret = jwtSecret[0]
	}
	return &Runtime{store: store, bus: bus, jwtSecret: secret}
}

// Execute runs code with the given RequestContext and returns the captured response.
// Returns an error if the script does not call response.json, response.error, or
// response.redirect (i.e. capture.Written remains false).
func (r *Runtime) Execute(code string, rc *RequestContext) (statusCode int, body any, headers map[string]string, err error) {
	capture := &ResponseCapture{}

	script := tengo.NewScript([]byte(code))

	// Inject all stdlib modules as pre-defined variables.
	if addErr := script.Add("db", newDBModule(r.store)); addErr != nil {
		return 0, nil, nil, fmt.Errorf("add db module: %w", addErr)
	}
	if addErr := script.Add("request", newRequestModule(rc, r.jwtSecret)); addErr != nil {
		return 0, nil, nil, fmt.Errorf("add request module: %w", addErr)
	}
	if addErr := script.Add("response", newResponseModule(capture)); addErr != nil {
		return 0, nil, nil, fmt.Errorf("add response module: %w", addErr)
	}
	if addErr := script.Add("date", newDateModule()); addErr != nil {
		return 0, nil, nil, fmt.Errorf("add date module: %w", addErr)
	}
	if addErr := script.Add("crypto", newCryptoModule()); addErr != nil {
		return 0, nil, nil, fmt.Errorf("add crypto module: %w", addErr)
	}
	if addErr := script.Add("log", newLogModule(r.bus)); addErr != nil {
		return 0, nil, nil, fmt.Errorf("add log module: %w", addErr)
	}
	if addErr := script.Add("auth", newAuthModule(r.jwtSecret, r.store)); addErr != nil {
		return 0, nil, nil, fmt.Errorf("add auth module: %w", addErr)
	}

	if _, runErr := script.Run(); runErr != nil {
		return 0, nil, nil, fmt.Errorf("script error: %w", runErr)
	}

	if !capture.Written {
		return 0, nil, nil, fmt.Errorf("script produced no response")
	}

	return capture.StatusCode, capture.Body, capture.Headers, nil
}
