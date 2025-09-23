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
		// Boundary and edge cases
		{"escapedDollar", `{"x":"\\${FOO}"}`, true, false},                                                                      // escaped placeholder is still detected in value
		{"multipleInOne", `{"x":"${A} and ${B}"}`, true, false},                                                                 // multiple placeholders in one string
		{"deepNested", `{"a":{"b":{"c":{"d":{"e":"${DEEP}"}}}}}`, true, false},                                                  // deep nesting
		{"mixedTypes", `{"s":"ok","n":42,"b":true,"o":{"x":"${Y}"}}`, true, false},                                              // mixed data types
		{"emptyPlaceholder", `{"x":"${}"}`, false, false},                                                                       // empty placeholder not matched by current regex
		{"partialPlaceholder", `{"x":"${"}`, false, false},                                                                      // incomplete placeholder should not match
		{"dollarWithoutBrace", `{"x":"$FOO"}`, false, false},                                                                    // dollar without braces should not match
		{"placeholderInArray", `{"env":["HOME=/home","DB_URL=${DB_CONNECTION}"]}`, true, false},                                 // placeholder in array
		{"nestedArrayWithPlaceholder", `{"services":[{"name":"web","env":{"URL":"${SERVICE_URL}"}}]}`, true, false},             // nested array with placeholder
		{"multiplePlaceholdersDeep", `{"a":{"b":"${X}"},"c":{"d":{"e":"${Y}"}}}`, true, false},                                  // multiple placeholders at different depths
		{"placeholderAtRoot", `{"${ROOT_KEY}":"value"}`, false, false},                                                          // placeholder as key not detected by current implementation
		{"bracesInLiteral", `{"x":"this is {not} a placeholder"}`, false, false},                                                // braces without dollar should not match
		{"placeholderWithSpecialChars", `{"x":"${FOO.BAR_BAZ-123}"}`, true, false},                                              // placeholder with special characters
		{"longPlaceholder", `{"x":"${VERY_LONG_PLACEHOLDER_NAME_WITH_MANY_UNDERSCORES_AND_DOTS.AND.MORE.STUFF}"}`, true, false}, // very long placeholder
		{"adjacentPlaceholders", `{"x":"${A}${B}"}`, true, false},                                                               // adjacent placeholders without space
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
