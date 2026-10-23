// motor: classifica registros JSON Lines pela primeira regra satisfeita.
//
//	motor --entrada entrada.jsonl [--regras regras/duplicatas.json] [--explicar] > saida.txt
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
)

func main() {
	debug.SetGCPercent(-1) // processo curto e quase sem alocação: GC só atrapalha

	entrada := flag.String("entrada", "", "arquivo JSON Lines de entrada")
	regrasArq := flag.String("regras", "", "arquivo de regras (padrão: regras/duplicatas.json ao lado do binário)")
	explicar := flag.Bool("explicar", false, "anexa todas as regras que disparam em cada linha")
	flag.Parse()
	if *entrada == "" {
		fmt.Fprintln(os.Stderr, "uso: motor --entrada entrada.jsonl [--regras arquivo.json] [--explicar]")
		os.Exit(2)
	}

	m, err := carregarRegras(acharRegras(*regrasArq))
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro nas regras:", err)
		os.Exit(1)
	}

	dados, liberar, err := lerArquivo(*entrada)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro ao ler entrada:", err)
		os.Exit(1)
	}
	defer liberar()

	if err := m.processar(dados, os.Stdout, *explicar); err != nil {
		fmt.Fprintln(os.Stderr, "erro ao escrever saída:", err)
		os.Exit(1)
	}
}

func acharRegras(flagValor string) string {
	if flagValor != "" {
		return flagValor
	}
	if v := os.Getenv("MOTOR_REGRAS"); v != "" {
		return v
	}
	if exe, err := os.Executable(); err == nil {
		c := filepath.Join(filepath.Dir(exe), "regras", "duplicatas.json")
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return filepath.Join("regras", "duplicatas.json")
}

type escritor interface{ Write([]byte) (int, error) }

func (m *motor) processar(dados []byte, w escritor, explicar bool) error {
	dados = bytes.TrimPrefix(dados, []byte("\xef\xbb\xbf")) // BOM
	v := make([]Val, m.nslots)
	p := &parser{b: dados, v: v}
	out := make([]byte, 0, 1<<20)

	for p.i < len(dados) {
		p.ws()
		if p.i >= len(dados) {
			break
		}
		ini := p.i
		for k := range v {
			v[k] = Val{}
		}
		p.err = false
		if dados[p.i] == '{' {
			p.objeto(m.raiz)
			if !p.err {
				p.wsLinha()
				if p.i < len(dados) && dados[p.i] != '\n' {
					p.err = true
				}
			}
		} else {
			p.err = true
		}
		if p.err {
			// caminho de segurança: linha que o leitor rápido não aceitou
			fim := bytes.IndexByte(dados[ini:], '\n')
			if fim < 0 {
				fim = len(dados)
			} else {
				fim += ini
			}
			p.i = fim
			if !m.lento(dados[ini:fim], v) {
				fmt.Fprintf(os.Stderr, "linha ignorada (JSON inválido): %.80s\n", dados[ini:fim])
				continue
			}
		}

		id := &v[m.idSlot]
		if id.K != Num && id.K != Str {
			fmt.Fprintln(os.Stderr, "linha ignorada (sem id)")
			continue
		}
		out = append(out, "id:"...)
		out = append(out, id.S...)
		out = append(out, " - regra: "...)
		out = append(out, m.classificar(v)...)
		if explicar {
			out = append(out, "  # dispararam: "...)
			out = append(out, strings.Join(m.todasQueDisparam(v), ",")...)
		}
		out = append(out, '\n')
		if len(out) > 1<<20-256 {
			if _, err := w.Write(out); err != nil {
				return err
			}
			out = out[:0]
		}
	}
	_, err := w.Write(out)
	return err
}

// lento decodifica a linha com encoding/json e preenche os slots. Usado só
// quando o leitor rápido rejeita a linha.
func (m *motor) lento(linha []byte, v []Val) bool {
	for k := range v {
		v[k] = Val{}
	}
	d := json.NewDecoder(bytes.NewReader(linha))
	d.UseNumber()
	var obj map[string]any
	if d.Decode(&obj) != nil {
		return false
	}
	var andar func(n *node, x any)
	andar = func(n *node, x any) {
		if n.slot >= 0 {
			v[n.slot] = paraVal(x)
		}
		o, ok := x.(map[string]any)
		if !ok {
			return
		}
		for i, nome := range n.nomes {
			if y, ok := o[nome]; ok {
				andar(n.filhos[i], y)
			}
		}
	}
	andar(m.raiz, obj)
	return true
}

func paraVal(x any) Val {
	switch t := x.(type) {
	case nil:
		return Val{K: Null}
	case string:
		return Val{K: Str, S: []byte(t)}
	case bool:
		return Val{K: Bool, B: t}
	case json.Number:
		f, _ := t.Float64()
		return Val{K: Num, N: f, S: []byte(t.String())}
	}
	return Val{K: Other}
}
