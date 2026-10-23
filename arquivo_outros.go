//go:build !unix

package main

import "os"

func lerArquivo(caminho string) ([]byte, func(), error) {
	b, err := os.ReadFile(caminho)
	return b, func() {}, err
}
