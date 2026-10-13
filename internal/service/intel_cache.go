package service

import (
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/scoring"
)

// Bootstrap, dashboard dan halaman lain menghitung skor yang sama berkali-kali (±25 query).
// Hasil disimpan sebentar per RS, lalu dibuang setiap ada perubahan data lewat API
// (lihat middleware/score_cache.go). Perubahan langsung via DBeaver terlihat maks. intelTTL kemudian.
const intelTTL = 30 * time.Second

type intelEntry struct {
	in    scoring.Intelligence
	fe    map[string]interface{}
	at    time.Time
	epoch uint64
}

var (
	intelMu     sync.Mutex
	intelCache  = map[string]intelEntry{}
	intelEpoch  uint64
	intelFlight = map[string]*intelCall{} // hitungan yang sedang berjalan, per RS+epoch
)

// intelCall: satu hitungan skor yang ditunggu bersama oleh request lain (cegah cache stampede).
type intelCall struct {
	wg  sync.WaitGroup
	in  scoring.Intelligence
	fe  map[string]interface{}
	err error
}

// InvalidateIntel membuang semua cache skor (dipanggil setelah request yang mengubah data).
func InvalidateIntel() {
	intelMu.Lock()
	intelEpoch++
	intelCache = map[string]intelEntry{}
	intelMu.Unlock()
}

// GetIntel menghitung intel satu RS (scoring engine) dan memetakannya ke bentuk frontend.
// Kalau banyak request datang bersamaan saat cache kosong, hanya satu yang menghitung,
// sisanya menunggu hasil yang sama. Hasil dipakai bersama: pemanggil hanya boleh membaca.
func GetIntel(db *gorm.DB, hid string, toFrontend func(*gorm.DB, string, scoring.Intelligence) map[string]interface{}) (scoring.Intelligence, map[string]interface{}, error) {
	intelMu.Lock()
	e, hit := intelCache[hid]
	if hit && time.Since(e.at) < intelTTL {
		intelMu.Unlock()
		return e.in, e.fe, nil
	}
	epoch := intelEpoch
	key := hid + "#" + strconv.FormatUint(epoch, 10)
	if c, ok := intelFlight[key]; ok {
		intelMu.Unlock()
		c.wg.Wait()
		return c.in, c.fe, c.err
	}
	c := &intelCall{}
	c.wg.Add(1)
	intelFlight[key] = c
	intelMu.Unlock()

	defer func() {
		intelMu.Lock()
		delete(intelFlight, key)
		if c.err == nil && c.fe != nil && epoch == intelEpoch { // jangan simpan hasil dari sebelum data berubah
			intelCache[hid] = intelEntry{in: c.in, fe: c.fe, at: time.Now(), epoch: epoch}
		}
		intelMu.Unlock()
		c.wg.Done()
	}()
	c.in, c.err = scoring.ComputeIntel(db, hid)
	if c.err == nil {
		c.fe = toFrontend(db, hid, c.in)
	}
	return c.in, c.fe, c.err
}
