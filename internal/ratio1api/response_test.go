package ratio1api

import (
	"encoding/json"
	"strconv"
	"testing"
)

func TestExtractResult(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "double-encoded object",
			body:     `{"result":"{\"count\":1}"}`,
			expected: `{"count":1}`,
		},
		{
			name:     "quoted double-encoded object",
			body:     `{"result":"\"{\\\"count\\\":1}\""}`,
			expected: `{"count":1}`,
		},
		{
			name:     "direct object",
			body:     `{"result":{"keys":["a","b"]}}`,
			expected: `{"keys":["a","b"]}`,
		},
		{
			name:     "plain string",
			body:     `{"result":"hello"}`,
			expected: `"hello"`,
		},
		{
			name:     "null passthrough",
			body:     `null`,
			expected: `null`,
		},
		{
			name:     "empty body",
			body:     ``,
			expected: ``,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExtractResult([]byte(tc.body))
			if err != nil {
				t.Fatalf("ExtractResult returned error: %v", err)
			}
			if string(got) != tc.expected {
				t.Fatalf("ExtractResult mismatch: expected %q, got %q", tc.expected, string(got))
			}
		})
	}
}

func TestDecodeResult(t *testing.T) {
	var str string
	if err := json.Unmarshal([]byte(`"\"{\\\"count\\\":1}\""`), &str); err != nil {
		t.Fatalf("unmarshal debug string: %v", err)
	}
	if str != "\"{\\\"count\\\":1}\"" {
		t.Fatalf("unexpected intermediate string: %q", str)
	}
	decoded := str
	for i := 0; i < 4; i++ {
		unquoted, err := strconv.Unquote(decoded)
		if err != nil {
			break
		}
		decoded = unquoted
	}
	if decoded != "{\"count\":1}" {
		t.Fatalf("unexpected decoded string: %q", decoded)
	}
	var inner json.RawMessage
	if err := json.Unmarshal([]byte(decoded), &inner); err != nil {
		t.Fatalf("unmarshal inner debug string: %v", err)
	}
	if string(inner) != "{\"count\":1}" {
		t.Fatalf("unexpected inner raw: %q", string(inner))
	}

	body := []byte(`{"result":"{\"value\":\"ok\"}"}`)
	var payload struct {
		Value string `json:"value"`
	}
	if err := DecodeResult(body, &payload); err != nil {
		t.Fatalf("DecodeResult error: %v", err)
	}
	if payload.Value != "ok" {
		t.Fatalf("DecodeResult mismatch: got %q", payload.Value)
	}

	var nullPayload struct {
		Value string `json:"value"`
	}
	if err := DecodeResult(nil, &nullPayload); err != nil {
		t.Fatalf("DecodeResult nil body: %v", err)
	}

	var direct map[string]any
	if err := DecodeResult([]byte(`{"foo":1}`), &direct); err != nil {
		t.Fatalf("DecodeResult direct: %v", err)
	}
	if direct["foo"] != float64(1) {
		b, _ := json.Marshal(direct)
		t.Fatalf("DecodeResult direct mismatch: %s", string(b))
	}
}
