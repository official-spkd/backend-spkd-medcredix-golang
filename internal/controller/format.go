package controller

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Helper untuk kontrak API frontend Vue: field camelCase, status berlabel ("Dalam Pengerjaan"),
// sedangkan database menyimpan kode ("dalam_pengerjaan").
type J = map[string]interface{}

// label mengubah kode DB menjadi label tampilan: "dalam_pengerjaan" -> "Dalam Pengerjaan".
func label(code string) string {
	if code == "" {
		return ""
	}
	words := strings.Fields(strings.ReplaceAll(code, "_", " "))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// codeOf mengubah label tampilan menjadi kode DB: "Dalam Pengerjaan" -> "dalam_pengerjaan".
func codeOf(lbl string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(lbl)), " ", "_")
}

func labelP(p *string) interface{} {
	if p == nil {
		return nil
	}
	return label(*p)
}

func labelStr(p *string) *string {
	if p == nil {
		return nil
	}
	s := label(*p)
	return &s
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func strOr(p *string, def string) string {
	if p == nil || *p == "" {
		return def
	}
	return *p
}

func sv(p *string) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func fv(p *float64) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func iv(p *int) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func tv(p *time.Time) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func rawStr(r json.RawMessage) interface{} {
	if len(r) == 0 || string(r) == "null" {
		return nil
	}
	return string(r)
}

func yesNo(b *bool, yes, no string) interface{} {
	if b == nil {
		return nil
	}
	if *b {
		return yes
	}
	return no
}

// parseDate menerima "2026-10-15" atau RFC3339.
func parseDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return &t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	return nil
}

func dash(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return t.Format("2006-01-02")
}

var monthsID = []string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

func monthLabel(m string) string {
	if len(m) == 7 {
		if i, err := strconv.Atoi(m[5:]); err == nil && i >= 1 && i <= 12 {
			return monthsID[i-1]
		}
	}
	return m
}

func numStr(v interface{}) string {
	if f, ok := v.(float64); ok {
		return fmtNum(f)
	}
	return "—"
}

func fmtNum(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

func likeAny(term string, vals ...string) bool {
	term = strings.ToLower(term)
	for _, v := range vals {
		if strings.Contains(strings.ToLower(v), term) {
			return true
		}
	}
	return false
}

// rank level/prioritas/keparahan untuk pengurutan: kritis > tinggi > sedang > rendah.
func rank(code string) int {
	switch strings.ToLower(code) {
	case "kritis":
		return 4
	case "tinggi":
		return 3
	case "sedang":
		return 2
	case "rendah":
		return 1
	}
	return 0
}

func timeAfter(a, b *time.Time) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.After(*b)
}

func latest(a, b *time.Time) *time.Time {
	if timeAfter(a, b) {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// sortJ mengurutkan slice J secara stabil dengan pembanding bebas.
func sortJ(rows []J, less func(a, b J) bool) {
	sort.SliceStable(rows, func(i, j int) bool { return less(rows[i], rows[j]) })
}
