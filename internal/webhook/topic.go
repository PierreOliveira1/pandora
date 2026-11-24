package webhook

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func eventTopic(hook Hook, header http.Header, body []byte) string {
	if hook.TopicHeader != "" {
		if value := header.Get(hook.TopicHeader); value != "" {
			return value
		}
	}
	if len(hook.TopicFields) == 0 || len(body) == 0 {
		return ""
	}
	var document any
	if err := json.Unmarshal(body, &document); err != nil {
		return ""
	}
	for _, path := range hook.TopicFields {
		value, ok := lookupPath(document, path)
		if !ok {
			continue
		}
		if text, ok := value.(string); ok {
			return text
		}
		return lastFieldName(path)
	}
	return ""
}

func lookupPath(document any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	current := document
	for _, segment := range strings.Split(path, ".") {
		switch node := current.(type) {
		case map[string]any:
			value, ok := node[segment]
			if !ok {
				return nil, false
			}
			current = value
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(node) {
				return nil, false
			}
			current = node[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func lastFieldName(path string) string {
	segments := strings.Split(path, ".")
	for i := len(segments) - 1; i >= 0; i-- {
		if _, err := strconv.Atoi(segments[i]); err != nil {
			return segments[i]
		}
	}
	return ""
}
