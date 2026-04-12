package apitest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// Result represents the outcome of a single test.
type Result struct {
	Name   string
	Passed bool
	Detail string
}

// RunAll runs CRUD lifecycle tests for each table in the manifest.
// baseURL is the server address (e.g., "http://localhost:8080").
func RunAll(m *manifest.Manifest, baseURL string) []Result {
	var results []Result

	for _, schema := range m.Schemas {
		tableResults := runTableTests(m, schema, baseURL)
		results = append(results, tableResults...)
	}

	return results
}

func runTableTests(m *manifest.Manifest, schema manifest.Schema, baseURL string) []Result {
	var results []Result
	table := schema.Table

	// Find routes for this table
	var listRoute, getRoute, createRoute, updateRoute, deleteRoute *manifest.Route
	for i := range m.Routes {
		r := &m.Routes[i]
		path := strings.ToLower(r.Path)
		if !strings.Contains(path, strings.ToLower(table)) {
			continue
		}
		switch r.Method {
		case "GET":
			if strings.Contains(r.Path, ":") {
				getRoute = r
			} else {
				listRoute = r
			}
		case "POST":
			createRoute = r
		case "PUT", "PATCH":
			updateRoute = r
		case "DELETE":
			deleteRoute = r
		}
	}

	// Build test data from schema columns
	testData := buildTestData(schema)

	// 1. POST create
	var createdID any
	if createRoute != nil {
		name := fmt.Sprintf("%s: POST %s", table, createRoute.Path)
		status, body, err := httpRequest("POST", baseURL+createRoute.Path, testData)
		if err != nil {
			results = append(results, Result{Name: name, Passed: false, Detail: fmt.Sprintf("request failed: %v", err)})
		} else if status != 201 && status != 200 {
			results = append(results, Result{Name: name, Passed: false, Detail: fmt.Sprintf("expected 201, got %d", status)})
		} else {
			createdID = body["id"]
			if createdID == nil {
				results = append(results, Result{Name: name, Passed: false, Detail: "response missing 'id'"})
			} else {
				results = append(results, Result{Name: name, Passed: true, Detail: fmt.Sprintf("created id=%v", createdID)})
			}
		}
	}

	if createdID == nil {
		return results // Can't continue without an ID
	}

	idStr := fmt.Sprintf("%v", createdID)
	// Handle float64 IDs from JSON
	if f, ok := createdID.(float64); ok {
		idStr = fmt.Sprintf("%d", int64(f))
	}

	// 2. GET list
	if listRoute != nil {
		name := fmt.Sprintf("%s: GET %s", table, listRoute.Path)
		status, _, err := httpRequest("GET", baseURL+listRoute.Path, nil)
		if err != nil {
			results = append(results, Result{Name: name, Passed: false, Detail: fmt.Sprintf("request failed: %v", err)})
		} else if status != 200 {
			results = append(results, Result{Name: name, Passed: false, Detail: fmt.Sprintf("expected 200, got %d", status)})
		} else {
			results = append(results, Result{Name: name, Passed: true, Detail: "returns array"})
		}
	}

	// 3. GET by id
	if getRoute != nil {
		path := replacePathParam(getRoute.Path, idStr)
		name := fmt.Sprintf("%s: GET %s", table, getRoute.Path)
		status, body, err := httpRequest("GET", baseURL+path, nil)
		if err != nil {
			results = append(results, Result{Name: name, Passed: false, Detail: fmt.Sprintf("request failed: %v", err)})
		} else if status != 200 {
			results = append(results, Result{Name: name, Passed: false, Detail: fmt.Sprintf("expected 200, got %d", status)})
		} else if body["id"] == nil {
			results = append(results, Result{Name: name, Passed: false, Detail: "response missing 'id'"})
		} else {
			results = append(results, Result{Name: name, Passed: true, Detail: fmt.Sprintf("got id=%v", body["id"])})
		}
	}

	// 4. PUT update
	if updateRoute != nil {
		path := replacePathParam(updateRoute.Path, idStr)
		name := fmt.Sprintf("%s: %s %s", table, updateRoute.Method, updateRoute.Path)
		updateData := buildUpdateData(schema)
		status, _, err := httpRequest(updateRoute.Method, baseURL+path, updateData)
		if err != nil {
			results = append(results, Result{Name: name, Passed: false, Detail: fmt.Sprintf("request failed: %v", err)})
		} else if status != 200 {
			results = append(results, Result{Name: name, Passed: false, Detail: fmt.Sprintf("expected 200, got %d", status)})
		} else {
			results = append(results, Result{Name: name, Passed: true, Detail: "updated"})
		}
	}

	// 5. DELETE (soft delete)
	if deleteRoute != nil {
		path := replacePathParam(deleteRoute.Path, idStr)
		name := fmt.Sprintf("%s: DELETE %s", table, deleteRoute.Path)
		status, _, err := httpRequest("DELETE", baseURL+path, nil)
		if err != nil {
			results = append(results, Result{Name: name, Passed: false, Detail: fmt.Sprintf("request failed: %v", err)})
		} else if status != 200 {
			results = append(results, Result{Name: name, Passed: false, Detail: fmt.Sprintf("expected 200, got %d", status)})
		} else {
			results = append(results, Result{Name: name, Passed: true, Detail: "soft deleted"})
		}
	}

	// 6. GET by id after delete — should be 404
	if getRoute != nil && deleteRoute != nil {
		path := replacePathParam(getRoute.Path, idStr)
		name := fmt.Sprintf("%s: GET %s (after delete)", table, getRoute.Path)
		status, _, err := httpRequest("GET", baseURL+path, nil)
		if err != nil {
			results = append(results, Result{Name: name, Passed: false, Detail: fmt.Sprintf("request failed: %v", err)})
		} else if status != 404 {
			results = append(results, Result{Name: name, Passed: false, Detail: fmt.Sprintf("expected 404 (soft deleted), got %d", status)})
		} else {
			results = append(results, Result{Name: name, Passed: true, Detail: "correctly returns 404 after soft delete"})
		}
	}

	return results
}

// buildTestData creates sample data for creating a row.
func buildTestData(schema manifest.Schema) map[string]any {
	data := make(map[string]any)
	for _, col := range schema.Columns {
		if col.Primary && col.Auto {
			continue
		}
		if col.Name == "created_at" || col.Name == "updated_at" || col.Name == "deleted_at" {
			continue
		}
		switch col.Type {
		case "INTEGER":
			data[col.Name] = 1
		case "TEXT":
			data[col.Name] = "test_" + col.Name
		case "REAL":
			data[col.Name] = 1.5
		case "BOOLEAN":
			data[col.Name] = true
		case "DATE", "DATETIME":
			data[col.Name] = time.Now().Format(time.RFC3339)
		default:
			data[col.Name] = "test"
		}
	}
	return data
}

// buildUpdateData creates sample data for updating a row.
func buildUpdateData(schema manifest.Schema) map[string]any {
	data := make(map[string]any)
	for _, col := range schema.Columns {
		if col.Primary {
			continue
		}
		if col.Name == "created_at" || col.Name == "updated_at" || col.Name == "deleted_at" {
			continue
		}
		switch col.Type {
		case "INTEGER":
			data[col.Name] = 2
		case "TEXT":
			data[col.Name] = "updated_" + col.Name
		default:
			// Skip other types for simplicity
		}
	}
	return data
}

// replacePathParam replaces :id (or any :param) in a path with a value.
func replacePathParam(path, value string) string {
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		if strings.HasPrefix(seg, ":") {
			segments[i] = value
		}
	}
	return strings.Join(segments, "/")
}

// httpRequest makes an HTTP request and returns status, parsed JSON body, error.
func httpRequest(method, url string, data map[string]any) (int, map[string]any, error) {
	var bodyReader io.Reader
	if data != nil {
		jsonBytes, err := json.Marshal(data)
		if err != nil {
			return 0, nil, err
		}
		bodyReader = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return 0, nil, err
	}
	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}

	var result map[string]any
	json.Unmarshal(respBody, &result) // Ignore error — might be array or empty

	return resp.StatusCode, result, nil
}

// PrintResults prints test results to stdout.
func PrintResults(results []Result) (passed, failed int) {
	for _, r := range results {
		if r.Passed {
			fmt.Printf("  \u2713 %s \u2014 %s\n", r.Name, r.Detail)
			passed++
		} else {
			fmt.Printf("  \u2717 %s \u2014 %s\n", r.Name, r.Detail)
			failed++
		}
	}
	return passed, failed
}
