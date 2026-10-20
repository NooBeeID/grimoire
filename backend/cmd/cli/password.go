package main

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

// resolvePassword: env CREATE_ADMIN_PASSWORD, lalu stdin (non-TTY), lalu prompt (TTY).
// Password tidak pernah masuk ke pesan error.
func (c *cli) resolvePassword() (string, error) {
	if v := strings.TrimRight(c.getenv("CREATE_ADMIN_PASSWORD"), "\r\n"); v != "" {
		return v, nil
	}
	var pw string
	if !c.isTTY() {
		line, err := bufio.NewReader(c.stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", errors.New("gagal membaca password dari stdin")
		}
		pw = strings.TrimRight(line, "\r\n")
	} else {
		p1, err := c.readSecret("Password: ")
		if err != nil {
			return "", errors.New("gagal membaca password")
		}
		p2, err := c.readSecret("Ulangi password: ")
		if err != nil {
			return "", errors.New("gagal membaca password")
		}
		if p1 != p2 {
			return "", errors.New("password tidak sama")
		}
		pw = p1
	}
	if pw == "" {
		return "", errors.New("password tidak boleh kosong")
	}
	return pw, nil
}
