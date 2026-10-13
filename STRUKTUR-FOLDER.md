# Struktur Folder Backend MedCredix

Panduan singkat supaya orang baru bisa langsung tahu kode mana ada di mana.

## Alur satu request

```
Browser / frontend
   │
   ▼
internal/router        URL apa diarahkan ke controller mana
   │
   ▼
internal/middleware    CORS, header keamanan, batas body, cek token login, cek izin peran
   │
   ▼
internal/controller    baca input, validasi, panggil database, kirim JSON
   │
   ├── internal/service     cache skor, job otomatis
   ├── internal/scoring     rumus skor
   ├── internal/audit       catat audit trail
   └── internal/model  ─►  internal/database (PostgreSQL)
```

## Isi tiap folder

| Folder | Isinya | Kapan dibuka |
|---|---|---|
| `cmd/server` | `main.go`: baca env, buka database, isi data demo, jalankan server | Mengubah cara server start |
| `internal/router` | `router.go`: daftar semua URL dan middleware-nya | Menambah atau mengganti endpoint |
| `internal/middleware` | Kode yang jalan sebelum controller (login, izin, CORS, dll.) | Mengubah aturan keamanan untuk semua endpoint |
| `internal/controller` | Handler HTTP, satu file per fitur (`*_controller.go`) | Mengubah isi respons atau logika satu fitur |
| `internal/service` | Logika yang tidak terikat satu request: cache skor dan job status kredensial | Mengubah cache atau job terjadwal |
| `internal/model` | Struct GORM, satu struct per tabel | Menambah kolom atau tabel |
| `internal/database` | Koneksi PostgreSQL dan `schema.sql` (33 tabel) | Mengubah skema database |
| `internal/config` | Baca environment: `APP_ENV`, `TRUSTED_PROXIES`, angka/boolean | Menambah variabel env baru |
| `internal/auth` | JWT, daftar peran dan izin (RBAC), isolasi per RS | Mengubah peran atau izin |
| `internal/audit` | Menulis audit trail | Mengubah format log audit |
| `internal/scoring` | Mesin skor (Health Score, BDCS, Trust, dll.) + unit test | Mengubah rumus skor |
| `internal/seed` | Data demo, hanya jalan kalau `SEED_DEMO=true` | Mengubah data contoh |
| `internal/storage` | Simpan berkas unggahan terenkripsi (AES-256-GCM) | Mengubah cara simpan berkas |
| `internal/integration` | Klien FHIR untuk SIMRS / SATUSEHAT | Mengubah koneksi ke sistem luar |

## Isi folder controller

File helper yang dipakai semua controller:

| File | Isinya |
|---|---|
| `response.go` | `ok()` dan `fail()`: format JSON `{ok, data}` / `{ok, error}` |
| `request.go` | Ambil user dan RS aktif, cek izin, baca body JSON |
| `format.go` | Ubah kode DB ke label tampilan, format angka dan tanggal |
| `mapper.go` | Ubah baris database ke bentuk JSON yang dipakai frontend |
| `intel_mapper.go` | Ubah hasil scoring engine ke bentuk JSON frontend |
| `upload.go` | Simpan berkas unggahan dan kirim berkas ke browser |
| `login_limiter.go` | Kunci sementara setelah salah login berkali-kali |

Daftar endpoint dan file controller-nya:

| Method | URL | Fungsi | File |
|---|---|---|---|
| GET | `/` | `Home` | `health_controller.go` |
| GET | `/api/health` | `Health` | `health_controller.go` |
| POST | `/api/auth` | `Login` | `auth_controller.go` |
| POST | `/api/auth/logout` | `Logout` | `auth_controller.go` |
| GET | `/api/bootstrap` | `GetBootstrap` | `dashboard_controller.go` |
| GET | `/api/dashboard` | `GetDashboard` | `dashboard_controller.go` |
| GET | `/api/scores` | `GetScores` | `score_controller.go` |
| GET | `/api/admin` | `GetAdmin` | `dashboard_controller.go` |
| GET | `/api/risks` | `GetRisks` | `risk_controller.go` |
| GET | `/api/warnings` | `GetWarnings` | `warning_controller.go` |
| GET | `/api/actions` | `GetActions` | `action_controller.go` |
| GET | `/api/practitioners` | `GetPractitioners` | `practitioner_controller.go` |
| GET | `/api/credentials` | `GetCredentials` | `credential_controller.go` |
| GET | `/api/requirements` | `GetRequirements` | `requirement_controller.go` |
| GET | `/api/requirements/file` | `GetRequirementFile` | `requirement_controller.go` |
| GET | `/api/documents` | `GetDocuments` | `document_controller.go` |
| GET | `/api/documents/file` | `GetDocumentFile` | `document_controller.go` |
| GET | `/api/claims` | `GetClaims` | `claim_controller.go` |
| GET | `/api/clinical` | `GetClinical` | `clinical_controller.go` |
| GET | `/api/compliance` | `GetCompliance` | `compliance_controller.go` |
| GET | `/api/evidence` | `GetEvidence` | `evidence_controller.go` |
| GET | `/api/audit` | `GetAudit` | `audit_controller.go` |
| GET | `/api/search` | `Search` | `search_controller.go` |
| GET | `/api/reports` | `GetReports` | `report_controller.go` |
| GET | `/api/settings` | `GetSettings` | `setting_controller.go` |
| GET | `/api/data` | `GetData` | `data_controller.go` |
| PATCH | `/api/risks` | `PatchRisks` | `risk_controller.go` |
| PATCH | `/api/warnings` | `PatchWarnings` | `warning_controller.go` |
| POST | `/api/actions` | `PostActions` | `action_controller.go` |
| PATCH | `/api/actions` | `PatchActions` | `action_controller.go` |
| PATCH | `/api/credentials` | `PatchCredentials` | `credential_controller.go` |
| PATCH | `/api/requirements` | `PatchRequirements` | `requirement_controller.go` |
| POST | `/api/requirements/upload` | `UploadRequirementFile` | `requirement_controller.go` |
| POST | `/api/documents/upload` | `UploadDocument` | `document_controller.go` |
| PATCH | `/api/claims` | `PatchClaims` | `claim_controller.go` |
| POST | `/api/reports` | `PostReports` | `report_controller.go` |
| PATCH | `/api/settings` | `PatchSettings` | `setting_controller.go` |
| POST | `/api/data` | `PostData` | `data_controller.go` |
| POST | `/api/scores/snapshot` | `PostScoreSnapshot` | `score_controller.go` |

Semua endpoint di bawah `/api` kecuali `/api/health` dan `POST /api/auth` wajib login.

## Cara menambah endpoint baru

Contoh: endpoint `GET /api/contoh`.

1. Buat `internal/controller/contoh_controller.go`:

   ```go
   package controller

   // GetContoh menampilkan ... (GET /api/contoh).
   func GetContoh(db *gorm.DB) gin.HandlerFunc {
   	return func(c *gin.Context) {
   		_, hid, good := feCtx(c, db) // user login + RS aktif
   		if !good {
   			return
   		}
   		// ... query pakai db.Where("hospital_id = ?", hid)
   		ok(c, J{"items": hasil})
   	}
   }
   ```

2. Daftarkan di `internal/router/router.go`:

   ```go
   s.GET("/contoh", view, controller.GetContoh(db))
   ```

3. Jalankan `go build ./...` dan `go test ./...`.

## Aturan penamaan

- Nama file pakai huruf kecil dan garis bawah: `risk_controller.go`, `security_headers.go`.
- Fungsi controller diawali method HTTP: `GetRisks`, `PatchRisks`, `PostActions`. Pengecualian: `Login`, `Logout`, `Search`, `Home`, `Health`, dan unggahan (`UploadDocument`, `UploadRequirementFile`).
- Fungsi yang hanya dipakai di dalam folder yang sama diawali huruf kecil (`feCtx`, `saveUpload`).
- Setiap query data RS selalu memakai `hospital_id = ?` supaya data antar RS tidak bocor.