-- Seeder DEV: super admin awal admin@noobee.id / SeCr3t (hash argon2id, param sama dengan default app).
-- must_change_password = true -> login pertama dipaksa ganti password.
-- Idempoten: tidak menimpa akun yang sudah ada. JANGAN dipakai di produksi.
INSERT INTO users (email, name, password_hash, role, is_active, must_change_password, created_by_label)
VALUES ('admin@noobee.id', 'Super Admin',
        '$argon2id$v=19$m=65536,t=3,p=2$eb0A+85eJpBvWWFGpv6/HQ$rGwXoN1Ab18RtZhVtREXDzFfVc3YMr7WYCv1YbvmnHU',
        'super_admin', true, true, 'seed')
ON CONFLICT (email) DO NOTHING;
