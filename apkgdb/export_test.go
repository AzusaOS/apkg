package apkgdb

import "testing"

func TestFormatSize(t *testing.T) {
	tests := []struct {
		input    uint64
		expected string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{500, "500 B"},
		{1024, "1024 B"},
		{1536, "1.50 kiB"},
		{2048, "2.00 kiB"},
		{1048576, "1024.00 kiB"},
		{1572864, "1.50 MiB"},
		{1073741824, "1024.00 MiB"},
		{1099511627776, "1024.00 GiB"},
	}

	for _, tt := range tests {
		result := formatSize(tt.input)
		if result != tt.expected {
			t.Errorf("formatSize(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
