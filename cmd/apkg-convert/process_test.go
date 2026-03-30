package main

import (
	"testing"
)

func TestParsePackageFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantCat  string
		wantName string
		wantSub  string
		wantVer  []string
		wantOS   string
		wantArch string
		wantErr  bool
	}{
		{
			name:     "valid full filename",
			input:    "core.foobar.libs.1.2.3.linux.amd64",
			wantCat:  "core",
			wantName: "foobar",
			wantSub:  "libs",
			wantVer:  []string{"1", "2", "3"},
			wantOS:   "linux",
			wantArch: "amd64",
		},
		{
			name:     "valid minimal version",
			input:    "x11.libdrm.dev.2.linux.arm64",
			wantCat:  "x11",
			wantName: "libdrm",
			wantSub:  "dev",
			wantVer:  []string{"2"},
			wantOS:   "linux",
			wantArch: "arm64",
		},
		{
			name:     "valid no version parts",
			input:    "azusa.apkg.core.linux.amd64",
			wantCat:  "azusa",
			wantName: "apkg",
			wantSub:  "core",
			wantVer:  nil,
			wantOS:   "linux",
			wantArch: "amd64",
		},
		{
			name:    "too few components - 4",
			input:   "a.b.c.d",
			wantErr: true,
		},
		{
			name:    "too few components - 3",
			input:   "a.b.c",
			wantErr: true,
		},
		{
			name:    "too few components - 1",
			input:   "noperiods",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat, name, subcat, ver, osStr, archStr, _, err := parsePackageFilename(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for input %q, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cat != tt.wantCat {
				t.Errorf("cat = %q, want %q", cat, tt.wantCat)
			}
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
			if subcat != tt.wantSub {
				t.Errorf("subcat = %q, want %q", subcat, tt.wantSub)
			}
			if osStr != tt.wantOS {
				t.Errorf("os = %q, want %q", osStr, tt.wantOS)
			}
			if archStr != tt.wantArch {
				t.Errorf("arch = %q, want %q", archStr, tt.wantArch)
			}
			if len(ver) != len(tt.wantVer) {
				t.Errorf("version = %v, want %v", ver, tt.wantVer)
			} else {
				for i, v := range ver {
					if v != tt.wantVer[i] {
						t.Errorf("version[%d] = %q, want %q", i, v, tt.wantVer[i])
					}
				}
			}
		})
	}
}
