package model

import (
	"crypto/rand"
	"encoding/hex"
)

// NewID membuat ID acak dengan prefix, contoh: NewID("usr") -> "usr_9f2c1a7b3d4e5f60".
// Semua primary key di MedCredix bertipe string (VARCHAR(40)), jadi ID harus diisi aplikasi.
func NewID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}
