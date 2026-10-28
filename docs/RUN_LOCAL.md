# Menjalankan TalePuff secara lokal

Semua layanan berjalan di laptop; HP (APK) terhubung lewat jaringan yang sama.

```
HP (Flutter) ──HTTP──> Gin :8080 ──HTTP──> FastAPI :8000 ──HTTP──> Ollama :11434
                         │                    │
                         │                    └── edge-tts (internet) + ffmpeg -> berkas .wav di /audio
                         └──> PostgreSQL :5432

Firebase Auth (cloud): aplikasi login ke Firebase; Gin hanya memverifikasi tokennya.
```

| Layanan | Folder | Port |
|---|---|---|
| PostgreSQL | (layanan Windows) | 5432 |
| Ollama | (layanan Windows) | 11434 |
| FastAPI (AI) | `ai_services` | 8000 |
| Gin (API utama) | `talepuff_backend` | 8080 |
| Aplikasi | `talepuff_app` | - |

## 1. Prasyarat (sekali saja)

| Perangkat lunak | Catatan |
|---|---|
| Go 1.25+ | backend |
| Python 3.13 | layanan AI |
| PostgreSQL | buat dua database (lihat langkah 2) |
| [Ollama](https://ollama.com) | model `qwen3:4b` (±2,5 GB, muat di GPU 6 GB) |
| ffmpeg | wajib ada di `PATH`; cek dengan `ffmpeg -version`. Dipakai mengubah audio TTS menjadi `.wav` |
| Flutter + Android SDK | untuk menjalankan atau membangun APK |
| Akses internet | edge-tts memakai layanan suara cloud Microsoft; Firebase Auth juga cloud |

## 2. Konfigurasi (sekali saja)

**Berkas `.env`.** Di tiap proyek, salin contohnya lalu isi (PowerShell):

```powershell
Copy-Item talepuff_backend\.env.example talepuff_backend\.env
Copy-Item talepuff_app\.env.example     talepuff_app\.env
Copy-Item ai_services\.env.example      ai_services\.env     # opsional: .env kosong pun sah
```

Penjelasan setiap variabel ada di berkas `.env.example` masing-masing. File `.env` tidak masuk git. Khusus `talepuff_app\.env`: isinya ikut dikemas ke APK, jadi jangan berisi rahasia.

**Database PostgreSQL** (contoh; sesuaikan nama dan pengguna):

```sql
CREATE DATABASE talepuff;
CREATE DATABASE talepuff_test;   -- khusus tes backend, namanya WAJIB berakhiran _test
```

Isi `DB_URL` dan `TEST_DB_URL` di `talepuff_backend\.env`. Tabel dibuat otomatis saat Gin pertama kali menyala.

**Model AI:**

```powershell
ollama pull qwen3:4b
```

**Layanan AI (Python):**

```powershell
cd ai_services
python -m venv venv
.\venv\Scripts\Activate.ps1          # bila diblokir: Set-ExecutionPolicy -Scope Process -ExecutionPolicy RemoteSigned
pip install -r requirements-dev.txt  # requirements.txt saja bila tidak ingin menjalankan tes
```

Di VS Code, pilih interpreter `ai_services\venv\Scripts\python.exe` (lihat bagian masalah umum).

**Firebase.** Berkas `talepuff_app\android\app\google-services.json` harus ada, metode login Email/Password dan Google aktif, dan SHA-1 keystore debug laptop ini terdaftar di Firebase (lihat bagian masalah umum). `FIREBASE_PROJECT_ID` di backend adalah `project_info.project_id` dari berkas itu.

## 3. Menjalankan (urutan)

1. **PostgreSQL dan Ollama** biasanya sudah menyala sebagai layanan Windows. Cek: `ollama ps` (boleh kosong) dan `psql -U <pengguna> -d talepuff -c "select 1"`.
2. **Layanan AI:**
   ```powershell
   cd ai_services
   .\venv\Scripts\Activate.ps1
   uvicorn app.main:app --host 0.0.0.0 --port 8000 --env-file .env
   ```
   Model langsung dimuat ke GPU di latar belakang; permintaan pertama tidak perlu menunggu pemuatan.
3. **Backend:**
   ```powershell
   cd talepuff_backend
   go run .
   ```
4. **Aplikasi:** `cd talepuff_app; flutter run`, atau bangun APK:
   ```powershell
   flutter build apk --release        # hasil: build\app\outputs\flutter-apk\app-release.apk
   ```

## 4. Jaringan untuk HP asli

- Laptop dan HP harus berada di jaringan WiFi yang sama.
- Cari IP laptop: `ipconfig` (IPv4 di adapter WiFi), lalu isi `API_BASE_URL=http://<IP>:8080` di `talepuff_app\.env`. Bukan `localhost`, bukan `10.0.2.2`.
- **Firewall Windows** harus mengizinkan koneksi masuk ke port 8080 (dan 8000 bila perangkat lain mengambil audio langsung). PowerShell sebagai administrator:
  ```powershell
  New-NetFirewallRule -DisplayName "TalePuff Gin 8080" -Direction Inbound -Protocol TCP -LocalPort 8080 -Action Allow
  New-NetFirewallRule -DisplayName "TalePuff AI 8000"  -Direction Inbound -Protocol TCP -LocalPort 8000 -Action Allow
  ```
  Jaringan WiFi sebaiknya bertipe *Private*.
- IP laptop bisa berubah saat WiFi tersambung ulang. Tidak perlu membangun ulang APK: ubah di aplikasi lewat **Settings > About > Server Address** (hanya alamat jaringan lokal yang diterima), atau lewat tombol "Change server address" di layar error awal.

## 5. Pemeriksaan cepat (tanpa aplikasi)

```powershell
curl http://localhost:8080/api/health   # {"service":"talepuff","status":"ok"}
curl http://localhost:8000/             # {"Status":"Server AI TalePuff On!"}
ollama ps                               # model qwen3:4b, PROCESSOR 100% GPU setelah AI menyala
```

Dokumentasi interaktif layanan AI: `http://localhost:8000/docs`. Daftar endpoint Gin ada di `talepuff_backend\README.md`.

## 6. Menjalankan tes

```powershell
cd talepuff_backend; go test ./...                              # tes rute butuh TEST_DB_URL (database *_test)
cd ai_services;      .\venv\Scripts\python -m pytest            # folder tests/ dan benchmarks/ sengaja tidak masuk git
cd talepuff_app;     flutter analyze; flutter test
```

## 6b. Totem (Mosquitto + Pico)

Backend yang mengirim perintah ke totem; aplikasi cukup memanggil API.

1. Mosquitto lokal: lihat `mosquitto/` dan `totem_firmware/README.md` (pengguna `backend` dibuat dengan `mosquitto_passwd`).
2. Tambahkan ke `.env`: `MQTT_BROKER_URL`, `MQTT_USER`, `MQTT_PASSWORD`, `AUDIO_PUBLIC_BASE_URL` (lihat `.env.example`).
   `AUDIO_PUBLIC_BASE_URL` harus alamat yang bisa dijangkau Pico (IP laptop), bukan `localhost`.
3. Jalankan FastAPI dengan `--host 0.0.0.0` agar Pico bisa mengunduh `/audio/...`, dan buka port 8000 di firewall.
4. Endpoint (semua perlu token): `GET /api/totem`, `POST /api/totem/pair` `{device_id}`, `DELETE /api/totem`,
   `POST /api/totem/play` `{story_id, volume?}`, `POST /api/totem/command` `{action: pause|resume|stop|volume, value?}`.
   Totem hanya bisa ditautkan bila sedang terlihat online di broker; satu akun satu totem.

## 7. Masalah yang pernah terjadi

| Gejala | Penyebab | Solusi |
|---|---|---|
| VS Code menandai `import fastapi could not be resolved` padahal sudah terpasang | Pylance memakai Python lain, bukan venv; mengaktifkan venv di terminal tidak memengaruhinya | `Ctrl+Shift+P` > *Python: Select Interpreter* > pilih `ai_services\venv\Scripts\python.exe` (lewat *Enter interpreter path*), lalu *Reload Window* |
| `ModuleNotFoundError: No module named 'ai_services'` saat menyalakan uvicorn | VS Code mengisi impor otomatis relatif ke folder induk (`from ai_services.app...`) | Impor di dalam `ai_services` harus diawali `app.`. Setelah memindah atau mengganti nama berkas, jalankan `python -c "import app.main"` |
| `ModuleNotFoundError: No module named 'dotenv'` dengan `--env-file` | `python-dotenv` belum terpasang | `pip install -r requirements.txt` |
| Rute Gin baru menjawab teks polos `404 page not found` | Gin masih proses lama | Hentikan lalu jalankan ulang `go run .` |
| Login Google gagal, `DEVELOPER_ERROR` atau `sign_in_failed` | SHA-1 keystore penandatangan APK belum terdaftar di Firebase | `keytool -list -v -keystore "$env:USERPROFILE\.android\debug.keystore" -alias androiddebugkey -storepass android -keypass android`, daftarkan SHA-1 di Firebase, unduh ulang `google-services.json`. `keytool` ada di `Android Studio\jbr\bin` |
| Aplikasi menampilkan "Can't reach the server" | Gin mati, IP laptop berubah, firewall, atau beda jaringan | Cek bagian 4; uji `curl http://<IP>:8080/api/health` dari HP atau laptop lain |
| Cerita pertama terasa lama | Ollama belum memuat model | Normal bila AI baru menyala (model dimuat ±6 detik). `OLLAMA_KEEP_ALIVE` mengatur lamanya model ditahan (bawaan 30 menit) |
| Pembuatan suara gagal dengan 403 | Versi `edge-tts` lama ditolak layanan Microsoft | Gunakan versi di `requirements.txt` (7.3.1); perbarui bila terulang |
| `ffmpeg` tidak ditemukan | Tidak ada di `PATH` | Tambahkan folder `bin` ffmpeg ke `PATH`, buka terminal baru, cek `ffmpeg -version` |
| Tes backend menolak jalan | Nama database tes tidak berakhiran `_test` | Perbaiki `TEST_DB_URL` |
