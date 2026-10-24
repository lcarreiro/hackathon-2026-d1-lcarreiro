package motor

// Leitor JSON seletivo: percorre cada linha uma vez, extrai só os campos que
// as regras usam e pula o resto sem alocar memória. Em chave repetida vale a
// última ocorrência, como no json do Python e no encoding/json do Go.

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"math/bits"
	"unsafe"
)

// ---------- valores ----------

type tipo uint8

const (
	tipoAusente tipo = iota // o campo não veio na linha
	tipoNulo
	tipoTexto
	tipoNumero
	tipoBooleano
	tipoComposto // objeto ou lista
)

type valor struct {
	tipo tipo
	b    bool
	n    float64
	s    string // texto já decodificado; em números, o literal como veio (usado no id)
}

// ---------- árvore de campos ----------

// campo é um nó da árvore dos caminhos usados pelas regras: "financeiro.valor"
// vira raiz → financeiro → valor. O leitor só desce nos nós da árvore.
type campo struct {
	nomes      []string
	filhos     []*campo
	porTamanho [][]int // porTamanho[n] = índices dos filhos cujo nome tem n bytes
	slot       int     // posição do valor no vetor; -1 se o nó é só caminho
	indice     int     // número do nó na árvore (posição em leitor.visto)
}

func (c *campo) filho(nome string, novo func() *campo) *campo {
	for i, s := range c.nomes {
		if s == nome {
			return c.filhos[i]
		}
	}
	for len(c.porTamanho) <= len(nome) {
		c.porTamanho = append(c.porTamanho, nil)
	}
	c.porTamanho[len(nome)] = append(c.porTamanho[len(nome)], len(c.nomes))
	f := novo()
	c.nomes = append(c.nomes, nome)
	c.filhos = append(c.filhos, f)
	return f
}

// buscar devolve o filho com esse nome, ou nil. O tamanho do nome descarta
// quase todos os candidatos antes de comparar texto.
func (c *campo) buscar(nome []byte) *campo {
	if len(nome) >= len(c.porTamanho) {
		return nil
	}
	for _, i := range c.porTamanho[len(nome)] {
		if c.nomes[i] == string(nome) {
			return c.filhos[i]
		}
	}
	return nil
}

// zerar apaga os valores da subárvore (objeto pai repetido na mesma linha).
func (c *campo) zerar(v []valor) {
	if c.slot >= 0 {
		v[c.slot] = valor{}
	}
	for _, f := range c.filhos {
		f.zerar(v)
	}
}

// ---------- leitor ----------

type leitor struct {
	b     []byte
	i     int
	v     []valor // valores da linha atual, um por slot
	visto []int   // visto[c.indice] = última linha em que o objeto c apareceu
	linha int
	erro  bool
}

// lerLinha lê o objeto que começa em l.i e preenche l.v. Devolve false se a
// linha não está num formato que o caminho rápido aceita.
func (l *leitor) lerLinha(raiz *campo) bool {
	clear(l.v)
	l.linha++
	l.erro = false
	if l.b[l.i] != '{' {
		return false
	}
	l.lerObjeto(raiz)
	if l.erro {
		return false
	}
	for l.i < len(l.b) && (l.b[l.i] == ' ' || l.b[l.i] == '\t' || l.b[l.i] == '\r') {
		l.i++
	}
	return l.i == len(l.b) || l.b[l.i] == '\n'
}

// pularEspacos tem um atalho inline para o caso comum (não há espaço a pular).
func (l *leitor) pularEspacos() {
	if l.i < len(l.b) && l.b[l.i] > ' ' {
		return
	}
	l.pularEspacosLento()
}

func (l *leitor) pularEspacosLento() {
	for l.i < len(l.b) {
		switch l.b[l.i] {
		case ' ', '\t', '\r', '\n':
			l.i++
		default:
			return
		}
	}
}

// lerObjeto lê um objeto a partir de '{', descendo só nos filhos de c.
func (l *leitor) lerObjeto(c *campo) {
	l.i++ // '{'
	l.pularEspacos()
	if l.i < len(l.b) && l.b[l.i] == '}' {
		l.i++
		return
	}
	for {
		l.pularEspacos()
		if l.i >= len(l.b) || l.b[l.i] != '"' {
			l.erro = true
			return
		}
		nome := l.lerTexto()
		l.pularEspacos()
		if l.erro || l.i >= len(l.b) || l.b[l.i] != ':' {
			l.erro = true
			return
		}
		l.i++
		l.pularEspacos()
		if l.i >= len(l.b) {
			l.erro = true
			return
		}

		if f := c.buscar(nome); f != nil {
			l.lerValor(f)
		} else {
			l.pularValor()
		}

		l.pularEspacos()
		if l.erro || l.i >= len(l.b) {
			l.erro = true
			return
		}
		switch l.b[l.i] {
		case ',':
			l.i++
		case '}':
			l.i++
			return
		default:
			l.erro = true
			return
		}
	}
}

// lerValor lê o valor do campo c, que está na árvore.
func (l *leitor) lerValor(c *campo) {
	if len(c.filhos) > 0 {
		if l.visto[c.indice] == l.linha {
			c.zerar(l.v) // objeto pai repetido: vale só a última ocorrência
		}
		l.visto[c.indice] = l.linha
	}

	var x valor
	switch l.b[l.i] {
	case '{':
		x.tipo = tipoComposto
		if len(c.filhos) > 0 {
			l.lerObjeto(c)
		} else {
			l.pularValor()
		}
	case '[':
		x.tipo = tipoComposto
		l.pularValor()
	case '"':
		x = valor{tipo: tipoTexto, s: b2s(l.lerTexto())}
	default:
		x = l.lerLiteral()
	}
	if c.slot >= 0 {
		l.v[c.slot] = x
	}
}

// lerLiteral lê número, true, false ou null (e NaN/Infinity, que o json do
// Python aceita).
func (l *leitor) lerLiteral() valor {
	inicio := l.i
	for l.i < len(l.b) && !delimitador[l.b[l.i]] {
		l.i++
	}
	tok := l.b[inicio:l.i]
	switch string(tok) {
	case "true":
		return valor{tipo: tipoBooleano, b: true}
	case "false":
		return valor{tipo: tipoBooleano}
	case "null":
		return valor{tipo: tipoNulo}
	case "NaN":
		return valor{tipo: tipoNumero, n: math.NaN(), s: b2s(tok)}
	case "Infinity":
		return valor{tipo: tipoNumero, n: math.Inf(1), s: b2s(tok)}
	case "-Infinity":
		return valor{tipo: tipoNumero, n: math.Inf(-1), s: b2s(tok)}
	}
	n, ok := lerNumero(tok)
	if !ok {
		l.erro = true
		return valor{}
	}
	return valor{tipo: tipoNumero, n: n, s: b2s(tok)}
}

// delimitador marca os bytes que encerram um literal.
var delimitador = [256]bool{',': true, '}': true, ']': true, ' ': true, '\t': true, '\r': true, '\n': true}

// lerTexto lê um texto a partir de '"' e devolve o conteúdo decodificado.
func (l *leitor) lerTexto() []byte {
	l.i++ // '"'
	inicio := l.i
	i := acharEspecial(l.b, l.i)
	if i < len(l.b) && l.b[i] == '"' {
		l.i = i + 1
		return l.b[inicio:i]
	}
	if i < len(l.b) && l.b[i] == '\\' {
		l.i = i
		return l.lerTextoComEscape(inicio)
	}
	l.i = i
	l.erro = true
	return nil
}

// lerTextoComEscape é o caminho raro: texto com \uXXXX, \", \n...
func (l *leitor) lerTextoComEscape(inicio int) []byte {
	for l.i < len(l.b) {
		switch l.b[l.i] {
		case '\\':
			l.i += 2
			continue
		case '"':
			bruto := l.b[inicio-1 : l.i+1]
			l.i++
			var s string
			if json.Unmarshal(bruto, &s) != nil {
				l.erro = true
				return nil
			}
			return []byte(s)
		case '\n':
			l.erro = true
			return nil
		}
		l.i++
	}
	l.erro = true
	return nil
}

func (l *leitor) pularTexto() {
	i := l.i + 1
	for i < len(l.b) {
		i = acharEspecial(l.b, i)
		if i >= len(l.b) || l.b[i] == '\n' {
			break
		}
		if l.b[i] == '"' {
			l.i = i + 1
			return
		}
		i += 2 // escape
	}
	l.i = i
	l.erro = true
}

// pularValor avança sobre um valor que as regras não usam.
func (l *leitor) pularValor() {
	if l.i >= len(l.b) {
		l.erro = true
		return
	}
	switch l.b[l.i] {
	case '"':
		l.pularTexto()
	case '{', '[':
		profundidade := 0
		for l.i < len(l.b) {
			switch l.b[l.i] {
			case '"':
				l.pularTexto()
				if l.erro {
					return
				}
				continue
			case '{', '[':
				profundidade++
			case '}', ']':
				profundidade--
				if profundidade == 0 {
					l.i++
					return
				}
			case '\n':
				l.erro = true
				return
			}
			l.i++
		}
		l.erro = true
	default:
		inicio := l.i
		for l.i < len(l.b) && !delimitador[l.b[l.i]] {
			l.i++
		}
		if l.i == inicio {
			l.erro = true
		}
	}
}

// ---------- busca de 8 em 8 bytes (SWAR) ----------

const (
	uns   = 0x0101010101010101
	altos = 0x8080808080808080
)

// temZero liga o bit alto de cada byte de x que é zero.
func temZero(x uint64) uint64 { return (x - uns) & ^x & altos }

// especial marca os bytes que encerram o caminho rápido de um texto.
var especial = [256]bool{'"': true, '\\': true, '\n': true}

// acharEspecial devolve a posição do primeiro '"', '\\' ou '\n' a partir de i,
// testando 8 bytes por vez; len(b) se não houver.
func acharEspecial(b []byte, i int) int {
	for i+8 <= len(b) {
		x := binary.LittleEndian.Uint64(b[i:])
		t := temZero(x^(0x22*uns)) | temZero(x^(0x5c*uns)) | temZero(x^(0x0a*uns))
		if t != 0 {
			return i + bits.TrailingZeros64(t)/8
		}
		i += 8
	}
	for i < len(b) && !especial[b[i]] {
		i++
	}
	return i
}

// b2s vê os bytes como string sem copiar. Os bytes vêm da entrada, que não
// muda enquanto Processar roda.
func b2s(b []byte) string { return unsafe.String(unsafe.SliceData(b), len(b)) }
