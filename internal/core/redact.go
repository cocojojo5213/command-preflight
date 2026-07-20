package core

import (
	"regexp"
	"strings"
)

var (
	secretAssignment = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|auth(?:orization)?|token|secret|password|passwd|cookie|client[_-]?secret)\b(\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s,;]+)`)
	bearerToken      = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	urlSecret        = regexp.MustCompile(`(?i)([?&](?:token|key|secret|password|sig|signature|auth)=)[^&#\s]+`)
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
