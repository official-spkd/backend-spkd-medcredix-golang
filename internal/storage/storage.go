// Package storage menyimpan berkas unggahan (PDF/JPG/PNG) terenkripsi AES-256-GCM di disk.
package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/spkd/medcredix/internal/appenv"
)

// MaxSize = batas ukuran satu berkas (10 MB).
const MaxSize = 10 << 20

// Jenis berkas yang diterima, dicek dari isi berkas (bukan dari nama/ekstensi).
var allowed = map[string]string{
	"application/pdf": ".pdf",
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
}

var (
	ErrTooLarge = errors.New("Ukuran berkas melebihi 10 MB.")
	ErrType     = errors.New("Jenis berkas harus PDF, JPG, atau PNG.")
	ErrEmpty    = errors.New("Berkas kosong.")
	ErrNotFound = errors.New("Berkas tidak ditemukan di penyimpanan.")

	keyPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
	dir        string
	aead       cipher.AEAD
)

// Init membaca UPLOAD_DIR dan FILE_ENCRYPTION_KEY. Wajib dipanggil sekali saat start.
// Di produksi FILE_ENCRYPTION_KEY wajib diisi (32 byte, base64 atau hex).
func Init() error {
	dir = os.Getenv("UPLOAD_DIR")
	if dir == "" {
		dir = "uploads"
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("gagal membuat folder UPLOAD_DIR %q: %w", dir, err)
	}

	var key []byte
	if raw := os.Getenv("FILE_ENCRYPTION_KEY"); raw != "" {
		k, err := decodeKey(raw)
		if err != nil {
			return err
		}
		key = k
	} else if appenv.IsProduction() {
		return errors.New("FILE_ENCRYPTION_KEY wajib diisi saat APP_ENV=production (32 byte, base64)")
	} else {
		// Dev saja: kunci diturunkan dari JWT_SECRET supaya tidak perlu setting tambahan.
		log.Println("FILE_ENCRYPTION_KEY kosong, memakai kunci dev (jangan dipakai di produksi)")
		sum := sha256.Sum256([]byte("medcredix-file-dev:" + os.Getenv("JWT_SECRET")))
		key = sum[:]
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err = cipher.NewGCM(block)
	return err
}

func decodeKey(raw string) ([]byte, error) {
	if k, err := base64.StdEncoding.DecodeString(raw); err == nil && len(k) == 32 {
		return k, nil
	}
	if k, err := hex.DecodeString(raw); err == nil && len(k) == 32 {
		return k, nil
	}
	return nil, errors.New("FILE_ENCRYPTION_KEY harus 32 byte (base64 44 karakter atau hex 64 karakter)")
}

// Save membaca berkas (maks MaxSize), memeriksa jenisnya, lalu menyimpannya terenkripsi.
// Mengembalikan kunci penyimpanan, MIME, dan ukuran asli.
func Save(r io.Reader) (key, mime string, size int, err error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxSize+1))
	if err != nil {
		return "", "", 0, err
	}
	if len(data) == 0 {
		return "", "", 0, ErrEmpty
	}
	if len(data) > MaxSize {
		return "", "", 0, ErrTooLarge
	}
	mime = http.DetectContentType(data)
	if _, ok := allowed[mime]; !ok {
		return "", "", 0, ErrType
	}

	id := make([]byte, 16)
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(id); err != nil {
		return "", "", 0, err
	}
	if _, err := rand.Read(nonce); err != nil {
		return "", "", 0, err
	}
	key = hex.EncodeToString(id)
	// Kunci penyimpanan ikut jadi "additional data": berkas tidak bisa ditukar nama.
	sealed := aead.Seal(nonce, nonce, data, []byte(key))

	tmp := filepath.Join(dir, key+".tmp")
	if err := os.WriteFile(tmp, sealed, 0o600); err != nil {
		return "", "", 0, err
	}
	if err := os.Rename(tmp, filepath.Join(dir, key+".bin")); err != nil {
		os.Remove(tmp)
		return "", "", 0, err
	}
	return key, mime, len(data), nil
}

// Open membaca dan mendekripsi berkas.
func Open(key string) ([]byte, error) {
	if !keyPattern.MatchString(key) {
		return nil, ErrNotFound
	}
	sealed, err := os.ReadFile(filepath.Join(dir, key+".bin"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	n := aead.NonceSize()
	if len(sealed) < n {
		return nil, errors.New("berkas rusak")
	}
	return aead.Open(nil, sealed[:n], sealed[n:], []byte(key))
}

// Remove menghapus berkas lama (dipakai saat diganti). Kesalahan diabaikan.
func Remove(key string) {
	if keyPattern.MatchString(key) {
		os.Remove(filepath.Join(dir, key+".bin"))
	}
}

// Ext memberi ekstensi sesuai MIME, untuk nama unduhan.
func Ext(mime string) string { return allowed[mime] }

// IsUserError: kesalahan yang aman ditampilkan ke pengguna (400).
func IsUserError(err error) bool {
	return errors.Is(err, ErrTooLarge) || errors.Is(err, ErrType) || errors.Is(err, ErrEmpty)
}
