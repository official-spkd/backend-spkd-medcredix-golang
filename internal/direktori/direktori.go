// Package direktori: daftar rumah sakit di Indonesia untuk form Tambah RS.
//
// Sumber: data SIRS Kemenkes RI yang dihimpun repo github.com/fraankiids/dashboard-rs-ri-lastupdated2024
// (pembaruan 2024, 3.155 RS). Kab/kota sudah dicocokkan ke kode wilayah Kepmendagri 2025 (paket wilayah),
// jadi tersisa 3.150 RS; 5 baris dengan kolom bergeser di data sumber dilewati.
// Format rumah_sakit.tsv: kode_rs, nama, kode_kab_kota, kelas, jenis, kepemilikan, tempat_tidur, alamat.
// Data ini hanya untuk isian awal: kelas dan TT bisa sudah berubah, jadi tetap bisa diubah di form.
package direktori

import (
	_ "embed"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed rumah_sakit.tsv
var raw string

// Hospital satu RS dari data Kemenkes.
type Hospital struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	CityCode  string `json:"cityCode"`
	Class     string `json:"class"`
	Type      string `json:"type"`
	Ownership string `json:"ownership"`
	Beds      int    `json:"beds"`
	Address   string `json:"address"`
}

var (
	once   sync.Once
	byCity map[string][]Hospital
	total  int
)

func load() {
	byCity = make(map[string][]Hospital, 520)
	for _, line := range strings.Split(raw, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(f) < 8 {
			continue
		}
		beds, _ := strconv.Atoi(f[6])
		h := Hospital{Code: f[0], Name: f[1], CityCode: f[2], Class: f[3], Type: f[4], Ownership: f[5], Beds: beds, Address: f[7]}
		byCity[h.CityCode] = append(byCity[h.CityCode], h)
		total++
	}
	for _, list := range byCity {
		sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name) })
	}
}

// ByCity: daftar RS di satu kabupaten/kota (kode Kepmendagri, mis. "32.73"), urut nama.
func ByCity(cityCode string) []Hospital {
	once.Do(load)
	return byCity[cityCode]
}

// Total jumlah RS di direktori.
func Total() int {
	once.Do(load)
	return total
}
