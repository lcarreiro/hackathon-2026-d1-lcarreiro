package motor

// Carga do arquivo de regras (JSON) e compilação de cada condição num
// predicado já tipado (condicoes.go).

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

// condJSON é um combinador (todas, qualquer, nao) ou uma comparação de campo.
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

var operadores = map[string]operador{
	"=": opIgual, "!=": opDiferente, "<": opMenor, "<=": opMenorIgual, ">": opMaior, ">=": opMaiorIgual,
}

var tiposDeComparacao = map[string]tipoComparacao{
	"texto": cmpTexto, "numero": cmpNumero, "data": cmpData, "booleano": cmpBooleano,
}

// ---------- carga ----------

// Carregar lê e compila um arquivo de regras. Um arquivo inválido (operador
// desconhecido, variável inexistente, campo_ref sem tipo...) devolve erro.
func Carregar(caminho string) (*Motor, error) {
	b, err := os.ReadFile(caminho)
	if err != nil {
		return nil, err
	}
	var a arquivoRegras
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, fmt.Errorf("%s: %w", caminho, err)
	}

	m := &Motor{padrao: a.Padrao}
	if m.padrao == "" {
		m.padrao = "SEM_REGRA"
	}
	m.raiz = m.novoCampo()
	m.slotID = m.slot("id")
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
	return m, nil
}

func (m *Motor) novoCampo() *campo {
	c := &campo{slot: -1, indice: m.nCampos}
	m.nCampos++
	return c
}

// slot devolve a posição do campo (caminho com pontos, ex. "financeiro.valor")
// no vetor de valores, registrando-o na árvore se for a primeira vez.
func (m *Motor) slot(caminho string) int {
	c := m.raiz
	for _, nome := range strings.Split(caminho, ".") {
		c = c.filho(nome, m.novoCampo)
	}
	if c.slot < 0 {
		c.slot = m.nSlots
		m.nSlots++
	}
	return c.slot
}

// ---------- compilação ----------

func (m *Motor) compilar(c condJSON, vars map[string]string) (condicao, error) {
	switch {
	case c.Todas != nil:
		lista, err := m.compilarLista(c.Todas, vars)
		return todas(lista), err
	case c.Qualquer != nil:
		lista, err := m.compilarLista(c.Qualquer, vars)
		return qualquer(lista), err
	case c.Nao != nil:
		x, err := m.compilar(*c.Nao, vars)
		return nao{x}, err
	}

	if c.Campo == "" {
		return nil, fmt.Errorf("condição sem campo")
	}
	slot := m.slot(c.Campo)

	switch c.Op {
	case "vazio":
		return vazio{slot}, nil
	case "comeca_com":
		var prefixo string
		if err := json.Unmarshal(c.Valor, &prefixo); err != nil {
			return nil, fmt.Errorf("comeca_com exige valor texto")
		}
		return comecaCom{slot, prefixo}, nil
	case "em":
		return compilarEm(slot, c.Valor)
	}

	op, ok := operadores[c.Op]
	if !ok {
		return nil, fmt.Errorf("operador desconhecido %q", c.Op)
	}

	if c.CampoRef != "" {
		tipo, ok := tiposDeComparacao[c.Tipo]
		if !ok {
			return nil, fmt.Errorf("campo_ref exige tipo (numero, texto, data, booleano)")
		}
		if tipo == cmpBooleano && op != opIgual && op != opDiferente {
			return nil, fmt.Errorf("booleano só aceita = e !=")
		}
		return entreCampos{slot, m.slot(c.CampoRef), op, tipo}, nil
	}

	return compararComLiteral(slot, op, c.Valor, c.Tipo, vars)
}

func (m *Motor) compilarLista(cs []condJSON, vars map[string]string) ([]condicao, error) {
	out := make([]condicao, 0, len(cs))
	for _, c := range cs {
		x, err := m.compilar(c, vars)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, nil
}

// compararComLiteral monta "campo <op> valor". O tipo vem do próprio JSON
// (texto, número, booleano); "tipo": "data" transforma um texto em data.
// Textos que começam com "$" são variáveis do arquivo.
func compararComLiteral(slot int, op operador, raw json.RawMessage, tipoDeclarado string, vars map[string]string) (condicao, error) {
	var lit any
	if err := json.Unmarshal(raw, &lit); err != nil {
		return nil, fmt.Errorf("valor ausente ou inválido")
	}
	if s, ok := lit.(string); ok && strings.HasPrefix(s, "$") {
		v, ok := vars[s[1:]]
		if !ok {
			return nil, fmt.Errorf("variável %s não definida", s)
		}
		lit = v
	}

	switch x := lit.(type) {
	case string:
		if tipoDeclarado == "data" {
			if !dataValida(x) {
				return nil, fmt.Errorf("data inválida %q", x)
			}
			return comparaData{slot, op, x}, nil
		}
		return comparaTexto{slot, op, x}, nil
	case float64:
		return comparaNumero{slot, op, x}, nil
	case bool:
		if op != opIgual && op != opDiferente {
			return nil, fmt.Errorf("booleano só aceita = e !=")
		}
		return comparaBooleano{slot, op == opIgual, x}, nil
	}
	return nil, fmt.Errorf("tipo de valor não suportado")
}

// compilarEm monta "campo em [a, b, ...]": lista só de textos ou só de números.
func compilarEm(slot int, raw json.RawMessage) (condicao, error) {
	var lista []json.RawMessage
	if err := json.Unmarshal(raw, &lista); err != nil || len(lista) == 0 {
		return nil, fmt.Errorf("em exige lista")
	}
	var textos []string
	var numeros []float64
	for _, e := range lista {
		var s string
		var f float64
		switch {
		case json.Unmarshal(e, &s) == nil:
			textos = append(textos, s)
		case json.Unmarshal(e, &f) == nil:
			numeros = append(numeros, f)
		default:
			return nil, fmt.Errorf("valor inválido em lista: %s", e)
		}
	}
	switch {
	case len(textos) > 0 && len(numeros) > 0:
		return nil, fmt.Errorf("lista mistura texto e número")
	case len(numeros) > 0:
		return pertenceNumero{slot, numeros}, nil
	}
	return pertenceTexto{slot, textos}, nil
}
