package httptool

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type partKind int

const (
	literalPart partKind = iota
	paramPart
	envPart
)

type part struct {
	kind partKind
	text string
	name string
}

func parseTemplate(raw string) ([]part, error) {
	parts := make([]part, 0)
	remaining := raw
	for {
		open := strings.Index(remaining, "{{")
		if open < 0 {
			if remaining != "" {
				parts = append(parts, part{kind: literalPart, text: remaining})
			}
			break
		}
		if open > 0 {
			parts = append(parts, part{kind: literalPart, text: remaining[:open]})
		}
		rest := remaining[open+2:]
		close := strings.Index(rest, "}}")
		if close < 0 {
			return nil, errors.New("invalid template: missing }}")
		}
		token := rest[:close]
		if strings.HasPrefix(token, "env.") {
			name := strings.TrimPrefix(token, "env.")
			if !validEnvName(name) {
				return nil, errors.New("invalid account value: use {{env.NAME}}")
			}
			parts = append(parts, part{kind: envPart, name: name})
		} else {
			if token == "" {
				return nil, errors.New("invalid template: empty parameter")
			}
			parts = append(parts, part{kind: paramPart, name: token})
		}
		remaining = rest[close+2:]
	}
	return parts, nil
}

func validEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' {
			continue
		}
		if i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

func expandTemplate(parts []part, args map[string]any, env map[string]string, escape func(string) string) (string, string, error) {
	var b strings.Builder
	for _, p := range parts {
		switch p.kind {
		case literalPart:
			b.WriteString(p.text)
		case paramPart:
			value, ok := args[p.name]
			if !ok || value == nil {
				return "", p.name, nil
			}
			text := valueText(value)
			if escape != nil {
				text = escape(text)
			}
			b.WriteString(text)
		case envPart:
			value, ok := env[p.name]
			if !ok {
				return "", "", fmt.Errorf("account value %s is not set", p.name)
			}
			if escape != nil {
				value = escape(value)
			}
			b.WriteString(value)
		}
	}
	return b.String(), "", nil
}

func valueText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int8:
		return strconv.FormatInt(int64(typed), 10)
	case int16:
		return strconv.FormatInt(int64(typed), 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint:
		return strconv.FormatUint(uint64(typed), 10)
	case uint8:
		return strconv.FormatUint(uint64(typed), 10)
	case uint16:
		return strconv.FormatUint(uint64(typed), 10)
	case uint32:
		return strconv.FormatUint(uint64(typed), 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case []string:
		return strings.Join(typed, ",")
	case []any:
		values := make([]string, len(typed))
		for i, item := range typed {
			values[i] = valueText(item)
		}
		return strings.Join(values, ",")
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
