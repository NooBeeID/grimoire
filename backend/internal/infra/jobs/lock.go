package jobs

import (
	"context"
	"database/sql/driver"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

const lockKeyPrefix = "grimoire.job."

// connProvider adalah bagian *db.DB yang dibutuhkan scheduler (sqlx.DB memenuhinya).
type connProvider interface {
	Connx(ctx context.Context) (*sqlx.Conn, error)
}

// locker memisahkan akses lock dari Scheduler agar bisa difake di unit test.
type locker interface {
	// TryLock: ok=false, err=nil bila lock dipegang pihak lain.
	// unlock dipanggil tepat sekali oleh pemanggil bila ok=true.
	TryLock(ctx context.Context, name string) (unlock func(ctx context.Context) error, ok bool, err error)
}

type pgLocker struct{ db connProvider }

func (p pgLocker) TryLock(ctx context.Context, name string) (func(context.Context) error, bool, error) {
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := p.db.Connx(ctx2)
	if err != nil {
		return nil, false, err
	}
	key := lockKeyPrefix + name
	var got bool
	if err := conn.GetContext(ctx2, &got, "SELECT pg_try_advisory_lock(hashtext($1))", key); err != nil {
		// Query gagal di tengah jalan bisa saja sudah mengambil lock: buang koneksi.
		discard(conn)
		return nil, false, err
	}
	if !got {
		_ = conn.Close()
		return nil, false, nil
	}
	return func(uctx context.Context) error {
		var released bool
		err := conn.GetContext(uctx, &released, "SELECT pg_advisory_unlock(hashtext($1))", key)
		if err != nil || !released {
			discard(conn)
			if err == nil {
				err = errors.New("jobs: advisory unlock returned false")
			}
			return err
		}
		return conn.Close()
	}, true, nil
}

// discard membuang koneksi dari pool; menutup koneksi fisik melepas lock sesi di Postgres.
func discard(conn *sqlx.Conn) {
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = conn.Close()
}
