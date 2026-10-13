package config

import (
	"os"
	"strconv"
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

// Int membaca env angka positif; bila kosong / tidak valid memakai def.
func Int(key string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// TrustedProxies dari TRUSTED_PROXIES. Kosong = tidak percaya proxy mana pun.
func TrustedProxies() []string {
	var out []string
	for _, p := range strings.Split(os.Getenv("TRUSTED_PROXIES"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
