// Comando motor: classifica um arquivo JSON Lines pela primeira regra satisfeita.
//
//	motor --entrada entrada.jsonl [--regras regras/duplicatas.json] [--explicar] > saida.txt
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	"duplicatas/motor"
)

func main() {
	debug.SetGCPercent(-1) // processo curto e quase sem alocação: o GC só custaria tempo

	entrada := flag.String("entrada", "", "arquivo JSON Lines de entrada")
	regras := flag.String("regras", "", "arquivo de regras (padrão: regras/duplicatas.json)")
	explicar := flag.Bool("explicar", false, "mostra todas as regras que disparam em cada linha")
	flag.Parse()
	if *entrada == "" {
		fmt.Fprintln(os.Stderr, "uso: motor --entrada entrada.jsonl [--regras arquivo.json] [--explicar]")
		os.Exit(2)
	}

	m, err := motor.Carregar(caminhoDasRegras(*regras))
	if err != nil {
		falhar("erro nas regras:", err)
	}

	// ReadFile em vez de mmap: no Docker Desktop o -v passa por um compartilhamento
	// 9p, e o mmap busca página por página (~10 s a mais no set grande).
	dados, err := os.ReadFile(*entrada)
	if err != nil {
		falhar("erro ao ler entrada:", err)
	}

	if err := m.Processar(dados, os.Stdout, *explicar); err != nil {
		falhar("erro ao escrever saída:", err)
	}
}

// caminhoDasRegras escolhe o arquivo de regras, nesta ordem: flag --regras,
// variável MOTOR_REGRAS, regras/ ao lado do binário, regras/ na pasta atual.
func caminhoDasRegras(flagRegras string) string {
	if flagRegras != "" {
		return flagRegras
	}
	if v := os.Getenv("MOTOR_REGRAS"); v != "" {
		return v
	}
	padrao := filepath.Join("regras", "duplicatas.json")
	if exe, err := os.Executable(); err == nil {
		c := filepath.Join(filepath.Dir(exe), padrao)
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return padrao
}

func falhar(msg string, err error) {
	fmt.Fprintln(os.Stderr, msg, err)
	os.Exit(1)
}
