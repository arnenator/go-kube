package resources

import (
	"math/rand"
	"strings"
)

var (
	allowedChars    = []byte("abcdefghijklmnopqrstuvwxyz0123456789")
	allowedCharsLen = len(allowedChars)
)

func randomName(length int, prefixes []string) string {
	builder := strings.Builder{}

	builder.WriteString(strings.Join(prefixes, "-"))
	builder.WriteString("-")

	remainingLength := length - builder.Len()
	if remainingLength <= 0 {
		return builder.String()
	}

	for i := 0; i < remainingLength; i++ {
		builder.WriteByte(allowedChars[rand.Intn(allowedCharsLen)])
	}

	return builder.String()
}
