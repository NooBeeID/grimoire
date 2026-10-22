package base

// AAD membangun additional authenticated data enkripsi PAT: mengikat ciphertext ke
// (user, provider, host) sehingga baris yang dipindah tidak bisa dibuka.
func AAD(userID string, p Provider, host string) []byte {
	return []byte("grimoire.git_credential.v1|" + userID + "|" + string(p) + "|" + host)
}
