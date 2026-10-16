// Package storage menyimpan berkas unggahan (PDF/DOC/DOCX) terenkripsi AES-256-GCM di disk.
package storage

import (
	"archive/zip"
	"bytes"
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

	"github.com/spkd/medcredix/internal/config"
)

// MaxSize = batas ukuran satu berkas (5 MB).
const MaxSize = 5 << 20

const (
	mimePDF  = "application/pdf"
	mimeDOC  = "application/msword"
	mimeDOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
)

// Jenis berkas yang diterima: hanya PDF dan dokumen Word (DOC/DOCX).
// Dicek dari isi berkas (bukan dari nama/ekstensi), jadi gambar yang diganti namanya tetap ditolak.
var allowed = map[string]string{
	mimePDF:  ".pdf",
	mimeDOC:  ".doc",
	mimeDOCX: ".docx",
}

var (
	ErrTooLarge = errors.New("Ukuran berkas melebihi 5 MB.")
	ErrType     = errors.New("Jenis berkas harus PDF atau Word (DOC/DOCX).")
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
	} else if config.IsProduction() {
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
	mime = detectType(data)
	if mime == "" {
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

// oleMagic = tanda awal berkas Office lama (DOC/XLS/PPT memakai format OLE yang sama).
var oleMagic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

// wordStream = nama stream "WordDocument" (UTF-16LE) yang hanya ada di berkas .doc, bukan XLS/PPT.
var wordStream = []byte("W\x00o\x00r\x00d\x00D\x00o\x00c\x00u\x00m\x00e\x00n\x00t\x00")

// detectType mengenali PDF, DOC, dan DOCX dari isi berkas. Jenis lain mengembalikan "".
func detectType(data []byte) string {
	switch {
	case http.DetectContentType(data) == mimePDF:
		return mimePDF
	case bytes.HasPrefix(data, oleMagic):
		if bytes.Contains(data, wordStream) {
			return mimeDOC
		}
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		// DOCX = arsip ZIP yang berisi word/document.xml (XLSX/PPTX/ZIP biasa ditolak).
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return ""
		}
		for _, f := range zr.File {
			if f.Name == "word/document.xml" {
				return mimeDOCX
			}
		}
	}
	return ""
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