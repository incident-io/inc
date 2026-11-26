package output

import (
	"bytes"
	"testing"
)

func TestPrintJSONFieldsPreserveValues(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{"large integer", `{"keep":9007199254740993,"drop":"hidden"}`, `{"keep":9007199254740993}`},
		{"negative integer", `{"keep":-9007199254740993,"drop":"hidden"}`, `{"keep":-9007199254740993}`},
		{"precise decimal", `{"keep":0.123456789012345678901,"drop":"hidden"}`, `{"keep":0.123456789012345678901}`},
		{"nested values", `[{"keep":{"values":[9007199254740993,0.123456789012345678901]},"drop":"hidden"}]`, `[{"keep":{"values":[9007199254740993,0.123456789012345678901]}}]`},
		{"large exponent", `{"keep":1e400,"drop":"hidden"}`, `{"keep":1e400}`},
		{"scalar controls", `[{"keep":"text","drop":1},{"keep":true,"drop":2},{"keep":null,"drop":3}]`, `[{"keep":"text"},{"keep":true},{"keep":null}]`},
		{"missing field", `{"drop":"hidden"}`, `{}`},
		{"empty array", `[]`, `[]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bytes.Buffer
			if err := Print(&got, "json", "", "keep", []byte(tt.data)); err != nil {
				t.Fatal(err)
			}
			if want := tt.want + "\n"; got.String() != want {
				t.Errorf("Print() = %s, want %s", got.String(), want)
			}
		})
	}
}
