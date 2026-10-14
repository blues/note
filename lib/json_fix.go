package notelib

import (
	"encoding/json"
	"fmt"
	"strings"
)

// doesJSONParse reports whether s decodes with json.Unmarshal.
func doesJSONParse(s string) bool {
	// nosemgrep: go.lang.security.deserialization.unsafe-deserialization-interface.go-unsafe-deserialization-interface -- no unsafe deserialization here: this only asks whether the string parses as JSON and throws the decoded value away
	var v interface{}
	return json.Unmarshal([]byte(s), &v) == nil
}

// FixCorruptJSON attempts to fix single-character corruptions in JSON data
// It tries replacing each character with valid JSON structure characters
// and returns the first successful parse
func FixCorruptJSON(corruptData string) (string, error) {
	// First, try to parse the JSON as-is
	if doesJSONParse(corruptData) {
		// JSON is already valid
		return corruptData, nil
	}

	// Define possible replacement characters
	// Include JSON structure chars and digits
	replacements := []rune{'{', '}', '[', ']', ':', ',', '"', ' ', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9'}

	// Try fixing by replacing each character
	runes := []rune(corruptData)
	for i := 0; i < len(runes); i++ {
		original := runes[i]

		// Skip if character is already a valid ASCII character that's likely correct
		// This optimization helps focus on actual corruptions
		if original >= 32 && original <= 126 {
			// Try replacements for this position
			for _, replacement := range replacements {
				if replacement == original {
					continue // Skip if same character
				}

				// Make the replacement
				runes[i] = replacement
				candidate := string(runes)

				// Try to parse the modified JSON
				if doesJSONParse(candidate) {
					// Success! Return the fixed JSON
					return candidate, nil
				}

				// Restore original character for next attempt
				runes[i] = original
			}
		} else {
			// For non-ASCII or control characters, try all replacements
			for _, replacement := range replacements {
				runes[i] = replacement
				candidate := string(runes)

				if doesJSONParse(candidate) {
					return candidate, nil
				}
			}
			// Restore original
			runes[i] = original
		}
	}

	// If no single-character fix works, try some common patterns
	// Sometimes the corruption might be a missing character rather than wrong character

	// Try inserting characters at positions where they might be missing
	for i := 0; i <= len(runes); i++ {
		for _, char := range replacements {
			// Insert character at position i
			var candidate string
			if i == 0 {
				candidate = string(char) + corruptData
			} else if i == len(runes) {
				candidate = corruptData + string(char)
			} else {
				candidate = corruptData[:i] + string(char) + corruptData[i:]
			}

			if doesJSONParse(candidate) {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("unable to fix JSON with single-character correction")
}

// FixCorruptJSONVerbose is like FixCorruptJSON but returns details about what was fixed
func FixCorruptJSONVerbose(corruptData string) (fixed string, changeDescription string, err error) {
	// First, try to parse the JSON as-is
	if doesJSONParse(corruptData) {
		return corruptData, "JSON was already valid", nil
	}

	replacements := []rune{'{', '}', '[', ']', ':', ',', '"', ' ', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9'}
	runes := []rune(corruptData)

	for i := 0; i < len(runes); i++ {
		original := runes[i]

		for _, replacement := range replacements {
			if replacement == original && original >= 32 && original <= 126 {
				continue
			}

			runes[i] = replacement
			candidate := string(runes)

			if doesJSONParse(candidate) {
				desc := fmt.Sprintf("Replaced character at position %d: '%c' (0x%X) -> '%c'",
					i, original, original, replacement)
				if original < 32 || original > 126 {
					desc = fmt.Sprintf("Replaced character at position %d: 0x%X -> '%c'",
						i, original, replacement)
				}
				return candidate, desc, nil
			}

			runes[i] = original
		}
	}

	// Try insertions
	for i := 0; i <= len(runes); i++ {
		for _, char := range replacements {
			var candidate string
			if i == 0 {
				candidate = string(char) + corruptData
			} else if i == len(runes) {
				candidate = corruptData + string(char)
			} else {
				candidate = corruptData[:i] + string(char) + corruptData[i:]
			}

			if doesJSONParse(candidate) {
				desc := fmt.Sprintf("Inserted '%c' at position %d", char, i)
				return candidate, desc, nil
			}
		}
	}

	return "", "", fmt.Errorf("unable to fix JSON with single-character correction")
}

// TryFixWithContext attempts to fix JSON using context clues
// This is a more intelligent version that looks for patterns
func TryFixWithContext(corruptData string) (string, error) {
	// First try the basic fix
	if fixed, err := FixCorruptJSON(corruptData); err == nil {
		return fixed, nil
	}

	// Look for specific patterns that might help
	// Pattern 1: Missing colons after keys (common corruption)
	if strings.Contains(corruptData, `":"`) {
		// Look for patterns like `"key"X` where X should be :
		runes := []rune(corruptData)
		inString := false
		escaped := false

		for i := 0; i < len(runes)-1; i++ {
			if runes[i] == '\\' && !escaped {
				escaped = true
				continue
			}

			if runes[i] == '"' && !escaped {
				inString = !inString
			}

			// If we just closed a string and next char isn't : or , or } or ]
			if !inString && i > 0 && runes[i] == '"' && runes[i-1] != '\\' {
				if i+1 < len(runes) && runes[i+1] != ':' && runes[i+1] != ',' &&
					runes[i+1] != '}' && runes[i+1] != ']' && runes[i+1] != ' ' {
					// Try replacing next character with :
					original := runes[i+1]
					runes[i+1] = ':'
					candidate := string(runes)

					if doesJSONParse(candidate) {
						return candidate, nil
					}
					runes[i+1] = original
				}
			}

			escaped = false
		}
	}

	return "", fmt.Errorf("unable to fix JSON even with context analysis")
}
