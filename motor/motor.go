// Pacote motor classifica registros JSON Lines pela primeira regra satisfeita.
// Nada aqui conhece duplicatas: campos, operadores e valores vêm do arquivo de
// regras (veja regras.go).
//
// Fluxo de cada linha: o leitor (leitor.go) extrai só os campos que as regras
// usam para um vetor de valores; as regras compiladas (condicoes.go) são
// avaliadas em ordem e a primeira verdadeira dá o código da saída.
package motor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

const tamanhoBuffer = 1 << 20 // a saída é escrita em blocos de 1 MB

type regra struct {
	codigo string
	se     condicao
}

// Motor guarda as regras compiladas e a árvore dos campos que elas usam.
// Um Motor pode ser reaproveitado, mas Processar não deve rodar em paralelo
// sobre o mesmo Motor.
type Motor struct {
	padrao  string // código de saída quando nenhuma regra é satisfeita
	regras  []regra
	raiz    *campo
	nSlots  int // quantidade de campos com valor (tamanho do vetor de valores)
	nCampos int // quantidade de nós da árvore de campos
	slotID  int
}

// Processar classifica cada linha de dados e escreve "id:<id> - regra: <código>"
// em w. Com explicar, acrescenta todas as regras satisfeitas pela linha.
func (m *Motor) Processar(dados []byte, w io.Writer, explicar bool) error {
	dados = bytes.TrimPrefix(dados, []byte("\xef\xbb\xbf")) // BOM do UTF-8
	l := &leitor{
		b:     dados,
		v:     make([]valor, m.nSlots),
		visto: make([]int, m.nCampos),
	}
	out := make([]byte, 0, tamanhoBuffer)

	for {
		l.pularEspacos()
		if l.i >= len(dados) {
			break
		}
		inicio := l.i
		if !l.lerLinha(m.raiz) {
			// Caminho de segurança: a linha está num formato que o leitor
			// rápido não aceita; tenta de novo com encoding/json.
			fim := bytes.IndexByte(dados[inicio:], '\n')
			if fim < 0 {
				fim = len(dados)
			} else {
				fim += inicio
			}
			l.i = fim
			if !m.lerLinhaLenta(dados[inicio:fim], l.v) {
				fmt.Fprintf(os.Stderr, "linha ignorada (JSON inválido): %.80s\n", dados[inicio:fim])
				continue
			}
		}

		id := &l.v[m.slotID]
		if id.tipo != tipoNumero && id.tipo != tipoTexto {
			fmt.Fprintln(os.Stderr, "linha ignorada (sem id)")
			continue
		}
		out = append(out, "id:"...)
		out = append(out, id.s...)
		out = append(out, " - regra: "...)
		out = append(out, m.classificar(l.v)...)
		if explicar {
			out = append(out, "  # dispararam: "...)
			out = append(out, strings.Join(m.todasSatisfeitas(l.v), ",")...)
		}
		out = append(out, '\n')

		if len(out) > tamanhoBuffer-256 {
			if _, err := w.Write(out); err != nil {
				return err
			}
			out = out[:0]
		}
	}
	_, err := w.Write(out)
	return err
}

// classificar devolve o código da primeira regra satisfeita.
func (m *Motor) classificar(v []valor) string {
	for i := range m.regras {
		if m.regras[i].se.avaliar(v) {
			return m.regras[i].codigo
		}
	}
	return m.padrao
}

// todasSatisfeitas lista todas as regras satisfeitas (modo --explicar).
func (m *Motor) todasSatisfeitas(v []valor) []string {
	var out []string
	for i := range m.regras {
		if m.regras[i].se.avaliar(v) {
			out = append(out, m.regras[i].codigo)
		}
	}
	return out
}

// lerLinhaLenta decodifica a linha com encoding/json e preenche os valores.
// Só é usada quando o leitor rápido rejeita a linha.
func (m *Motor) lerLinhaLenta(linha []byte, v []valor) bool {
	clear(v)
	d := json.NewDecoder(bytes.NewReader(linha))
	d.UseNumber()
	var obj map[string]any
	if d.Decode(&obj) != nil {
		return false
	}
	var preencher func(c *campo, x any)
	preencher = func(c *campo, x any) {
		if c.slot >= 0 {
			v[c.slot] = valorDeJSON(x)
		}
		o, ok := x.(map[string]any)
		if !ok {
			return
		}
		for i, nome := range c.nomes {
			if y, ok := o[nome]; ok {
				preencher(c.filhos[i], y)
			}
		}
	}
	preencher(m.raiz, obj)
	return true
}

func valorDeJSON(x any) valor {
	switch t := x.(type) {
	case nil:
		return valor{tipo: tipoNulo}
	case string:
		return valor{tipo: tipoTexto, s: t}
	case bool:
		return valor{tipo: tipoBooleano, b: t}
	case json.Number:
		f, _ := t.Float64()
		return valor{tipo: tipoNumero, n: f, s: t.String()}
	}
	return valor{tipo: tipoComposto}
}
