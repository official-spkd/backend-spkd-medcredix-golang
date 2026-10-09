package appenv

import (
	"os"
	"strings"
)

// IsProduction bernilai true bila APP_ENV=production.
func IsProduction() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))) {
	case "production", "prod":
		return true
	}
	return false
}

// Bool membaca env bernilai true/1/yes.
func Bool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "y", "on":
		return true
	}
	return false
}
