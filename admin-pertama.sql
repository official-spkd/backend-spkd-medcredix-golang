-- Membuat RS pertama + akun Super Admin pertama di database produksi (SEED_DEMO=false = database kosong).
-- Jalankan SEKALI dari folder backend di VPS:
--   docker compose -f docker-compose.prod.yml exec -T db psql -U medcredix -d medcredix -v email='admin@spkd.ac.id' -v sandi='SandiKuatMinimal12' < admin-pertama.sql
-- Ganti email & sandi di perintah di atas. Setelah login, RS lain bisa ditambah dari menu Admin.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

INSERT INTO hospitals (id, name, code, hospital_class, province, city)
VALUES ('h_spkd', 'RS Demo SPKD', 'SPKD-001', 'C', 'DKI Jakarta', 'Jakarta')
ON CONFLICT (id) DO NOTHING;

-- Hash bcrypt (format $2a$, sama dengan yang dipakai backend Go).
INSERT INTO users (id, hospital_id, email, password_hash, name, role)
VALUES ('u_superadmin', 'h_spkd', :'email', crypt(:'sandi', gen_salt('bf', 10)), 'Super Admin', 'super_admin')
ON CONFLICT (email) DO NOTHING;
