// Package touch adalah kontrak bus project.apikey.touch (Publish; provider T04, consumer T07).
//
// Dipanggil consumer setelah verifikasi sukses: d.Bus.Publish(ctx, constants.BUS_PROJECT_APIKEY_TOUCH, touch.Request{KeyID: id}).
// Provider memperbarui last_used_at paling sering sekali per menit per key (ditegakkan di SQL, aman multi-instance).
// KeyID bukan uuid atau tidak ada -> no-op (log debug). Error DB -> log warn, tidak dikembalikan.
// Tipe request salah -> error membungkus bus.ErrBadRequestType (bus mencatatnya sebagai "bus publish failed").
// Tidak ada respons.
package touch

type Request struct {
	KeyID string
}
