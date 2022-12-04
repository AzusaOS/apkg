package main

import "runtime/debug"

var DATE_TAG = "unknown"

func init() {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}

	for _, setting := range bi.Settings {
		switch setting.Key {
		case "vcs.time":
			if DATE_TAG != "unknown" {
				break
			}
			// {Key:vcs.time Value:2022-06-01T01:55:46Z}
			v := make([]byte, 0, 14) // normal length
			for _, c := range setting.Value {
				if c >= '0' && c <= '9' {
					v = append(v, byte(c))
				}
			}
			if len(v) == 14 {
				DATE_TAG = string(v)
			}
		}
	}
}
