# MedCredix API

Backend **MedCredix** (Medical Credentialing Intelligence) by SPKD. Fungsinya:

- menghitung skor kesiapan rumah sakit, yaitu Hospital Health Score, Skor Kredensial BPJS (BDCS), Data Trust, CCS dan CPCS;
- mengelola kredensial tenaga medis, prasyarat, risiko, early warning, aksi perbaikan, klaim BPJS dan laporan;
- melayani frontend Vue lewat REST API.

Satu backend dipakai banyak rumah sakit (multi-tenant). Data tiap RS dipisah, dan setiap perubahan tercatat di audit trail.

---

## Daftar Isi

1. [Teknologi](#teknologi)
2. [Struktur Folder](#struktur-folder)
3. [Prasyarat (Windows & macOS)](#prasyarat)
4. [Mulai Cepat](#mulai-cepat)
5. [Instalasi](#instalasi)
6. [Konfigurasi `.env`](#konfigurasi-env)
7. [Menjalankan Server](#menjalankan-server)
8. [Menghubungkan dengan Frontend](#menghubungkan-dengan-frontend)
9. [Akun Demo](#akun-demo)
10. [Ringkasan API](#ringkasan-api)
11. [Peran & Izin](#peran--izin)
12. [Testing](#testing)
13. [Deploy ke Produksi](#deploy-ke-produksi)
14. [Backup Database](#backup-database)
15. [Troubleshooting](#troubleshooting)

---

## Teknologi

| Komponen | Versi | Keterangan |
|---|---|---|
| Go | 1.27+ | Bahasa backend |
| Gin | 1.12 | HTTP router |
| GORM | 1.31 | ORM ke database |
| PostgreSQL | 16+ (diuji di 16 dan 18) | Database |
| golang-jwt | v5 | Token login (HS256) |
| bcrypt | `golang.org/x/crypto` | Hash password |
| godotenv | 1.5 | Membaca file `.env` |

GORM, Gin dan library lain **tidak perlu di-install manual**. Semuanya terdaftar di `go.mod` dan otomatis diunduh oleh Go (lihat [Langkah 4](#4-unduh-dependency)).

---

## Struktur Folder

```
backend-spkd-medcredix/
├── cmd/server/main.go          # titik masuk aplikasi
├── internal/
│   ├── api/                    # handler HTTP & router
│   │   ├── router.go           # daftar endpoint + middleware keamanan
│   │   ├── auth_handler.go     # login + pembatas percobaan login
│   │   ├── fe_*.go             # endpoint yang dipakai frontend Vue
│   │   ├── jobs.go             # job otomatis (status kredensial tiap jam)
│   │   └── ...
│   ├── auth/                   # JWT, peran & izin (RBAC), isolasi RS
│   ├── audit/                  # pencatatan audit trail
│   ├── db/                     # koneksi database + schema.sql (33 tabel)
│   ├── model/                  # struct GORM per tabel
│   ├── scoring/                # mesin perhitungan skor + unit test
│   ├── seed/                   # data demo (hanya untuk lokal)
│   ├── integration/            # klien FHIR (SIMRS / SATUSEHAT)
│   └── appenv/                 # helper environment
├── go.mod / go.sum             # daftar dependency
└── .gitignore
```

---

## Prasyarat

Butuh empat aplikasi: **Go**, **PostgreSQL**, **Git**, dan **DBeaver** (opsional). Pilih panduan sesuai sistem operasi kamu.

<details open>
<summary><b>🪟 Windows</b></summary>

<br>

**1. Go**

1. Unduh installer `.msi` di <https://go.dev/dl/>.
2. Jalankan installer dengan pilihan default.
3. Buka **PowerShell baru**, lalu cek:
   ```powershell
   go version
   ```
   Hasilnya harus `go1.27` atau lebih baru.

**2. PostgreSQL**

1. Unduh installer di <https://www.postgresql.org/download/windows/> (installer dari EDB).
2. Saat instalasi:
   - catat **password user `postgres`** yang kamu buat;
   - port biarkan **5432**;
   - komponen **Command Line Tools** tetap dicentang.
3. Tambahkan folder `bin` PostgreSQL ke **PATH** supaya perintah `psql` dan `pg_dump` dikenali:
   ```
   C:\Program Files\PostgreSQL\18\bin
   ```
   Caranya: *Start → Edit the system environment variables → Environment Variables → Path → Edit → New*. Sesuaikan angka `18` dengan versi PostgreSQL kamu.
4. Buka PowerShell baru, lalu cek:
   ```powershell
   psql --version
   ```

**3. Git**

Unduh di <https://git-scm.com/download/win>, lalu install dengan pilihan default.

**4. DBeaver** (opsional)

Unduh di <https://dbeaver.io/download/>, lalu install.

> **Alternatif cepat:** Go, Git dan DBeaver juga bisa di-install lewat `winget`:
> ```powershell
> winget install GoLang.Go Git.Git dbeaver.dbeaver
> ```

</details>

<details open>
<summary><b>🍎 macOS</b></summary>

<br>

Panduan ini memakai **Homebrew**. Bila belum punya, install dulu:

```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

**1. Go**

```bash
brew install go
go version
```

Hasilnya harus `go1.27` atau lebih baru.

**2. PostgreSQL**

```bash
brew install postgresql@18
brew services start postgresql@18
```

Tambahkan `psql` dan `pg_dump` ke PATH:

```bash
echo 'export PATH="$(brew --prefix postgresql@18)/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
psql --version
```

> Di Homebrew, user admin PostgreSQL adalah **username Mac kamu** (bukan `postgres`) dan tidak memakai password. Ini berpengaruh di [langkah pembuatan database](#2-buat-user--database-postgresql).
>
> Tidak mau pakai terminal? Gunakan [Postgres.app](https://postgresapp.com/) sebagai alternatif.

**3. Git**

Git biasanya sudah tersedia. Cek dengan `git --version`. Bila belum ada:

```bash
brew install git
```

**4. DBeaver** (opsional)

```bash
brew install --cask dbeaver-community
```

</details>

---

## Mulai Cepat

Sudah punya semua prasyarat? Lima langkah ini cukup:

```bash
git clone https://github.com/official-spkd/backend-spkd-medcredix-golang.git
cd backend-spkd-medcredix-golang
# 1. buat database (lihat langkah 2 di bawah)
# 2. buat file .env (lihat bagian Konfigurasi .env)
go mod download
go run ./cmd/server
```

Penjelasan lengkap tiap langkah ada di bawah.

---

## Instalasi

### 1. Clone repository

```bash
git clone https://github.com/official-spkd/backend-spkd-medcredix-golang.git
cd backend-spkd-medcredix-golang
```

### 2. Buat user & database PostgreSQL

Pilih salah satu cara di bawah.

**Cara A: lewat terminal**

| Windows (PowerShell) | macOS (Homebrew) |
|---|---|
| `psql -U postgres` lalu masukkan password `postgres` | `psql postgres` (tanpa password) |

Setelah masuk ke `psql`, jalankan:

```sql
CREATE USER medcredix WITH PASSWORD 'ganti-dengan-password-kamu';
CREATE DATABASE medcredix OWNER medcredix;
\q
```

**Cara B: lewat DBeaver**

1. *Database → New Database Connection → PostgreSQL*.
2. Isi Host `localhost` dan Port `5432`.
   - Windows: User `postgres` dengan password yang kamu buat saat instalasi.
   - macOS (Homebrew): User = username Mac kamu, password dikosongkan.
3. Klik kanan koneksi, pilih *SQL Editor → New SQL script*, lalu jalankan dua perintah `CREATE` di atas.

> Tabel **tidak perlu** dibuat manual. Saat server pertama kali jalan, ke-33 tabel otomatis dibuat dari `internal/db/schema.sql`.

### 3. Buat file `.env`

Buat file bernama `.env` di root folder project, lalu isi seperti contoh di bagian [Konfigurasi `.env`](#konfigurasi-env).

> File `.env` berisi password dan secret. File ini sudah masuk `.gitignore`, jadi **jangan pernah di-commit**.

### 4. Unduh dependency

```bash
go mod download
```

Perintah ini mengunduh Gin, GORM, driver PostgreSQL, JWT dan library lain sesuai `go.mod`.

---

## Konfigurasi `.env`

Contoh untuk **lokal / development**:

```env
APP_ENV=development
SEED_DEMO=true
MEDCREDIX_PORT=8080

DB_HOST=127.0.0.1
DB_PORT=5432
DB_USER=medcredix
DB_PASSWORD=ganti-dengan-password-kamu
DB_NAME=medcredix
DB_SSLMODE=disable

JWT_SECRET=isi-dengan-hasil-generate-di-bawah
CORS_ORIGIN=http://localhost:3000
TRUSTED_PROXIES=
```

| Variabel | Wajib | Keterangan |
|---|---|---|
| `APP_ENV` | Ya | `development` atau `production` |
| `SEED_DEMO` | Tidak | `true` = isi data & akun demo. **Dilarang** `true` di produksi. |
| `MEDCREDIX_PORT` | Tidak | Port server, default `8080` |
| `DB_HOST` / `DB_PORT` | Ya | Alamat PostgreSQL. Di komputer yang sama pakai `127.0.0.1` (lebih cepat dari `localhost` di Windows) |
| `DB_USER` / `DB_PASSWORD` / `DB_NAME` | Ya | Akun & nama database dari langkah 2 |
| `DB_SSLMODE` | Tidak | `disable` untuk lokal, `require` untuk produksi |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` | Tidak | Ukuran pool koneksi database, default `25` / `10` |
| `JWT_SECRET` | Ya di produksi | Kunci tanda tangan token login, minimal 32 karakter |
| `CORS_ORIGIN` | Tidak | Alamat frontend. Pisahkan dengan koma bila lebih dari satu. |
| `TRUSTED_PROXIES` | Tidak | IP reverse proxy (misalnya Nginx). Kosongkan bila tidak ada. |
| `SIMRS_FHIR_BASE_URL` | Tidak | URL server FHIR SIMRS. Kosong = Test Koneksi berjalan dalam mode simulasi. |
| `SIMRS_FHIR_TOKEN` | Tidak | Token server FHIR |

### Membuat `JWT_SECRET`

Buat secret acak dan **berbeda** untuk setiap lingkungan (lokal dan produksi).

**Windows (PowerShell):**

```powershell
$b = New-Object byte[] 48
[Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b)
[Convert]::ToBase64String($b)
```

**macOS / Linux / Git Bash:**

```bash
openssl rand -base64 48
```

Salin hasilnya ke `JWT_SECRET=` di `.env`.

---

## Menjalankan Server

```bash
go run ./cmd/server
```

Kalau berhasil, log menampilkan:

```
database terhubung
seed dasar selesai: 2 rumah sakit, 12 akun demo
MedCredix API jalan di :8080 (produksi=false)
```

Cek dari browser atau terminal:

```bash
curl http://localhost:8080/api/health
# {"data":{"status":"up"},"ok":true}
```

Saat server berjalan pertama kali:

- semua tabel dibuat otomatis;
- bila `SEED_DEMO=true`, data demo (2 RS dan 12 akun) ikut diisi;
- pada start berikutnya, data yang sudah ada **tidak ditimpa**.

Setiap kali server jalan, dan setelah itu tiap 1 jam, status kredensial diperbarui otomatis:

- `aktif` → `segera_berakhir` bila masa berlaku tinggal ≤ 90 hari;
- `aktif` / `segera_berakhir` → `kedaluwarsa` bila tanggalnya sudah lewat.

Perubahan ini tercatat di `audit_logs` dengan aktor **Sistem MedCredix**.

---

## Menghubungkan dengan Frontend

Frontend Vue berjalan di `http://localhost:3000`. Server dev Vite meneruskan semua request `/api` ke backend di `http://localhost:8080`.

1. Jalankan backend dengan `go run ./cmd/server`.
2. Di folder frontend:
   ```bash
   npm install
   npm run dev
   ```
3. Buka <http://localhost:3000>, lalu login dengan salah satu [akun demo](#akun-demo).

Pastikan di `.env` frontend tertulis `VITE_DEMO_MODE=false`.

---

## Akun Demo

Akun ini hanya ada bila `SEED_DEMO=true`. Semua akun memakai password `medcredix2026`.

| Email | Peran | Rumah Sakit |
|---|---|---|
| superadmin@spkd.co.id | Super Admin | semua RS |
| auditorbpjs@spkd.co.id | Auditor BPJS | semua RS (hanya lihat) |
| auditor@spkd.co.id | Auditor | RS Nusantara Sehat (hanya lihat) |
| admin@nusantarasehat.co.id | Admin RS | RS Nusantara Sehat |
| manajemen@nusantarasehat.co.id | Manajemen RS | RS Nusantara Sehat |
| kredensial@nusantarasehat.co.id | Petugas Kredensial | RS Nusantara Sehat |
| komite@nusantarasehat.co.id | Komite Medis | RS Nusantara Sehat |
| klaim@nusantarasehat.co.id | Tim Klaim | RS Nusantara Sehat |
| it@nusantarasehat.co.id | IT / SIMRS | RS Nusantara Sehat |
| sdm@nusantarasehat.co.id | HR / SDM | RS Nusantara Sehat |
| viewer@nusantarasehat.co.id | Viewer | RS Nusantara Sehat |
| admin@medikasejahtera.co.id | Admin RS | RS Medika Sejahtera |

### Mengganti password akun

Jalankan di SQL Editor DBeaver:

```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;

UPDATE users
SET password_hash = crypt('PasswordBaru123!', gen_salt('bf', 10))
WHERE email = 'auditorbpjs@spkd.co.id';
```

> Kode program ini publik, jadi password demo juga diketahui umum. **Wajib** ganti password semua akun yang dipakai di luar komputer lokal.

---

## Ringkasan API

Semua endpoint ada di bawah `/api`. Format respons:

```json
{ "ok": true,  "data": { ... } }
{ "ok": false, "error": "pesan kesalahan" }
```

### Autentikasi

```http
POST /api/auth
Content-Type: application/json

{ "email": "admin@nusantarasehat.co.id", "password": "medcredix2026" }
```

Respons login berisi `token`. Kirim token itu di setiap request berikutnya:

```http
Authorization: Bearer <token>
```

Token berlaku 12 jam. Untuk logout, panggil `POST /api/auth/logout` dengan token yang sama. Token itu langsung dicabut dan tidak bisa dipakai lagi.

> Daftar token yang sudah logout disimpan di memori server. Bila server di-restart, daftar ini kosong lagi.

Super Admin dan Auditor BPJS bisa memilih RS lewat parameter `?hospitalId=...`. Peran lain otomatis terkunci ke RS miliknya.

### Daftar endpoint

| Method | Endpoint | Fungsi |
|---|---|---|
| GET | `/health` | Cek server hidup |
| POST | `/auth` | Login |
| POST | `/auth/logout` | Logout, token dicabut di server |
| GET | `/bootstrap` | Data awal: user, RS, badge, notifikasi |
| GET | `/dashboard` | Ringkasan skor, risiko, warning, aksi |
| GET | `/scores?domain=` | Detail skor per domain |
| GET | `/admin` | Rekap semua RS (Super Admin / Auditor BPJS) |
| GET, PATCH | `/risks` | Risk Radar |
| GET, PATCH | `/warnings` | Early Warnings |
| GET, POST, PATCH | `/actions` | Action Center: buat, tugaskan, ubah status, komentar |
| GET | `/practitioners` | Data tenaga medis |
| GET, PATCH | `/credentials` | Kredensial: ajukan → verifikasi → tinjau → setujui → aktif |
| GET, PATCH | `/requirements` | Prasyarat wajib P1–P6 |
| GET, PATCH | `/claims` | Klaim BPJS & anomali |
| GET | `/clinical?type=` | Pathway, utilization, patient safety |
| GET | `/compliance` | Kepatuhan regulasi |
| GET | `/evidence` | Bukti data |
| GET | `/audit` | Audit trail |
| GET | `/search?q=` | Pencarian global |
| GET, POST | `/reports` | Buat laporan & unduh CSV (`?export=<id>`) |
| GET, PATCH | `/settings` | Profil RS, pengguna, aturan peringatan |
| GET, POST | `/data` | Kualitas data, data trust, sumber data, Test Koneksi FHIR |
| POST | `/scores/snapshot` | Simpan skor periode ini ke riwayat tren |

Setiap aksi `POST` / `PATCH` dicatat di tabel `audit_logs`, lengkap dengan nilai sebelum dan sesudah.

---

## Peran & Izin

| Peran | Izin |
|---|---|
| Super Admin, Admin RS | Semua izin |
| Manajemen RS | VIEW, EXPORT, PRINT, ASSIGN |
| Petugas Kredensial | VIEW, CREATE, EDIT, VERIFY, EXPORT, PRINT, ASSIGN |
| Komite Medis | VIEW, VERIFY, APPROVE, SIGN, EXPORT, RESOLVE, REOPEN |
| HR / SDM | VIEW, CREATE, EDIT, EXPORT |
| Auditor | VIEW, EXPORT, PRINT |
| Tim Klaim | VIEW, CREATE, EDIT, EXPORT, ASSIGN, RESOLVE |
| IT / SIMRS | VIEW, CREATE, EDIT, EXPORT, CONFIGURE |
| Viewer | VIEW |

Aturan tambahan:

- Pengguna yang memverifikasi sebuah kredensial **tidak boleh** menyetujui kredensial yang sama (segregation of duties).
- Status kredensial harus berurutan. Contohnya, status `diajukan` tidak bisa langsung menjadi `aktif`.
- Status aksi juga harus berurutan:

  | Ke status | Boleh dari | Izin |
  |---|---|---|
  | Ditugaskan | Baru, Dibuka Kembali | ASSIGN |
  | Dalam Pengerjaan | Baru, Ditugaskan, Menunggu Verifikasi, Dibuka Kembali | EDIT |
  | Menunggu Verifikasi | Dalam Pengerjaan | EDIT |
  | Selesai | Menunggu Verifikasi | RESOLVE |
  | Ditutup | Selesai | RESOLVE |
  | Ditolak | Baru, Ditugaskan, Dalam Pengerjaan, Menunggu Verifikasi | RESOLVE |
  | Dibuka Kembali | Selesai, Ditutup, Ditolak | REOPEN |

- Status risiko hanya boleh salah satu dari `baru`, `terbuka`, `ditinjau`, `dalam_pengerjaan`, `terkelola`, `teratasi`, `selesai`, `ditutup`. Menutup risiko (`teratasi`, `selesai`, `ditutup`) butuh izin RESOLVE.

---

## Testing

```bash
go vet ./...
go test ./...
```

Unit test ada di `internal/scoring` dan memeriksa rumus perhitungan skor.

### Postman

Collection `MedCredix_API_Test.postman_collection.json` berisi tes login, isolasi RS, kredensial, skor, logout, alur status aksi dan risiko, serta kecepatan respons.

1. Postman → **Import** → pilih file collection.
2. Pastikan variabel `base_url` bernilai `http://localhost:8080`.
3. Klik kanan sebuah folder → **Run folder**.

### Performa

- Hasil hitung skor disimpan 30 detik per rumah sakit, dan langsung dibuang setiap ada perubahan data lewat API. Perubahan langsung lewat DBeaver baru terlihat maksimal 30 detik kemudian.
- Koneksi database dipakai ulang lewat pool (lihat `DB_MAX_OPEN_CONNS`).

---

## Deploy ke Produksi

1. Build binary:
   ```bash
   go build -o medcredix-api ./cmd/server
   ```
   Di Windows, beri nama `medcredix-api.exe`.
2. Buat `.env` di server dengan pengaturan berikut:
   ```env
   APP_ENV=production
   SEED_DEMO=false
   DB_SSLMODE=require
   DB_PASSWORD=<minimal 16 karakter>
   JWT_SECRET=<secret baru, minimal 32 karakter, beda dari lokal>
   CORS_ORIGIN=https://alamat-frontend-kamu
   TRUSTED_PROXIES=<IP reverse proxy, bila ada>
   ```
3. Jalankan binary di belakang reverse proxy (Nginx atau Caddy) dengan HTTPS.

Saat `APP_ENV=production`, server **menolak start** bila:

- `JWT_SECRET` kosong atau kurang dari 32 karakter;
- `DB_PASSWORD` kurang dari 16 karakter;
- `SEED_DEMO=true`;
- `DB_SSLMODE=disable` ke database yang bukan di mesin yang sama.

---

## Backup Database

Backup (format custom, bisa di-restore dengan `pg_restore`):

```bash
pg_dump -U medcredix -h localhost -d medcredix -F c -f backup-db-medcredix/medcredix_YYYYMMDD.dump
```

Restore:

```bash
pg_restore -U medcredix -h localhost -d medcredix --clean backup-db-medcredix/medcredix_YYYYMMDD.dump
```

Folder `backup-db-medcredix/` dan file `*.dump` sudah masuk `.gitignore` karena berisi data pasien. **Jangan pernah** di-upload ke GitHub.

---

## Troubleshooting

| Masalah | Penyebab | Solusi |
|---|---|---|
| `bind: Only one usage of each socket address` (Windows) / `address already in use` (macOS) | Port 8080 masih dipakai server lain | Hentikan server lama dengan Ctrl+C. Bila terminalnya tidak ketemu: Windows `Get-Process -Id (Get-NetTCPConnection -LocalPort 8080).OwningProcess \| Stop-Process`, macOS `lsof -ti :8080 \| xargs kill` |
| `gagal konek database` | PostgreSQL belum jalan atau isi `.env` salah | Pastikan PostgreSQL berjalan (Windows: *Services → postgresql*, macOS: `brew services list`), lalu cek `DB_HOST`, `DB_USER`, `DB_PASSWORD` dan `DB_NAME` |
| `password authentication failed` | Password DB salah | Samakan `DB_PASSWORD` dengan password yang dibuat di langkah 2 |
| `tabel lama terdeteksi` | Database berisi skema versi lama | Jalankan `DROP TABLE IF EXISTS users, hospitals CASCADE;` di DBeaver, lalu jalankan ulang server |
| `pg_dump` / `psql` tidak dikenali | Folder `bin` PostgreSQL belum ada di PATH | Ikuti langkah PATH di bagian [Prasyarat](#prasyarat) sesuai OS, lalu buka terminal baru |
| `JWT_SECRET terlalu pendek` | Secret kurang dari 32 karakter | Buat secret baru (lihat [Membuat `JWT_SECRET`](#membuat-jwt_secret)) |
| Frontend: `Tidak dapat terhubung ke server` | Backend belum jalan atau port berbeda | Jalankan backend di port 8080, lalu restart `npm run dev` |
| Log `SLOW SQL >= 200ms` | Membuka koneksi baru ke PostgreSQL lambat, biasanya karena `DB_HOST=localhost` di Windows | Ganti jadi `DB_HOST=127.0.0.1`, restart server. Bila masih lambat, kecualikan folder data PostgreSQL dari scan antivirus. Muncul sekali di request pertama setelah start itu wajar. |
| Respons `503` "Database sedang tidak dapat diakses" | PostgreSQL mati atau koneksi putus | Nyalakan PostgreSQL. Pengguna tidak ter-logout; cukup ulangi request. |
| Respons `409` saat ubah status aksi | Status dilompati, misalnya Baru langsung Ditutup | Ikuti urutan di tabel [Peran & Izin](#peran--izin) |
| `undefined: ...` saat build | Ada file di `internal/api` yang hilang atau isinya tertukar | Pastikan semua file `fe_*.go` lengkap dan berada di folder `internal/api` |

---

© SPKD. MedCredix dirancang untuk mendukung kepatuhan. Setiap keputusan akhir tetap memerlukan verifikasi oleh rumah sakit dan regulator.
