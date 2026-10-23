//go:build unix

package main

import (
	"os"
	"syscall"
)

// lerArquivo mapeia o arquivo em memória (sem cópia); cai para ReadFile se falhar.
func lerArquivo(caminho string) ([]byte, func(), error) {
	f, err := os.Open(caminho)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if st.Size() == 0 {
		return nil, func() {}, nil
	}
	b, err := syscall.Mmap(int(f.Fd()), 0, int(st.Size()), syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		b, err := os.ReadFile(caminho)
		return b, func() {}, err
	}
	return b, func() { syscall.Munmap(b) }, nil
}
