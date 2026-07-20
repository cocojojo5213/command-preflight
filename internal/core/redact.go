package core

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	secretAssignment = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|auth(?:orization)?|token|secret|password|passwd|cookie|client[_-]?secret)\b(\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s,;]+)`)
	bearerToken      = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	urlSecret        = regexp.MustCompile(`(?i)([?&](?:token|key|secret|password|sig|signature|auth)=)[^&#\s]+`)
	emailValue       = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	publicURL        = regexp.MustCompile(`(?i)\b(?:https?|ftp)://[^\s"'<>]+`)
	publicEnvAssign  = regexp.MustCompile(`(?i)(?:\bexport\s+|\bset\s+)?[A-Z_][A-Z0-9_]{1,63}\s*=\s*(?:"[^"]*"|'[^']*'|[^\s,;]+)`)
	publicEnvRef     = regexp.MustCompile(`(?i)(?:\$\{[A-Z_][A-Z0-9_]*\}|\$env:[A-Z_][A-Z0-9_]*|%[A-Z_][A-Z0-9_]*%|\$[A-Z_][A-Z0-9_]*)`)
	publicWinPath    = regexp.MustCompile(`(?i)(?:[A-Z]:[\\/]|\\\\)[^\s"'<>|]+`)
	publicUnixPath   = regexp.MustCompile("(?i)(^|[\\s\\x60\"'(){}\\[\\]:=])(?:~?/|\\./|\\.\\./)[^\\s\\x60\"'<>),;\\]}]+")
	publicRelPath    = regexp.MustCompile("(?i)(^|[\\s\\x60\"'(){}\\[\\]:=])(?:[A-Z0-9_.-]+[\\\\/])+[A-Z0-9_.-]+")
	ipv4Value        = regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`)
	unixHome         = regexp.MustCompile(`(?i)(/home/|/users/)[^/\s]+`)
	windowsHome      = regexp.MustCompile(`(?i)([A-Za-z]:\\Users\\)[^\\\s]+`)
	uuidValue        = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b`)
	hexValue         = regexp.MustCompile(`(?i)\b0x[0-9a-f]+\b`)
	numericValue     = regexp.MustCompile(`\b\d{4,}\b`)
	pathValue        = regexp.MustCompile(`(?i)(?:[A-Za-z]:\\[^\s"']+|/(?:Users|home|tmp|var/tmp)/[^\s"']+)`)
)

// RedactSecrets removes common credential forms before an event can leave the host.
// It is deliberately conservative: callers should still treat the result as sensitive.
func RedactSecrets(input string) string {
	if input == "" {
		return input
	}
	redacted := bearerToken.ReplaceAllString(input, "Bearer [REDACTED]")
	redacted = secretAssignment.ReplaceAllString(redacted, `$1$2[REDACTED]`)
	redacted = urlSecret.ReplaceAllString(redacted, `$1[REDACTED]`)
	redacted = unixHome.ReplaceAllString(redacted, `$1<USER>`)
	redacted = windowsHome.ReplaceAllString(redacted, `$1<USER>`)
	return redacted
}

// RedactPublicText applies the same local secret and path protection to
// short, model-authored report text before it can leave the host. Reports are
// explanatory text, not a place to preserve exact command transcripts.
func RedactPublicText(input string) string {
	value := RedactSecrets(input)
	value = emailValue.ReplaceAllString(value, "<EMAIL>")
	value = publicURL.ReplaceAllString(value, "<URL>")
	value = publicEnvAssign.ReplaceAllStringFunc(value, func(match string) string {
		if strings.Contains(match, "[REDACTED]") {
			return "[REDACTED]"
		}
		return "<ENV>"
	})
	value = publicEnvRef.ReplaceAllString(value, "<ENV>")
	value = publicWinPath.ReplaceAllString(value, "<PATH>")
	value = publicUnixPath.ReplaceAllString(value, "$1<PATH>")
	value = publicRelPath.ReplaceAllString(value, "$1<PATH>")
	value = uuidValue.ReplaceAllString(value, "<UUID>")
	value = ipv4Value.ReplaceAllString(value, "<IP>")
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || r >= 0x20 {
			return r
		}
		return -1
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	return truncateUTF8(value, 1200)
}

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	const suffix = "..."
	if maxBytes <= len(suffix) {
		return suffix[:maxBytes]
	}
	cut := maxBytes - len(suffix)
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut] + suffix
}

func NormalizeCommand(command string) string {
	value := RedactSecrets(command)
	value = pathValue.ReplaceAllString(value, "<PATH>")
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 512 {
		value = value[:512] + "..."
	}
	return value
}

func NormalizeError(text string) string {
	value := RedactSecrets(text)
	value = pathValue.ReplaceAllString(value, "<PATH>")
	value = uuidValue.ReplaceAllString(value, "<UUID>")
	value = hexValue.ReplaceAllString(value, "<HEX>")
	value = numericValue.ReplaceAllString(value, "<N>")
	value = strings.ToLower(strings.Join(strings.Fields(value), " "))
	if len(value) > 768 {
		value = value[:768] + "..."
	}
	return value
}
