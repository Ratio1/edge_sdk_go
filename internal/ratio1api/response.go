package ratio1api

import (
	"bytes"
	"encoding/json"
	"strconv"
)

// ExtractResult unwraps Ratio1 API responses, returning the JSON payload stored
// under the "result" field. If no such field exists the original body is
// returned. When the "result" field is a JSON-encoded string, ExtractResult
// parses the inner JSON document so callers receive the decoded payload.
func ExtractResult(body []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, nil
	}

	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(trimmed, &envelope); err != nil || envelope.Result == nil {
		// Body is either not an object or does not include a result field.
		return append([]byte(nil), trimmed...), nil
	}

	// If the result is a JSON string, attempt to decode the inner payload.
	var asString string
	if err := json.Unmarshal(envelope.Result, &asString); err == nil {
		decoded := asString
		for i := 0; i < 4; i++ {
			unquoted, err := strconv.Unquote(decoded)
			if err != nil {
				break
			}
			decoded = unquoted
		}
		var inner json.RawMessage
		if err := json.Unmarshal([]byte(decoded), &inner); err == nil {
			return append([]byte(nil), inner...), nil
		}
		// Not an encoded JSON document; fall back to the original JSON string.
	}

	return append([]byte(nil), envelope.Result...), nil
}

// DecodeResult decodes the JSON payload obtained via ExtractResult into out.
// When the response body is empty, out is populated with a JSON null.
func DecodeResult(body []byte, out any) error {
	payload, err := ExtractResult(body)
	if err != nil {
		return err
	}
	if len(payload) == 0 {
		payload = []byte("null")
	}
	return json.Unmarshal(payload, out)
}
