package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func carregar(t testing.TB) *motor {
	m, err := carregarRegras("regras/duplicatas.json")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func rodar(m *motor, entrada string) string {
	var out bytes.Buffer
	m.processar([]byte(entrada), &out, false)
	return out.String()
}

// Casos da seção "Semântica" do ENUNCIADO, um por linha.
func TestSemantica(t *testing.T) {
	m := carregar(t)
	casos := []struct{ nome, json, regra string }{
		{"texto exato", `{"id":1,"status":"CANCELADA"}`, "R01"},
		{"caixa diferente", `{"id":1,"status":"cancelada"}`, "SEM_REGRA"},
		{"escape unicode", `{"id":1,"status":"CANCELADA"}`, "R01"},
		{"chave repetida vale a última", `{"id":1,"status":"CANCELADA","status":"ATIVA"}`, "SEM_REGRA"},
		{"pai repetido zera filhos", `{"id":1,"financeiro":{"valor":900000},"financeiro":{"moeda":"BRL"}}`, "SEM_REGRA"},
		{"valor texto não é número", `{"id":1,"financeiro":{"valor":"900000"}}`, "SEM_REGRA"},
		{"limite >= inclui", `{"id":1,"financeiro":{"valor":500000}}`, "R07"},
		{"limite >= exponencial", `{"id":1,"financeiro":{"valor":5e5}}`, "R07"},
		{"abaixo do limite", `{"id":1,"financeiro":{"valor":499999.99}}`, "SEM_REGRA"},
		{"> exclui o limite", `{"id":1,"logistica":{"modal":"AEREO","peso_kg":500}}`, "SEM_REGRA"},
		{"> acima do limite", `{"id":1,"logistica":{"modal":"AEREO","peso_kg":500.01}}`, "R09"},
		{"vazio: ausente", `{"id":1,"logistica":{"status_entrega":"AVARIADA"}}`, "R04"},
		{"vazio: nulo", `{"id":1,"logistica":{"status_entrega":"AVARIADA"},"manifestacao":{"tipo":null}}`, "R04"},
		{"vazio: texto vazio", `{"id":1,"logistica":{"status_entrega":"AVARIADA"},"manifestacao":{"tipo":""}}`, "R04"},
		{"vazio: pai nulo", `{"id":1,"logistica":{"status_entrega":"AVARIADA"},"manifestacao":null}`, "R04"},
		{"não vazio: número", `{"id":1,"logistica":{"status_entrega":"AVARIADA"},"manifestacao":{"tipo":0}}`, "SEM_REGRA"},
		{"booleano falso", `{"id":1,"logistica":{"status_entrega":"ENTREGUE","migo":false}}`, "R10"},
		{"booleano em texto", `{"id":1,"logistica":{"status_entrega":"ENTREGUE","migo":"false"}}`, "SEM_REGRA"},
		{"booleano como 0", `{"id":1,"logistica":{"status_entrega":"ENTREGUE","migo":0}}`, "SEM_REGRA"},
		{"data vencida", `{"id":1,"data_vencimento":"2026-10-22","financeiro":{"valor":10,"valor_pago":0}}`, "R05"},
		{"data igual à referência", `{"id":1,"data_vencimento":"2026-10-23","financeiro":{"valor":10,"valor_pago":0}}`, "SEM_REGRA"},
		{"data inexistente", `{"id":1,"data_vencimento":"2026-02-29","financeiro":{"valor":10,"valor_pago":0}}`, "SEM_REGRA"},
		{"bissexto válido", `{"id":1,"data_vencimento":"2024-02-29","financeiro":{"valor":10,"valor_pago":0}}`, "R05"},
		{"data fora do formato", `{"id":1,"data_vencimento":"2026-1-05","financeiro":{"valor":10,"valor_pago":0}}`, "SEM_REGRA"},
		{"valor_pago ausente", `{"id":1,"data_vencimento":"2026-01-05","financeiro":{"valor":10}}`, "SEM_REGRA"},
		{"cfop texto", `{"id":1,"logistica":{"uf_origem":"SP","uf_destino":"RJ"},"fiscal":{"cfop":"5102"}}`, "R08"},
		{"cfop numérico", `{"id":1,"logistica":{"uf_origem":"SP","uf_destino":"RJ"},"fiscal":{"cfop":5102}}`, "SEM_REGRA"},
		{"!= com lado ausente é falso", `{"id":1,"logistica":{"uf_origem":"SP"},"fiscal":{"cfop":"5102"}}`, "SEM_REGRA"},
		{"conflito: ordem decide", `{"id":1,"status":"LIQUIDADA","logistica":{"status_entrega":"DEVOLVIDA"}}`, "R01"},
		{"espaços e CRLF", "  { \"id\" : 1 , \"status\" : \"CANCELADA\" }  \r", "R01"},
	}
	for _, c := range casos {
		got := strings.TrimSpace(rodar(m, c.json+"\n"))
		want := "id:1 - regra: " + c.regra
		if got != want {
			t.Errorf("%s: %s\n  obtido: %s\n  esperado: %s", c.nome, c.json, got, want)
		}
	}
}

func TestSetPequeno(t *testing.T) {
	m := carregar(t)
	in, _ := os.ReadFile("dados/pequeno/entrada.jsonl")
	gab, _ := os.ReadFile("dados/pequeno/gabarito.txt")
	if got := rodar(m, string(in)); got != string(gab) {
		t.Fatal("saída difere do gabarito do set pequeno")
	}
}

func TestRegrasInvalidas(t *testing.T) {
	ruins := []string{
		`{"regras":[{"codigo":"X","se":{"campo":"a","op":"~","valor":1}}]}`,
		`{"regras":[{"codigo":"X","se":{"campo":"a","op":"<","campo_ref":"b"}}]}`,
		`{"regras":[{"codigo":"X","se":{"campo":"a","op":"<","valor":"$nao_existe"}}]}`,
		`{"regras":[{"se":{"campo":"a","op":"=","valor":1}}]}`,
	}
	for _, r := range ruins {
		f, _ := os.CreateTemp(t.TempDir(), "*.json")
		f.WriteString(r)
		f.Close()
		if _, err := carregarRegras(f.Name()); err == nil {
			t.Errorf("deveria rejeitar: %s", r)
		}
	}
}

func BenchmarkSintetico(b *testing.B) {
	m := carregar(b)
	d, err := os.ReadFile("dados/sintetico/entrada.jsonl")
	if err != nil {
		b.Skip("gere com: python3 testes/gerar.py --n 200000 --saida dados/sintetico")
	}
	b.SetBytes(int64(len(d)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.processar(d, io.Discard, false)
	}
}
