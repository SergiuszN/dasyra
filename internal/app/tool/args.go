package tool

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Args are the arguments of a tool call. Models fill every optional field with "", null or [],
// those are dropped on parsing, so a tool sees only what was really passed.
type Args map[string]any

func ParseArgs(arguments string) Args {
	var raw map[string]any
	_ = json.Unmarshal([]byte(arguments), &raw)
	args := Args{}
	for k, v := range raw {
		if !blank(v) {
			args[k] = v
		}
	}
	return args
}

func blank(v any) bool {
	switch v := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	}
	return false
}

func (a Args) Has(key string) bool {
	_, ok := a[key]
	return ok
}

// String is the trimmed text value, numbers are formatted.
func (a Args) String(key string) string {
	switch v := a[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

// Int reads numbers and numeric strings, 0 otherwise.
func (a Args) Int(key string) int64 {
	return toInt(a[key])
}

func toInt(v any) int64 {
	switch v := v.(type) {
	case float64:
		return int64(v)
	case string:
		n, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return int64(n)
	}
	return 0
}

// IntOr is Int with a default for a missing value.
func (a Args) IntOr(key string, def int) int {
	if !a.Has(key) {
		return def
	}
	return int(a.Int(key))
}

// Bool returns the value and whether it was passed.
func (a Args) Bool(key string) (value, ok bool) {
	switch v := a[key].(type) {
	case bool:
		return v, true
	case string:
		return v == "true" || v == "1", true
	case float64:
		return v != 0, true
	}
	return false, false
}

// Strings reads a list, a single value becomes a list of one.
func (a Args) Strings(key string) []string {
	var out []string
	for _, v := range list(a[key]) {
		out = append(out, Args{"v": v}.String("v"))
	}
	return out
}

// Objects are the items of an array of objects, each with its blank fields dropped like the top level.
func (a Args) Objects(key string) []Args {
	var out []Args
	for _, v := range list(a[key]) {
		item := Args{}
		if m, ok := v.(map[string]any); ok {
			for k, field := range m {
				if !blank(field) {
					item[k] = field
				}
			}
		}
		out = append(out, item)
	}
	return out
}

func (a Args) Ints(key string) []int64 {
	var out []int64
	for _, v := range list(a[key]) {
		out = append(out, toInt(v))
	}
	return out
}

func list(v any) []any {
	switch v := v.(type) {
	case nil:
		return nil
	case []any:
		return v
	}
	return []any{v}
}

// JSON is the arguments as stored with a tool call.
func (a Args) JSON() string {
	if len(a) == 0 {
		return "[]"
	}
	return toJSON(map[string]any(a))
}
