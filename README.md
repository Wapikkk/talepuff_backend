# TalePuff Backend (Gin)

API utama TalePuff. Komunikasi aplikasi dengan layanan Gin. Gin yang memanggil layanan AI (FastAPI), menyimpan data di PostgreSQL, dan memverifikasi login Firebase.

```
Flutter ──HTTP──> Gin :8080 ──HTTP──> FastAPI :8000 ──> Ollama :11434
                    │
                    └──> PostgreSQL :5432        (login: token Firebase diverifikasi oleh Gin)
```

## Menjalankan

```
cp .env.example .env      # PowerShell: Copy-Item .env.example .env
go run .
```

Tabel dibuat otomatis saat server menyala (`AutoMigrate`). Ketika migrasi gagal, server berhenti dengan pesan error. Setiap kali menambah routing baru, server harus di restart: message `404 page not found` berarti routing belum terdaftar di proses yang sedang berjalan.

Panduan lengkap menjalankan seluruh sistem (PostgreSQL, Ollama, FastAPI, Gin, aplikasi): [`docs/RUN_LOCAL.md`](docs/RUN_LOCAL.md).

## Konfigurasi (`.env`)

| Variabel | Keterangan |
|---|---|
| `DB_URL` | Koneksi PostgreSQL |
| `FIREBASE_PROJECT_ID` | ID proyek Firebase untuk memverifikasi token |
| `AI_SERVICE_URL` | Alamat FastAPI, bawaan `http://localhost:8000` |
| `TEST_DB_URL` | Database tes; namanya wajib berakhiran `_test` |

## Tes

```
go test ./...
```

Tes routing memakai database terpisah dari `TEST_DB_URL` dan **menolak berjalan** ketika nama databasenya tidak berakhiran `_test`. Tanpa `TEST_DB_URL`, tes routing dilewati.

## API

Semua routing di bawah `/api` kecuali `/api/health` wajib header `Authorization: Bearer <ID token Firebase>`. Identitas selalu diambil dari token: uid pada path atau body yang berbeda dari token dijawab `403`, dan data milik pengguna lain dijawab `404`.

| Metode | Path | Fungsi | Kode khusus |
|---|---|---|---|
| GET | `/api/health` | Pemeriksaan hidup (publik) | |
| POST | `/api/register` | Mendaftarkan profil anak untuk uid dari token (email tidak diperlukan dan tidak disimpan) | `409` bila profil sudah ada (tidak menimpa) |
| GET | `/api/child/:uid` | Profil anak milik uid | `404` bila belum ada profil |
| PUT | `/api/child/update-name/:id` | Mengganti nama anak | |
| PUT | `/api/child/:id/mascot` | Memilih maskot | |
| POST | `/api/child/upload-photo/:id` | Mengunggah foto profil | `413` terlalu besar, `415` bukan JPEG/PNG/WebP |
| DELETE | `/api/child/:id/photo` | Menghapus foto profil | |
| POST | `/api/story/generate` | Memulai job pembuatan cerita | `202` + `job_id`; `409` bila anak masih punya job aktif |
| GET | `/api/story/jobs/:id` | Status job (tahap nyata) | |
| GET | `/api/stories` | Daftar cerita | query: `child_id` (wajib), `tab` = `recent`/`favorites`/`popular`, `q`, `limit` (1-100), `offset` |
| GET | `/api/stories/:id` | Detail cerita | |
| PUT | `/api/stories/:id/favorite` | Menandai favorit | |
| DELETE | `/api/stories/:id` | Menghapus cerita dan berkas audionya | |
| POST | `/api/audio/generate_audio` | Membuat audio untuk cerita | |
| POST | `/api/feedback` | Mengirim masukan | `201`; `429` bila melebihi batas per jam |

**Email tidak disimpan di backend.** Sumber kebenaran email hanya Firebase; backend memetakan akun lewat `firebase_uid`. Endpoint `PUT /api/user/update-email/:uid` sudah dihapus (kini `404`), dan body `register` dari klien lama yang masih mengirim `email` tetap diterima (field itu diabaikan). Kolom `users.email` di database yang sudah berjalan dibiarkan tidak terpakai; bila ingin dibuang (setelah membuat cadangan): `ALTER TABLE users DROP COLUMN email;` (indeks uniknya ikut terhapus).

Berkas statis foto profil dilayani di `/uploads` (publik; namanya acak 128-bit sehingga tidak bisa ditebak, tetapi belum ber-token).

## Struktur

```
auth/        verifikasi token Firebase + middleware
dto/         bentuk data masuk/keluar
handlers/    logika per endpoint
models/      tabel database (GORM)
router/      daftar rute + tes rute
validation/  aturan input yang sama dengan Flutter dan layanan AI (mis. nama anak)
```
