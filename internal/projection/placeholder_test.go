package projection

import "testing"

func TestHasUnresolvedPlaceholders(t *testing.T) {
	tcs := []struct {
		name    string
		json    string
		wantHas bool
		wantErr bool
	}{
		{"empty", ``, false, false},
		{"noPlaceholders", `{"a":"b","n":123,"arr":[1,2,3]}`, false, false},
		{"simple", `{"x":"${FOO}"}`, true, false},
		{"nested", `{"a":{"b":["ok", {"c":"v${BAR}"}]}}`, true, false},
		{"array", `["ok","${X}","y"]`, true, false},
		{"invalidJSON", `{"x":`, true, true}, // fail-safe: treat as unresolved
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			has, err := HasUnresolvedPlaceholders([]byte(tc.json))
			if has != tc.wantHas {
				t.Fatalf("has=%v, want=%v", has, tc.wantHas)
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
