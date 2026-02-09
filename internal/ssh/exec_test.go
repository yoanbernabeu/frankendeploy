package ssh

import (
	"bytes"
	"strings"
	"testing"
)

func TestStreamWithPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		prefix   string
		expected []string
	}{
		{
			name:     "single line",
			input:    "hello world\n",
			prefix:   "[app] ",
			expected: []string{"[app] hello world"},
		},
		{
			name:     "multiple lines",
			input:    "line1\nline2\nline3\n",
			prefix:   "> ",
			expected: []string{"> line1", "> line2", "> line3"},
		},
		{
			name:     "empty prefix",
			input:    "test\n",
			prefix:   "",
			expected: []string{"test"},
		},
		{
			name:     "empty input",
			input:    "",
			prefix:   "[p] ",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			var buf bytes.Buffer

			streamWithPrefix(reader, &buf, tt.prefix)

			output := buf.String()
			for _, exp := range tt.expected {
				if !strings.Contains(output, exp) {
					t.Errorf("output %q does not contain expected %q", output, exp)
				}
			}
		})
	}
}

func TestStreamWithPrefix_LargeInput(t *testing.T) {
	// Test with data larger than the 1024-byte buffer
	longLine := strings.Repeat("x", 2000) + "\n"
	reader := strings.NewReader(longLine)
	var buf bytes.Buffer

	streamWithPrefix(reader, &buf, "[p] ")

	output := buf.String()
	if !strings.Contains(output, "[p] ") {
		t.Error("expected prefix in output")
	}
}
