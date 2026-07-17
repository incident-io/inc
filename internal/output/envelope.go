package output

import (
	"encoding/json"
	"fmt"
)

// UnwrapEnvelope extracts a named key from a JSON object.
// e.g., UnwrapEnvelope(body, "incidents") extracts the array from {"incidents": [...], "pagination_meta": {...}}
func UnwrapEnvelope(body []byte, key string) (json.RawMessage, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse response envelope: %w", err)
	}
	data, ok := envelope[key]
	if !ok {
		return nil, fmt.Errorf("response missing expected key %q", key)
	}
	return data, nil
}
