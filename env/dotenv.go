package env

import (
	"fmt"
	"os"
	"strings"
	"unicode"
)

// LoadOptions controls how LoadDotEnvWithOptions applies parsed variables.
type LoadOptions struct {
	// OverrideExisting controls whether parsed variables replace existing
	// environment variables. The default behavior is false.
	OverrideExisting bool
}

// ParseError describes a malformed dotenv line.
type ParseError struct {
	Line int
	Text string
	Err  error
}

func (e *ParseError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("dotenv parse error on line %d: %v", e.Line, e.Err)
}

func (e *ParseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// LoadDotEnv parses a dotenv file and sets environment variables without
// overwriting variables that are already present in the process environment.
func LoadDotEnv(path string) error {
	return LoadDotEnvWithOptions(path, LoadOptions{})
}

// LoadDotEnvWithOptions parses a dotenv file and sets environment variables.
// It supports blank lines, comments, optional "export " prefixes, quoted
// values, empty values, and inline comments for unquoted values.
func LoadDotEnvWithOptions(path string, opts LoadOptions) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	for i, raw := range strings.Split(string(b), "\n") {
		key, val, ok, err := parseDotEnvLine(raw)
		if err != nil {
			return &ParseError{Line: i + 1, Text: raw, Err: err}
		}
		if !ok {
			continue
		}
		if !opts.OverrideExisting {
			if _, exists := os.LookupEnv(key); exists {
				continue
			}
		}
		if err := os.Setenv(key, val); err != nil {
			return fmt.Errorf("setenv %q failed: %w", key, err)
		}
	}
	return nil
}

func parseDotEnvLine(raw string) (key, val string, ok bool, err error) {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false, nil
	}

	if rest, hasExport := trimExportPrefix(line); hasExport {
		line = rest
	}

	eq := strings.IndexRune(line, '=')
	if eq < 0 {
		return "", "", false, fmt.Errorf("missing '=' separator")
	}

	key = strings.TrimSpace(line[:eq])
	if err := validateDotEnvKey(key); err != nil {
		return "", "", false, err
	}

	val, err = parseDotEnvValue(line[eq+1:])
	if err != nil {
		return "", "", false, err
	}
	return key, val, true, nil
}

func trimExportPrefix(line string) (string, bool) {
	if !strings.HasPrefix(line, "export") {
		return line, false
	}
	if len(line) == len("export") {
		return line, false
	}
	if !unicode.IsSpace(rune(line[len("export")])) {
		return line, false
	}
	return strings.TrimSpace(line[len("export"):]), true
}

func validateDotEnvKey(key string) error {
	if key == "" {
		return fmt.Errorf("empty key")
	}
	for _, r := range key {
		if r == '=' || r == 0 {
			return fmt.Errorf("invalid key %q", key)
		}
		if unicode.IsSpace(r) {
			return fmt.Errorf("invalid key %q", key)
		}
	}
	return nil
}

func parseDotEnvValue(raw string) (string, error) {
	trimmedLeft := strings.TrimLeftFunc(raw, unicode.IsSpace)
	if trimmedLeft == "" {
		return "", nil
	}

	switch trimmedLeft[0] {
	case '#':
		if len(trimmedLeft) != len(raw) {
			return "", nil
		}
		return strings.TrimSpace(trimmedLeft), nil
	case '\'':
		return parseSingleQuotedValue(trimmedLeft)
	case '"':
		return parseDoubleQuotedValue(trimmedLeft)
	default:
		return parseUnquotedValue(trimmedLeft), nil
	}
}

func parseSingleQuotedValue(raw string) (string, error) {
	end := strings.Index(raw[1:], "'")
	if end < 0 {
		return "", fmt.Errorf("unterminated single-quoted value")
	}
	value := raw[1 : 1+end]
	if err := validateQuotedTrailing(raw[1+end+1:]); err != nil {
		return "", err
	}
	return value, nil
}

func parseDoubleQuotedValue(raw string) (string, error) {
	var b strings.Builder
	for i := 1; i < len(raw); i++ {
		switch raw[i] {
		case '\\':
			if i+1 >= len(raw) {
				return "", fmt.Errorf("unterminated escape sequence in double-quoted value")
			}
			i++
			switch raw[i] {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case '\\':
				b.WriteByte('\\')
			case '"':
				b.WriteByte('"')
			default:
				b.WriteByte(raw[i])
			}
		case '"':
			if err := validateQuotedTrailing(raw[i+1:]); err != nil {
				return "", err
			}
			return b.String(), nil
		default:
			b.WriteByte(raw[i])
		}
	}
	return "", fmt.Errorf("unterminated double-quoted value")
}

func validateQuotedTrailing(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return nil
	}
	return fmt.Errorf("unexpected trailing content after quoted value")
}

func parseUnquotedValue(raw string) string {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '#' {
			continue
		}
		if i > 0 && unicode.IsSpace(rune(raw[i-1])) {
			return strings.TrimSpace(raw[:i])
		}
	}
	return strings.TrimSpace(raw)
}
