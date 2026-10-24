package motor

// Predicados compilados. Seguem a seção "Semântica" do enunciado: campo
// ausente, nulo ou de outro tipo torna qualquer comparação falsa, inclusive !=.

import (
	"cmp"
	"slices"
	"strings"
)

type condicao interface {
	avaliar(v []valor) bool
}

// ---------- combinadores ----------

type todas []condicao
type qualquer []condicao
type nao struct{ c condicao }

func (c todas) avaliar(v []valor) bool {
	for _, x := range c {
		if !x.avaliar(v) {
			return false
		}
	}
	return true
}

func (c qualquer) avaliar(v []valor) bool {
	for _, x := range c {
		if x.avaliar(v) {
			return true
		}
	}
	return false
}

func (c nao) avaliar(v []valor) bool { return !c.c.avaliar(v) }

// ---------- comparações ----------

type operador uint8

const (
	opIgual operador = iota
	opDiferente
	opMenor
	opMenorIgual
	opMaior
	opMaiorIgual
)

// tipoComparacao é o tipo que os dois lados precisam ter para a comparação valer.
type tipoComparacao uint8

const (
	cmpTexto tipoComparacao = iota
	cmpNumero
	cmpData // texto AAAA-MM-DD válido; a ordem do texto é a ordem cronológica
	cmpBooleano
)

func aplicar[T cmp.Ordered](op operador, a, b T) bool {
	switch op {
	case opIgual:
		return a == b
	case opDiferente:
		return a != b
	case opMenor:
		return a < b
	case opMenorIgual:
		return a <= b
	case opMaior:
		return a > b
	default:
		return a >= b
	}
}

func temTipo(t tipoComparacao, x *valor) bool {
	switch t {
	case cmpTexto:
		return x.tipo == tipoTexto
	case cmpNumero:
		return x.tipo == tipoNumero
	case cmpData:
		return x.tipo == tipoTexto && dataValida(x.s)
	default:
		return x.tipo == tipoBooleano
	}
}

// comparar aplica op a x e y; é falso se algum dos dois não tem o tipo t.
func comparar(t tipoComparacao, op operador, x, y *valor) bool {
	if !temTipo(t, x) || !temTipo(t, y) {
		return false
	}
	switch t {
	case cmpNumero:
		return aplicar(op, x.n, y.n)
	case cmpBooleano:
		return aplicar(op, paraInt(x.b), paraInt(y.b))
	default:
		return aplicar(op, x.s, y.s)
	}
}

func paraInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// As comparações com valor fixo têm um tipo cada, porque são avaliadas em
// toda linha: a versão especializada evita decidir o tipo a cada chamada.

// campo <op> texto fixo
type comparaTexto struct {
	campo int
	op    operador
	s     string
}

func (c comparaTexto) avaliar(v []valor) bool {
	x := &v[c.campo]
	return x.tipo == tipoTexto && aplicar(c.op, x.s, c.s)
}

// campo <op> data fixa (a data fixa já foi validada na compilação)
type comparaData struct {
	campo int
	op    operador
	data  string
}

func (c comparaData) avaliar(v []valor) bool {
	x := &v[c.campo]
	return x.tipo == tipoTexto && dataValida(x.s) && aplicar(c.op, x.s, c.data)
}

// campo <op> número fixo
type comparaNumero struct {
	campo int
	op    operador
	n     float64
}

func (c comparaNumero) avaliar(v []valor) bool {
	x := &v[c.campo]
	return x.tipo == tipoNumero && aplicar(c.op, x.n, c.n)
}

// campo = booleano ou campo != booleano
type comparaBooleano struct {
	campo int
	igual bool // true para =, false para !=
	b     bool
}

func (c comparaBooleano) avaliar(v []valor) bool {
	x := &v[c.campo]
	return x.tipo == tipoBooleano && (x.b == c.b) == c.igual
}

// campo <op> outro campo
type entreCampos struct {
	a, b int
	op   operador
	tipo tipoComparacao
}

func (c entreCampos) avaliar(v []valor) bool {
	return comparar(c.tipo, c.op, &v[c.a], &v[c.b])
}

// campo em [textos]
type pertenceTexto struct {
	campo  int
	opcoes []string
}

func (c pertenceTexto) avaliar(v []valor) bool {
	x := &v[c.campo]
	return x.tipo == tipoTexto && slices.Contains(c.opcoes, x.s)
}

// campo em [números]
type pertenceNumero struct {
	campo  int
	opcoes []float64
}

func (c pertenceNumero) avaliar(v []valor) bool {
	x := &v[c.campo]
	return x.tipo == tipoNumero && slices.Contains(c.opcoes, x.n)
}

// campo começa com prefixo (só texto)
type comecaCom struct {
	campo   int
	prefixo string
}

func (c comecaCom) avaliar(v []valor) bool {
	x := &v[c.campo]
	return x.tipo == tipoTexto && strings.HasPrefix(x.s, c.prefixo)
}

// campo vazio: ausente, nulo ou texto ""
type vazio struct{ campo int }

func (c vazio) avaliar(v []valor) bool {
	x := &v[c.campo]
	return x.tipo == tipoAusente || x.tipo == tipoNulo || (x.tipo == tipoTexto && x.s == "")
}

// dataValida: AAAA-MM-DD com dígitos ASCII, ano >= 1 e dia que existe no mês
// (bissexto incluído).
func dataValida(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for _, i := range [...]int{0, 1, 2, 3, 5, 6, 8, 9} {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	ano := int(s[0]-'0')*1000 + int(s[1]-'0')*100 + int(s[2]-'0')*10 + int(s[3]-'0')
	mes := int(s[5]-'0')*10 + int(s[6]-'0')
	dia := int(s[8]-'0')*10 + int(s[9]-'0')
	if ano < 1 || mes < 1 || mes > 12 || dia < 1 {
		return false
	}
	dias := [...]int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}[mes-1]
	if mes == 2 && ano%4 == 0 && (ano%100 != 0 || ano%400 == 0) {
		dias = 29
	}
	return dia <= dias
}
