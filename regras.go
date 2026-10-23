package main

// Carga e "compilação" das regras declarativas (JSON) em predicados Go.
// Nada aqui conhece duplicatas: campos, operadores e valores vêm do arquivo.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ---------- formato do arquivo ----------

type arquivoRegras struct {
	Dominio   string            `json:"dominio"`
	Padrao    string            `json:"padrao"`
	Variaveis map[string]string `json:"variaveis"`
	Regras    []regraJSON       `json:"regras"`
}

type regraJSON struct {
	Codigo    string   `json:"codigo"`
	Categoria string   `json:"categoria"`
	Se        condJSON `json:"se"`
}

type condJSON struct {
	Todas    []condJSON      `json:"todas"`
	Qualquer []condJSON      `json:"qualquer"`
	Nao      *condJSON       `json:"nao"`
	Campo    string          `json:"campo"`
	Op       string          `json:"op"`
	Tipo     string          `json:"tipo"`
	Valor    json.RawMessage `json:"valor"`
	CampoRef string          `json:"campo_ref"`
}

// ---------- predicados compilados ----------

type cond interface{ eval(v []Val) bool }

type todas []cond
type qualquer []cond
type nao struct{ c cond }

func (c todas) eval(v []Val) bool {
	for _, x := range c {
		if !x.eval(v) {
			return false
		}
	}
	return true
}
func (c qualquer) eval(v []Val) bool {
	for _, x := range c {
		if x.eval(v) {
			return true
		}
	}
	return false
}
func (c nao) eval(v []Val) bool { return !c.c.eval(v) }

// operadores de comparação
const (
	opEq = iota
	opNe
	opLt
	opLe
	opGt
	opGe
)

var nomesOp = map[string]int{"=": opEq, "!=": opNe, "<": opLt, "<=": opLe, ">": opGt, ">=": opGe}

func cmpNum(op int, a, b float64) bool {
	switch op {
	case opEq:
		return a == b
	case opNe:
		return a != b
	case opLt:
		return a < b
	case opLe:
		return a <= b
	case opGt:
		return a > b
	default:
		return a >= b
	}
}

func cmpStr(op int, a, b string) bool {
	switch op {
	case opEq:
		return a == b
	case opNe:
		return a != b
	case opLt:
		return a < b
	case opLe:
		return a <= b
	case opGt:
		return a > b
	default:
		return a >= b
	}
}

// texto
type txtCmp struct {
	slot, op int
	s        string
}

func (c txtCmp) eval(v []Val) bool {
	x := &v[c.slot]
	return x.K == Str && cmpStr(c.op, b2s(x.S), c.s)
}

type txtEm struct {
	slot int
	set  []string
}

func (c txtEm) eval(v []Val) bool {
	x := &v[c.slot]
	if x.K != Str {
		return false
	}
	s := b2s(x.S)
	for _, e := range c.set {
		if s == e {
			return true
		}
	}
	return false
}

type comecaCom struct {
	slot int
	p    string
}

func (c comecaCom) eval(v []Val) bool {
	x := &v[c.slot]
	return x.K == Str && strings.HasPrefix(b2s(x.S), c.p)
}

// número
type numCmp struct {
	slot, op int
	n        float64
}

func (c numCmp) eval(v []Val) bool {
	x := &v[c.slot]
	return x.K == Num && cmpNum(c.op, x.N, c.n)
}

type numEm struct {
	slot int
	set  []float64
}

func (c numEm) eval(v []Val) bool {
	x := &v[c.slot]
	if x.K != Num {
		return false
	}
	for _, e := range c.set {
		if x.N == e {
			return true
		}
	}
	return false
}

// booleano
type boolCmp struct {
	slot, op int
	b        bool
}

func (c boolCmp) eval(v []Val) bool {
	x := &v[c.slot]
	if x.K != Bool {
		return false
	}
	if c.op == opEq {
		return x.B == c.b
	}
	return x.B != c.b
}

// data (AAAA-MM-DD válida; comparação lexicográfica equivale à cronológica)
type dataCmp struct {
	slot, op int
	d        string
}

func (c dataCmp) eval(v []Val) bool {
	x := &v[c.slot]
	return x.K == Str && dataValida(x.S) && cmpStr(c.op, b2s(x.S), c.d)
}

// vazio: ausente, nulo ou texto ""
type vazio struct{ slot int }

func (c vazio) eval(v []Val) bool {
	x := &v[c.slot]
	return x.K == Absent || x.K == Null || (x.K == Str && len(x.S) == 0)
}

// campo contra campo, com o tipo exigido dos dois lados
type campoCmp struct {
	a, b, op int
	tipo     string
}

func (c campoCmp) eval(v []Val) bool {
	x, y := &v[c.a], &v[c.b]
	switch c.tipo {
	case "numero":
		return x.K == Num && y.K == Num && cmpNum(c.op, x.N, y.N)
	case "texto":
		return x.K == Str && y.K == Str && cmpStr(c.op, b2s(x.S), b2s(y.S))
	case "data":
		return x.K == Str && y.K == Str && dataValida(x.S) && dataValida(y.S) && cmpStr(c.op, b2s(x.S), b2s(y.S))
	case "booleano":
		if x.K != Bool || y.K != Bool {
			return false
		}
		if c.op == opEq {
			return x.B == y.B
		}
		return x.B != y.B
	}
	return false
}

// ---------- compilação ----------

type regra struct {
	codigo string
	c      cond
}

type motor struct {
	padrao string
	regras []regra
	raiz   *node
	nslots int
	idSlot int
}

func carregarRegras(caminho string) (*motor, error) {
	b, err := os.ReadFile(caminho)
	if err != nil {
		return nil, err
	}
	var a arquivoRegras
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, fmt.Errorf("%s: %w", caminho, err)
	}
	m := &motor{padrao: a.Padrao, raiz: novoNode()}
	if m.padrao == "" {
		m.padrao = "SEM_REGRA"
	}
	m.idSlot = m.slot("id")
	for i, r := range a.Regras {
		if r.Codigo == "" {
			return nil, fmt.Errorf("regra #%d sem codigo", i+1)
		}
		c, err := m.compilar(r.Se, a.Variaveis)
		if err != nil {
			return nil, fmt.Errorf("regra %s: %w", r.Codigo, err)
		}
		m.regras = append(m.regras, regra{r.Codigo, c})
	}
	m.raiz.fechar(0)
	return m, nil
}

// slot devolve o índice do campo (caminho com pontos), criando-o se preciso.
func (m *motor) slot(caminho string) int {
	n := m.raiz
	for _, k := range strings.Split(caminho, ".") {
		n = n.filho(k)
	}
	if n.slot < 0 {
		n.slot = m.nslots
		m.nslots++
	}
	return n.slot
}

func (m *motor) compilar(c condJSON, vars map[string]string) (cond, error) {
	switch {
	case c.Todas != nil:
		return m.lista(c.Todas, vars, false)
	case c.Qualquer != nil:
		return m.lista(c.Qualquer, vars, true)
	case c.Nao != nil:
		x, err := m.compilar(*c.Nao, vars)
		return nao{x}, err
	}
	if c.Campo == "" {
		return nil, fmt.Errorf("condição sem campo")
	}
	s := m.slot(c.Campo)

	switch c.Op {
	case "vazio":
		return vazio{s}, nil
	case "comeca_com":
		var p string
		if err := json.Unmarshal(c.Valor, &p); err != nil {
			return nil, fmt.Errorf("comeca_com exige valor texto")
		}
		return comecaCom{s, p}, nil
	case "em":
		var lista []json.RawMessage
		if err := json.Unmarshal(c.Valor, &lista); err != nil || len(lista) == 0 {
			return nil, fmt.Errorf("em exige lista")
		}
		var ss []string
		var ns []float64
		for _, e := range lista {
			var str string
			var f float64
			if json.Unmarshal(e, &str) == nil {
				ss = append(ss, str)
			} else if json.Unmarshal(e, &f) == nil {
				ns = append(ns, f)
			} else {
				return nil, fmt.Errorf("valor inválido em lista: %s", e)
			}
		}
		if len(ss) > 0 && len(ns) > 0 {
			return nil, fmt.Errorf("lista mistura texto e número")
		}
		if len(ns) > 0 {
			return numEm{s, ns}, nil
		}
		return txtEm{s, ss}, nil
	}

	op, ok := nomesOp[c.Op]
	if !ok {
		return nil, fmt.Errorf("operador desconhecido %q", c.Op)
	}

	if c.CampoRef != "" {
		switch c.Tipo {
		case "numero", "texto", "data", "booleano":
		default:
			return nil, fmt.Errorf("campo_ref exige tipo (numero, texto, data, booleano)")
		}
		return campoCmp{s, m.slot(c.CampoRef), op, c.Tipo}, nil
	}

	var lit any
	if err := json.Unmarshal(c.Valor, &lit); err != nil {
		return nil, fmt.Errorf("valor ausente ou inválido")
	}
	if str, ok := lit.(string); ok && strings.HasPrefix(str, "$") {
		v, ok := vars[str[1:]]
		if !ok {
			return nil, fmt.Errorf("variável %s não definida", str)
		}
		lit = v
	}
	switch x := lit.(type) {
	case string:
		if c.Tipo == "data" {
			if !dataValida([]byte(x)) {
				return nil, fmt.Errorf("data inválida %q", x)
			}
			return dataCmp{s, op, x}, nil
		}
		return txtCmp{s, op, x}, nil
	case float64:
		return numCmp{s, op, x}, nil
	case bool:
		if op != opEq && op != opNe {
			return nil, fmt.Errorf("booleano só aceita = e !=")
		}
		return boolCmp{s, op, x}, nil
	}
	return nil, fmt.Errorf("tipo de valor não suportado")
}

func (m *motor) lista(cs []condJSON, vars map[string]string, ou bool) (cond, error) {
	out := make([]cond, 0, len(cs))
	for _, c := range cs {
		x, err := m.compilar(c, vars)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	if ou {
		return qualquer(out), nil
	}
	return todas(out), nil
}

// classificar devolve o código da primeira regra satisfeita.
func (m *motor) classificar(v []Val) string {
	for i := range m.regras {
		if m.regras[i].c.eval(v) {
			return m.regras[i].codigo
		}
	}
	return m.padrao
}

// todasQueDisparam lista todas as regras satisfeitas (modo --explicar).
func (m *motor) todasQueDisparam(v []Val) []string {
	var out []string
	for i := range m.regras {
		if m.regras[i].c.eval(v) {
			out = append(out, m.regras[i].codigo)
		}
	}
	return out
}

// dataValida: AAAA-MM-DD, dígitos ASCII, ano >= 1, mês/dia válidos (bissexto incluso).
func dataValida(s []byte) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for _, i := range [...]int{0, 1, 2, 3, 5, 6, 8, 9} {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	a := int(s[0]-'0')*1000 + int(s[1]-'0')*100 + int(s[2]-'0')*10 + int(s[3]-'0')
	mes := int(s[5]-'0')*10 + int(s[6]-'0')
	d := int(s[8]-'0')*10 + int(s[9]-'0')
	if a < 1 || mes < 1 || mes > 12 || d < 1 {
		return false
	}
	dias := [...]int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}[mes-1]
	if mes == 2 && (a%4 == 0 && (a%100 != 0 || a%400 == 0)) {
		dias = 29
	}
	return d <= dias
}
