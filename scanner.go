package main

// Leitor JSON seletivo: percorre cada linha uma vez, extrai só os campos que
// as regras usam e pula o resto sem alocar. Chave duplicada: vale a última
// (mesmo comportamento do json do Python e do encoding/json do Go).

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"math/bits"
	"strconv"
	"unsafe"
)

type Kind uint8

const (
	Absent Kind = iota
	Null
	Str
	Num
	Bool
	Other // objeto ou lista
)

type Val struct {
	K Kind
	B bool
	N float64
	S []byte // conteúdo do texto (já decodificado); para números, o literal cru
}

// node: árvore de caminhos usados pelas regras.
type node struct {
	nomes    []string
	filhos   []*node
	tamanhos uint64 // bit i ligado = existe filho com nome de i bytes (descarte rápido)
	slot     int    // índice do valor, -1 se o caminho só é intermediário
	lo, hi   int    // faixa de slots desta subárvore (para zerar em chave repetida)
}

func novoNode() *node { return &node{slot: -1} }

func (n *node) filho(k string) *node {
	for i, s := range n.nomes {
		if s == k {
			return n.filhos[i]
		}
	}
	c := novoNode()
	if len(k) < 64 {
		n.tamanhos |= 1 << uint(len(k))
	} else {
		n.tamanhos = ^uint64(0)
	}
	n.nomes = append(n.nomes, k)
	n.filhos = append(n.filhos, c)
	return c
}

// fechar calcula lo/hi; os slots de uma subárvore não são necessariamente
// contíguos, então guardamos min/max e zeramos a faixa (barato: poucos slots).
func (n *node) fechar(_ int) (int, int) {
	lo, hi := math.MaxInt, -1
	if n.slot >= 0 {
		lo, hi = n.slot, n.slot
	}
	for _, c := range n.filhos {
		a, b := c.fechar(0)
		lo, hi = min(lo, a), max(hi, b)
	}
	n.lo, n.hi = lo, hi
	return lo, hi
}

func (n *node) zerar(v []Val) {
	if n.slot >= 0 {
		v[n.slot] = Val{}
	}
	for _, c := range n.filhos {
		c.zerar(v)
	}
}

func (n *node) busca(k []byte) *node {
	if len(k) < 64 && n.tamanhos&(1<<uint(len(k))) == 0 {
		return nil
	}
	for i, s := range n.nomes {
		if s == b2s(k) {
			return n.filhos[i]
		}
	}
	return nil
}

func b2s(b []byte) string { return unsafe.String(unsafe.SliceData(b), len(b)) }

type parser struct {
	b   []byte
	i   int
	v   []Val
	err bool
}

func (p *parser) ws() {
	if p.i < len(p.b) && p.b[p.i] > ' ' {
		return
	}
	p.wsLento()
}

func (p *parser) wsLento() {
	for p.i < len(p.b) {
		switch p.b[p.i] {
		case ' ', '\t', '\r', '\n':
			p.i++
		default:
			return
		}
	}
}

// linha: espaço em branco até '\n' (exclusive) é tolerado.
func (p *parser) wsLinha() {
	for p.i < len(p.b) {
		switch p.b[p.i] {
		case ' ', '\t', '\r':
			p.i++
		default:
			return
		}
	}
}

// especial marca bytes que encerram o caminho rápido de um texto.
var especial = [256]bool{'"': true, '\\': true, '\n': true}

const (
	uns   = 0x0101010101010101
	altos = 0x8080808080808080
)

func temZero(x uint64) uint64 { return (x - uns) & ^x & altos }

// achar devolve o índice do primeiro '"', '\\' ou '\n' a partir de i,
// testando 8 bytes por vez (SWAR); len(b) se não houver.
func achar(b []byte, i int) int {
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

// str lê um texto a partir de '"' e devolve o conteúdo decodificado.
func (p *parser) str() []byte {
	p.i++ // '"'
	ini := p.i
	b := p.b
	i := achar(b, p.i)
	if i < len(b) && b[i] == '"' {
		p.i = i + 1
		return b[ini:i]
	}
	if i < len(b) && b[i] == '\\' {
		p.i = i
		return p.strEscapada(ini)
	}
	p.i = i
	p.err = true
	return nil
}

// caminho lento, raro: texto com escapes (\uXXXX, \", ...).
func (p *parser) strEscapada(ini int) []byte {
	b := p.b
	for p.i < len(b) {
		c := b[p.i]
		if c == '\\' {
			p.i += 2
			continue
		}
		if c == '"' {
			raw := b[ini-1 : p.i+1]
			p.i++
			var s string
			if json.Unmarshal(raw, &s) != nil {
				p.err = true
				return nil
			}
			return []byte(s)
		}
		if c == '\n' {
			break
		}
		p.i++
	}
	p.err = true
	return nil
}

func (p *parser) pularStr() {
	b := p.b
	i := p.i + 1
	for i < len(b) {
		i = achar(b, i)
		if i >= len(b) || b[i] == '\n' {
			break
		}
		if b[i] == '"' {
			p.i = i + 1
			return
		}
		i += 2 // escape
	}
	p.i = i
	p.err = true
}

func (p *parser) pularValor() {
	b := p.b
	if p.i >= len(b) {
		p.err = true
		return
	}
	switch b[p.i] {
	case '"':
		p.pularStr()
	case '{', '[':
		prof := 0
		for p.i < len(b) {
			switch b[p.i] {
			case '"':
				p.pularStr()
				if p.err {
					return
				}
				continue
			case '{', '[':
				prof++
			case '}', ']':
				prof--
				if prof == 0 {
					p.i++
					return
				}
			case '\n':
				p.err = true
				return
			}
			p.i++
		}
		p.err = true
	default:
		ini := p.i
		for p.i < len(b) {
			switch b[p.i] {
			case ',', '}', ']', ' ', '\t', '\r', '\n':
				if p.i == ini {
					p.err = true
				}
				return
			}
			p.i++
		}
	}
}

// literal: número, true, false, null (e NaN/Infinity, aceitos pelo json do Python).
func (p *parser) literal(dst *Val) {
	ini := p.i
	b := p.b
	for p.i < len(b) {
		c := b[p.i]
		if c == ',' || c == '}' || c == ']' || c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			break
		}
		p.i++
	}
	tok := b[ini:p.i]
	switch b2s(tok) {
	case "true":
		*dst = Val{K: Bool, B: true}
	case "false":
		*dst = Val{K: Bool}
	case "null":
		*dst = Val{K: Null}
	case "NaN":
		*dst = Val{K: Num, N: math.NaN(), S: tok}
	case "Infinity":
		*dst = Val{K: Num, N: math.Inf(1), S: tok}
	case "-Infinity":
		*dst = Val{K: Num, N: math.Inf(-1), S: tok}
	default:
		if !numeroJSON(tok) {
			p.err = true
			return
		}
		f, err := strconv.ParseFloat(b2s(tok), 64)
		if err != nil && !math.IsInf(f, 0) { // fora de faixa vira ±Inf, como no Python
			p.err = true
			return
		}
		*dst = Val{K: Num, N: f, S: tok}
	}
}

// numeroJSON valida a gramática de número do JSON (sem +, sem zero à esquerda, sem hexa).
func numeroJSON(t []byte) bool {
	i, n := 0, len(t)
	if i < n && t[i] == '-' {
		i++
	}
	if i >= n {
		return false
	}
	if t[i] == '0' {
		i++
	} else if t[i] >= '1' && t[i] <= '9' {
		for i < n && t[i] >= '0' && t[i] <= '9' {
			i++
		}
	} else {
		return false
	}
	if i < n && t[i] == '.' {
		i++
		d := i
		for i < n && t[i] >= '0' && t[i] <= '9' {
			i++
		}
		if i == d {
			return false
		}
	}
	if i < n && (t[i] == 'e' || t[i] == 'E') {
		i++
		if i < n && (t[i] == '+' || t[i] == '-') {
			i++
		}
		d := i
		for i < n && t[i] >= '0' && t[i] <= '9' {
			i++
		}
		if i == d {
			return false
		}
	}
	return i == n
}

// objeto lê um objeto JSON a partir de '{' preenchendo os slots da árvore n.
func (p *parser) objeto(n *node) {
	p.i++ // '{'
	p.ws()
	if p.i < len(p.b) && p.b[p.i] == '}' {
		p.i++
		return
	}
	for !p.err {
		p.ws()
		if p.i >= len(p.b) || p.b[p.i] != '"' {
			p.err = true
			return
		}
		k := p.str()
		if p.err {
			return
		}
		p.ws()
		if p.i >= len(p.b) || p.b[p.i] != ':' {
			p.err = true
			return
		}
		p.i++
		p.ws()
		if p.i >= len(p.b) {
			p.err = true
			return
		}
		c := n.busca(k)
		if c == nil {
			p.pularValor()
		} else {
			p.valor(c)
		}
		if p.err {
			return
		}
		p.ws()
		if p.i >= len(p.b) {
			p.err = true
			return
		}
		switch p.b[p.i] {
		case ',':
			p.i++
		case '}':
			p.i++
			return
		default:
			p.err = true
			return
		}
	}
}

func (p *parser) valor(c *node) {
	if c.hi >= 0 && len(c.filhos) > 0 {
		c.zerar(p.v) // chave repetida ou pai trocado: descarta o que veio antes
	}
	ch := p.b[p.i]
	switch {
	case ch == '{':
		if c.slot >= 0 {
			p.v[c.slot] = Val{K: Other}
		}
		if len(c.filhos) > 0 {
			p.objeto(c)
		} else {
			p.pularValor()
		}
	case ch == '[':
		if c.slot >= 0 {
			p.v[c.slot] = Val{K: Other}
		}
		p.pularValor()
	case ch == '"':
		s := p.str()
		if c.slot >= 0 {
			p.v[c.slot] = Val{K: Str, S: s}
		}
	default:
		var x Val
		p.literal(&x)
		if c.slot >= 0 {
			p.v[c.slot] = x
		}
	}
}
