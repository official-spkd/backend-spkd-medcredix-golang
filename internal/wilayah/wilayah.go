// Package wilayah: data kode wilayah administrasi Indonesia (provinsi, kabupaten/kota, kecamatan, kelurahan/desa).
//
// Sumber: github.com/cahyadsn/wilayah rilis v2026.7 (MIT), sesuai Kepmendagri No 300.2.2-2138 Tahun 2025:
// 38 provinsi, 514 kabupaten/kota, 7.285 kecamatan, 83.762 kelurahan/desa.
// Data disimpan di wilayah.tsv (kode<TAB>nama) dan ikut ter-embed di binary, jadi tidak bergantung pada API luar.
// Untuk memperbarui: ganti wilayah.tsv dengan data rilis terbaru lalu build ulang.
package wilayah

import (
	_ "embed"
	"sort"
	"strings"
	"sync"
)

//go:embed wilayah.tsv
var raw string

// Region satu wilayah. Level: 1 provinsi, 2 kabupaten/kota, 3 kecamatan, 4 kelurahan/desa.
type Region struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Level int    `json:"level"`
}

var (
	once     sync.Once
	byCode   map[string]Region
	children map[string][]Region // kunci "" = daftar provinsi
)

func load() {
	byCode = make(map[string]Region, 92000)
	children = make(map[string][]Region, 8000)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		code, name, found := strings.Cut(line, "\t")
		if !found || code == "" {
			continue
		}
		r := Region{Code: code, Name: name, Level: strings.Count(code, ".") + 1}
		byCode[code] = r
		parent := ""
		if i := strings.LastIndex(code, "."); i >= 0 {
			parent = code[:i]
		}
		children[parent] = append(children[parent], r)
	}
	for _, list := range children {
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	}
}

// Children: daftar wilayah di bawah kode induk, urut nama. Induk kosong = 38 provinsi.
// Mengembalikan nil bila kode induk tidak dikenal.
func Children(parent string) []Region {
	once.Do(load)
	parent = strings.TrimSpace(parent)
	if parent != "" {
		if _, ok := byCode[parent]; !ok {
			return nil
		}
	}
	list := children[parent]
	if list == nil {
		return []Region{}
	}
	return list
}

// find mencari wilayah bernama name (tanpa membedakan huruf besar/kecil) di bawah induk.
func find(parent, name string) (Region, bool) {
	for _, r := range children[parent] {
		if strings.EqualFold(r.Name, name) {
			return r, true
		}
	}
	return Region{}, false
}

// Resolve memastikan nama provinsi > kabupaten/kota > kecamatan > kelurahan/desa saling cocok,
// lalu mengembalikan nama bakunya (huruf besar/kecil sesuai data Kepmendagri).
// Isian kosong di ujung boleh (mis. hanya provinsi + kota), tetapi tingkat bawah wajib punya tingkat atas.
// Mengembalikan pesan error (kosong bila valid).
func Resolve(province, city, district, village string) ([4]string, string) {
	var out [4]string
	once.Do(load)
	names := []string{strings.TrimSpace(province), strings.TrimSpace(city), strings.TrimSpace(district), strings.TrimSpace(village)}
	labels := []string{"Provinsi", "Kabupaten/kota", "Kecamatan", "Kelurahan/desa"}
	parent := ""
	for i, n := range names {
		if n == "" {
			for j := i + 1; j < len(names); j++ {
				if names[j] != "" {
					return out, labels[i] + " wajib dipilih sebelum " + strings.ToLower(labels[j]) + "."
				}
			}
			return out, ""
		}
		r, ok := find(parent, n)
		if !ok {
			if i == 0 {
				return out, "Provinsi \"" + n + "\" tidak dikenal. Pilih dari daftar."
			}
			return out, labels[i] + " \"" + n + "\" tidak ada di " + byCode[parent].Name + ". Pilih dari daftar."
		}
		out[i] = r.Name
		parent = r.Code
	}
	return out, ""
}

// Code mengembalikan kode wilayah dari rangkaian nama (mis. provinsi + kab/kota), atau "" bila tidak cocok.
func Code(names ...string) string {
	once.Do(load)
	parent := ""
	for _, n := range names {
		r, ok := find(parent, strings.TrimSpace(n))
		if !ok {
			return ""
		}
		parent = r.Code
	}
	return parent
}

// Name mengembalikan nama wilayah dari kodenya.
func Name(code string) string {
	once.Do(load)
	return byCode[code].Name
}
