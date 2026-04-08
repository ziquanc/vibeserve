package runtime

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	mathrand "math/rand"
	"strings"
	"time"

	tengo "github.com/d5/tengo/v2"
	"github.com/google/uuid"
	"github.com/vibeserve/vibeserve/internal/engine"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// rowsToInterface converts []map[string]any to []interface{} for tengo.FromInterface.
func rowsToInterface(rows []map[string]any) []interface{} {
	out := make([]interface{}, len(rows))
	for i, row := range rows {
		m := make(map[string]interface{}, len(row))
		for k, v := range row {
			m[k] = v
		}
		out[i] = m
	}
	return out
}

// tengoArrayToGoSlice converts a Tengo Array or ImmutableArray to []any.
func tengoArrayToGoSlice(obj tengo.Object) []any {
	switch a := obj.(type) {
	case *tengo.Array:
		out := make([]any, len(a.Value))
		for i, v := range a.Value {
			out[i] = tengo.ToInterface(v)
		}
		return out
	case *tengo.ImmutableArray:
		out := make([]any, len(a.Value))
		for i, v := range a.Value {
			out[i] = tengo.ToInterface(v)
		}
		return out
	default:
		return nil
	}
}

// tengoToGoMap converts a Tengo Map or ImmutableMap to map[string]any.
func tengoToGoMap(obj tengo.Object) (map[string]any, error) {
	switch m := obj.(type) {
	case *tengo.Map:
		out := make(map[string]any, len(m.Value))
		for k, v := range m.Value {
			out[k] = tengo.ToInterface(v)
		}
		return out, nil
	case *tengo.ImmutableMap:
		out := make(map[string]any, len(m.Value))
		for k, v := range m.Value {
			out[k] = tengo.ToInterface(v)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected map, got %T", obj)
	}
}

// strObj wraps a string in *tengo.String.
func strObj(s string) *tengo.String { return &tengo.String{Value: s} }

// intObj wraps an int64 in *tengo.Int.
func intObj(n int64) *tengo.Int { return &tengo.Int{Value: n} }

// ── db module ─────────────────────────────────────────────────────────────────

func newDBModule(store engine.DataStore) tengo.Object {
	return &tengo.ImmutableMap{Value: map[string]tengo.Object{

		"query": &tengo.UserFunction{
			Name: "query",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				sql, ok := tengo.ToString(args[0])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "sql", Expected: "string", Found: args[0].TypeName()}
				}
				params := tengoArrayToGoSlice(args[1])
				rows, err := store.Query(sql, params)
				if err != nil {
					return &tengo.Error{Value: strObj(err.Error())}, nil
				}
				obj, err := tengo.FromInterface(rowsToInterface(rows))
				if err != nil {
					return nil, err
				}
				return obj, nil
			},
		},

		"query_one": &tengo.UserFunction{
			Name: "query_one",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				sql, ok := tengo.ToString(args[0])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "sql", Expected: "string", Found: args[0].TypeName()}
				}
				params := tengoArrayToGoSlice(args[1])
				row, err := store.QueryOne(sql, params)
				if err != nil {
					return &tengo.Error{Value: strObj(err.Error())}, nil
				}
				if row == nil {
					return tengo.UndefinedValue, nil
				}
				// Convert map[string]any → map[string]interface{} for FromInterface
				m := make(map[string]interface{}, len(row))
				for k, v := range row {
					m[k] = v
				}
				obj, err := tengo.FromInterface(m)
				if err != nil {
					return nil, err
				}
				return obj, nil
			},
		},

		"insert": &tengo.UserFunction{
			Name: "insert",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				table, ok := tengo.ToString(args[0])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "table", Expected: "string", Found: args[0].TypeName()}
				}
				data, err := tengoToGoMap(args[1])
				if err != nil {
					return nil, err
				}
				row, err := store.Insert(table, data)
				if err != nil {
					return &tengo.Error{Value: strObj(err.Error())}, nil
				}
				m := make(map[string]interface{}, len(row))
				for k, v := range row {
					m[k] = v
				}
				obj, err := tengo.FromInterface(m)
				if err != nil {
					return nil, err
				}
				return obj, nil
			},
		},

		"update": &tengo.UserFunction{
			Name: "update",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 3 {
					return nil, tengo.ErrWrongNumArguments
				}
				table, ok := tengo.ToString(args[0])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "table", Expected: "string", Found: args[0].TypeName()}
				}
				id := tengo.ToInterface(args[1])
				data, err := tengoToGoMap(args[2])
				if err != nil {
					return nil, err
				}
				row, err := store.Update(table, id, data)
				if err != nil {
					return &tengo.Error{Value: strObj(err.Error())}, nil
				}
				m := make(map[string]interface{}, len(row))
				for k, v := range row {
					m[k] = v
				}
				obj, err := tengo.FromInterface(m)
				if err != nil {
					return nil, err
				}
				return obj, nil
			},
		},

		"delete": &tengo.UserFunction{
			Name: "delete",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				table, ok := tengo.ToString(args[0])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "table", Expected: "string", Found: args[0].TypeName()}
				}
				id := tengo.ToInterface(args[1])
				deleted, err := store.Delete(table, id)
				if err != nil {
					return &tengo.Error{Value: strObj(err.Error())}, nil
				}
				if deleted {
					return tengo.TrueValue, nil
				}
				return tengo.FalseValue, nil
			},
		},

		"count": &tengo.UserFunction{
			Name: "count",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				table, ok := tengo.ToString(args[0])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "table", Expected: "string", Found: args[0].TypeName()}
				}
				n, err := store.Count(table)
				if err != nil {
					return &tengo.Error{Value: strObj(err.Error())}, nil
				}
				return intObj(int64(n)), nil
			},
		},
	}}
}

// ── request module ────────────────────────────────────────────────────────────

func newRequestModule(rc *RequestContext) tengo.Object {
	return &tengo.ImmutableMap{Value: map[string]tengo.Object{

		"param": &tengo.UserFunction{
			Name: "param",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				name, ok := tengo.ToString(args[0])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "name", Expected: "string", Found: args[0].TypeName()}
				}
				if rc.PathParams == nil {
					return tengo.UndefinedValue, nil
				}
				v, exists := rc.PathParams[name]
				if !exists {
					return tengo.UndefinedValue, nil
				}
				return strObj(v), nil
			},
		},

		"query": &tengo.UserFunction{
			Name: "query",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				name, ok := tengo.ToString(args[0])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "name", Expected: "string", Found: args[0].TypeName()}
				}
				if rc.QueryParams == nil {
					return tengo.UndefinedValue, nil
				}
				v, exists := rc.QueryParams[name]
				if !exists {
					return tengo.UndefinedValue, nil
				}
				return strObj(v), nil
			},
		},

		"body": &tengo.UserFunction{
			Name: "body",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 0 {
					return nil, tengo.ErrWrongNumArguments
				}
				if rc.Body == nil {
					return tengo.UndefinedValue, nil
				}
				obj, err := tengo.FromInterface(rc.Body)
				if err != nil {
					return nil, err
				}
				return obj, nil
			},
		},

		"header": &tengo.UserFunction{
			Name: "header",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				name, ok := tengo.ToString(args[0])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "name", Expected: "string", Found: args[0].TypeName()}
				}
				if rc.Headers == nil {
					return tengo.UndefinedValue, nil
				}
				v, exists := rc.Headers[strings.ToLower(name)]
				if !exists {
					return tengo.UndefinedValue, nil
				}
				return strObj(v), nil
			},
		},

		"method": &tengo.UserFunction{
			Name: "method",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 0 {
					return nil, tengo.ErrWrongNumArguments
				}
				return strObj(rc.Method), nil
			},
		},

		"auth": &tengo.UserFunction{
			Name: "auth",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 0 {
					return nil, tengo.ErrWrongNumArguments
				}
				if rc.Headers == nil {
					return tengo.UndefinedValue, nil
				}
				authHeader, exists := rc.Headers["authorization"]
				if !exists {
					return tengo.UndefinedValue, nil
				}
				const prefix = "Bearer "
				if !strings.HasPrefix(authHeader, prefix) {
					return tengo.UndefinedValue, nil
				}
				token := authHeader[len(prefix):]

				// Try to base64-decode and parse as JSON
				decoded, err := base64.StdEncoding.DecodeString(token)
				if err != nil {
					// Try URL-safe base64
					decoded, err = base64.RawURLEncoding.DecodeString(token)
				}
				if err == nil {
					var payload map[string]any
					if jsonErr := json.Unmarshal(decoded, &payload); jsonErr == nil {
						obj, fromErr := tengo.FromInterface(payload)
						if fromErr != nil {
							return nil, fromErr
						}
						return obj, nil
					}
				}

				// On failure return {"raw_token": TOKEN}
				obj, err := tengo.FromInterface(map[string]any{"raw_token": token})
				if err != nil {
					return nil, err
				}
				return obj, nil
			},
		},
	}}
}

// ── response module ───────────────────────────────────────────────────────────

func newResponseModule(capture *ResponseCapture) tengo.Object {
	return &tengo.ImmutableMap{Value: map[string]tengo.Object{

		"json": &tengo.UserFunction{
			Name: "json",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) < 1 || len(args) > 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				capture.Body = tengo.ToInterface(args[0])
				if len(args) == 2 {
					status, ok := tengo.ToInt(args[1])
					if !ok {
						return nil, tengo.ErrInvalidArgumentType{Name: "status", Expected: "int", Found: args[1].TypeName()}
					}
					capture.StatusCode = status
				} else {
					capture.StatusCode = 200
				}
				capture.Written = true
				return tengo.UndefinedValue, nil
			},
		},

		// "error" is a Tengo keyword; use "fail" in scripts: response.fail(status, msg)
		"fail": &tengo.UserFunction{
			Name: "fail",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				status, ok := tengo.ToInt(args[0])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "status", Expected: "int", Found: args[0].TypeName()}
				}
				msg, ok := tengo.ToString(args[1])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "msg", Expected: "string", Found: args[1].TypeName()}
				}
				capture.StatusCode = status
				capture.Body = map[string]any{"error": msg}
				capture.Written = true
				return tengo.UndefinedValue, nil
			},
		},

		"header": &tengo.UserFunction{
			Name: "header",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				name, ok := tengo.ToString(args[0])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "name", Expected: "string", Found: args[0].TypeName()}
				}
				val, ok := tengo.ToString(args[1])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "val", Expected: "string", Found: args[1].TypeName()}
				}
				if capture.Headers == nil {
					capture.Headers = make(map[string]string)
				}
				capture.Headers[name] = val
				return tengo.UndefinedValue, nil
			},
		},

		"redirect": &tengo.UserFunction{
			Name: "redirect",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				url, ok := tengo.ToString(args[0])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "url", Expected: "string", Found: args[0].TypeName()}
				}
				capture.StatusCode = 302
				if capture.Headers == nil {
					capture.Headers = make(map[string]string)
				}
				capture.Headers["Location"] = url
				capture.Written = true
				return tengo.UndefinedValue, nil
			},
		},
	}}
}

// ── date module ───────────────────────────────────────────────────────────────

var dateLayouts = []string{"2006-01-02", time.RFC3339}

func parseDate(s string) (time.Time, error) {
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse date: %q", s)
}

func newDateModule() tengo.Object {
	return &tengo.ImmutableMap{Value: map[string]tengo.Object{

		"now": &tengo.UserFunction{
			Name: "now",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 0 {
					return nil, tengo.ErrWrongNumArguments
				}
				return strObj(time.Now().UTC().Format(time.RFC3339)), nil
			},
		},

		"diff_days": &tengo.UserFunction{
			Name: "diff_days",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				aStr, ok := tengo.ToString(args[0])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "a", Expected: "string", Found: args[0].TypeName()}
				}
				bStr, ok := tengo.ToString(args[1])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "b", Expected: "string", Found: args[1].TypeName()}
				}
				tA, err := parseDate(aStr)
				if err != nil {
					return &tengo.Error{Value: strObj(err.Error())}, nil
				}
				tB, err := parseDate(bStr)
				if err != nil {
					return &tengo.Error{Value: strObj(err.Error())}, nil
				}
				diff := tB.Sub(tA)
				days := int64(diff.Hours() / 24)
				return intObj(days), nil
			},
		},

		"add_days": &tengo.UserFunction{
			Name: "add_days",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				dStr, ok := tengo.ToString(args[0])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "d", Expected: "string", Found: args[0].TypeName()}
				}
				n, ok := tengo.ToInt(args[1])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "n", Expected: "int", Found: args[1].TypeName()}
				}
				t, err := parseDate(dStr)
				if err != nil {
					return &tengo.Error{Value: strObj(err.Error())}, nil
				}
				result := t.AddDate(0, 0, n)
				return strObj(result.Format("2006-01-02")), nil
			},
		},

		"format": &tengo.UserFunction{
			Name: "format",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				dStr, ok := tengo.ToString(args[0])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "d", Expected: "string", Found: args[0].TypeName()}
				}
				fmtStr, ok := tengo.ToString(args[1])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "fmt", Expected: "string", Found: args[1].TypeName()}
				}
				t, err := parseDate(dStr)
				if err != nil {
					return &tengo.Error{Value: strObj(err.Error())}, nil
				}
				return strObj(t.Format(fmtStr)), nil
			},
		},
	}}
}

// ── crypto module ─────────────────────────────────────────────────────────────

func newCryptoModule() tengo.Object {
	return &tengo.ImmutableMap{Value: map[string]tengo.Object{

		"hash": &tengo.UserFunction{
			Name: "hash",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				s, ok := tengo.ToString(args[0])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "str", Expected: "string", Found: args[0].TypeName()}
				}
				h := sha256.Sum256([]byte(s))
				return strObj(hex.EncodeToString(h[:])), nil
			},
		},

		"uuid": &tengo.UserFunction{
			Name: "uuid",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 0 {
					return nil, tengo.ErrWrongNumArguments
				}
				id, err := uuid.NewRandom()
				if err != nil {
					// Fallback using crypto/rand manually
					b := make([]byte, 16)
					if _, err2 := rand.Read(b); err2 != nil {
						return nil, err2
					}
					b[6] = (b[6] & 0x0f) | 0x40
					b[8] = (b[8] & 0x3f) | 0x80
					return strObj(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])), nil
				}
				return strObj(id.String()), nil
			},
		},

		"random": &tengo.UserFunction{
			Name: "random",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				min, ok := tengo.ToInt(args[0])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "min", Expected: "int", Found: args[0].TypeName()}
				}
				max, ok := tengo.ToInt(args[1])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "max", Expected: "int", Found: args[1].TypeName()}
				}
				if max < min {
					return nil, fmt.Errorf("max (%d) must be >= min (%d)", max, min)
				}
				// Use crypto/rand for better randomness
				rangeSize := int64(max - min + 1)
				n, err := rand.Int(rand.Reader, big.NewInt(rangeSize))
				if err != nil {
					// fallback to math/rand
					return intObj(int64(min) + mathrand.Int63n(rangeSize)), nil
				}
				return intObj(int64(min) + n.Int64()), nil
			},
		},
	}}
}

// ── log module ────────────────────────────────────────────────────────────────

func newLogModule(bus *engine.Bus) tengo.Object {
	emitLog := func(level string) func(args ...tengo.Object) (tengo.Object, error) {
		return func(args ...tengo.Object) (tengo.Object, error) {
			if len(args) != 1 {
				return nil, tengo.ErrWrongNumArguments
			}
			msg, ok := tengo.ToString(args[0])
			if !ok {
				return nil, tengo.ErrInvalidArgumentType{Name: "msg", Expected: "string", Found: args[0].TypeName()}
			}
			bus.Publish(engine.Event{
				Type: engine.EventLogEmitted,
				Data: map[string]any{"level": level, "message": msg},
			})
			return tengo.UndefinedValue, nil
		}
	}

	return &tengo.ImmutableMap{Value: map[string]tengo.Object{
		"info":  &tengo.UserFunction{Name: "info", Value: emitLog("info")},
		"warn":  &tengo.UserFunction{Name: "warn", Value: emitLog("warn")},
		"error": &tengo.UserFunction{Name: "error", Value: emitLog("error")},
	}}
}
