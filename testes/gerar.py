#!/usr/bin/env python3
"""Gera massa fictícia de duplicatas (+ gabarito pelo oráculo).

  python testes/gerar.py --n 200000 --saida dados/sintetico          # volume
  python testes/gerar.py --n 50000 --borda 0.6 --saida dados/borda   # casos de borda

--borda = fração de linhas que recebem 1 a 4 mutações de borda (campo ausente,
nulo, tipo trocado, valor no limite, data inválida, caixa, escape unicode,
chave duplicada, espaços...).
"""
import argparse, json, os, random, sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from oraculo import classificar  # noqa: E402

UFS = ["SP", "RJ", "MG", "PR", "SC", "RS", "BA", "PE", "GO", "DF"]
R = random.Random()


def dig(n): return "".join(R.choice("0123456789") for _ in range(n))


def data(a, b):
    import datetime
    d0 = datetime.date(2026, 1, 1) + datetime.timedelta(days=R.randint(a, b))
    return d0.isoformat()


def base(i):
    valor = round(R.choice([R.uniform(100, 5000), R.uniform(5000, 99999), R.uniform(90000, 250000),
                            R.uniform(200000, 800000)]), 2)
    pago = R.choice([0.0, valor, round(valor * R.random(), 2)])
    uo = R.choice(UFS)
    ud = uo if R.random() < 0.8 else R.choice(UFS)
    ent = R.choice(["ENTREGUE"] * 6 + ["EM_TRANSITO"] * 2 + ["DEVOLVIDA", "AVARIADA"])
    man = R.choice([None] * 5 + ["ACTO"] * 4 + ["RCAV", "RCQQ", "RCVN", "RCVL", "RCOP", "ANTC"])
    return {
        "id": i, "iud": dig(20), "numero": f"{dig(6)}/{R.randint(1, 6)}",
        "status": R.choice(["ATIVA"] * 8 + ["LIQUIDADA", "CANCELADA"]),
        "data_emissao": data(150, 290), "data_vencimento": data(200, 420),
        "sacador": {"cnpj": dig(14), "uf": uo, "porte": R.choice(["ME", "EPP", "MEDIA", "GRANDE"])},
        "sacado": {"cnpj": dig(14), "uf": ud,
                   "segmento": R.choice(["VAREJO", "INDUSTRIA", "AGRO", "SERVICOS", "SAUDE", "ATACADO"])},
        "financeiro": {"valor": valor, "valor_pago": pago, "moeda": "BRL", "parcela": 1,
                       "total_parcelas": R.randint(1, 6),
                       "tipo_negociacao": R.choice([None, "CSCO", "CSAO", "CSFD", "PENR", "PRCS"]),
                       "taxa_juros_mensal": round(R.uniform(0.5, 4), 2)},
        "pagamento": {"instrumento": R.choice(["TRCR", "PPIX", "BOLETO"]), "compe": R.choice(["001", "237", "341"]),
                      "tipo_conta": R.choice(["CCOR", "CPAG"])},
        "fiscal": {"chave_nfe": dig(44), "cfop": R.choice(["5102", "6102", "5405", "6108"])},
        "logistica": {"modal": R.choice(["RODOVIARIO"] * 5 + ["AEREO", "FERROVIARIO", "AQUAVIARIO"]),
                      "transportadora_cnpj": dig(14), "uf_origem": uo, "uf_destino": ud,
                      "peso_kg": round(R.choice([R.uniform(1, 600), R.uniform(100, 30000)]), 1),
                      "status_entrega": ent,
                      "data_entrega": data(200, 290) if ent == "ENTREGUE" else None,
                      "migo": R.random() < 0.7},
        "manifestacao": {"tipo": man, "data": data(200, 290) if man else None},
    }


CAMPOS = ["status", "data_vencimento", "sacador.porte", "sacado.segmento", "financeiro.valor",
          "financeiro.valor_pago", "financeiro.tipo_negociacao", "pagamento.instrumento", "fiscal.cfop",
          "logistica.modal", "logistica.uf_origem", "logistica.uf_destino", "logistica.peso_kg",
          "logistica.status_entrega", "logistica.migo", "manifestacao.tipo"]
PAIS = ["financeiro", "logistica", "manifestacao", "sacador", "sacado", "fiscal", "pagamento"]
LIMITES = [100000, 100000.0, 99999.99, 100000.01, 1e5, 500000, 499999.99, 500000.0, 200000, 199999.99,
           5000, 5000.0, 5000.01, 4999.99, 500, 500.0, 500.0001, 0, -1, 1e308, 10 ** 30]
DATAS = ["2026-10-23", "2026-10-22", "2026-10-24", "2026-02-29", "2024-02-29", "2026-02-28", "2026-13-01",
         "2026-00-10", "2026-04-31", "2026-1-05", "2026/10/01", "20261001", "", " 2026-10-01", "2026-10-01T00:00",
         "0000-01-01", "1900-02-29", "2000-02-29", "26-10-01"]
TEXTOS = {"status": ["ativa", "Cancelada", "CANCELADA ", "LIQUIDADA", "CANCELADA"],
          "manifestacao.tipo": ["", "acto", "ACTO", "RCOP", "RCAV", " RCAV", "ANTC"],
          "logistica.status_entrega": ["entregue", "ENTREGUE", "AVARIADA", "DEVOLVIDA", ""],
          "fiscal.cfop": ["5", "", " 5102", "5102", "6102", "05102", "5.102"],
          "sacado.segmento": ["agro", "AGRO", "AGRO "], "logistica.modal": ["aereo", "AEREO"],
          "sacador.porte": ["me", "ME", "EPP", "epp"], "pagamento.instrumento": ["PIX", "PPIX", "ppix"],
          "financeiro.tipo_negociacao": ["csfd", "CSFD", "PENR", ""]}


def setp(d, caminho, v, apagar=False):
    ks = caminho.split(".")
    for k in ks[:-1]:
        if not isinstance(d.get(k), dict):
            d[k] = {}
        d = d[k]
    if apagar:
        d.pop(ks[-1], None)
    else:
        d[ks[-1]] = v


def getp(d, caminho):
    for k in caminho.split("."):
        if not isinstance(d, dict) or k not in d:
            return None
        d = d[k]
    return d


def mutar(d):
    c = R.choice(CAMPOS)
    x = getp(d, c)
    t = R.randrange(12)
    if t == 0: setp(d, c, None, apagar=True)
    elif t == 1: setp(d, c, None)
    elif t == 2: setp(d, c, str(x) if x is not None else "x")                 # número/bool -> texto
    elif t == 3: setp(d, c, R.choice([True, False, 0, 1, "true", "false", [], {}, 5102, 1.5]))
    elif t == 4 and (c.startswith("financeiro.valor") or c == "logistica.peso_kg"): setp(d, c, R.choice(LIMITES))
    elif t == 5 and c == "data_vencimento": setp(d, c, R.choice(DATAS))
    elif t == 6 and c in TEXTOS: setp(d, c, R.choice(TEXTOS[c]))
    elif t == 7: d[R.choice(PAIS)] = R.choice([None, [], "x", {}, 1])     # pai inválido
    elif t == 8: setp(d, "logistica.uf_destino", getp(d, "logistica.uf_origem"))
    elif t == 9 and c == "logistica.migo": setp(d, c, R.choice(["false", "true", 0, 1, None]))
    else: setp(d, R.choice(["financeiro.valor", "financeiro.valor_pago"]), R.choice(LIMITES))


def serializar(d):
    s = json.dumps(d, ensure_ascii=False, separators=R.choice([(",", ":"), (", ", ": ")]))
    r = R.random()
    if r < 0.05:  # escape unicode em textos
        s = s.replace('"ATIVA"', '"\\u0041TIVA"').replace('"ACTO"', '"AC\\u0054O"').replace('"5102"', '"\\u00351\\u00302"')
    elif r < 0.08:  # chave duplicada: a última vale
        dup = R.choice(['"status":"CANCELADA",', '"status":"ATIVA",', '"financeiro":{"valor":900000},'])
        s = "{" + dup + s[1:]
    elif r < 0.10:
        s = "  " + s.replace(",", " ,\t") + " \r"
    return s


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--n", type=int, default=200000)
    ap.add_argument("--borda", type=float, default=0.0)
    ap.add_argument("--seed", type=int, default=2026)
    ap.add_argument("--saida", required=True)
    a = ap.parse_args()
    R.seed(a.seed)
    os.makedirs(a.saida, exist_ok=True)
    with open(os.path.join(a.saida, "entrada.jsonl"), "w", encoding="utf-8") as fe, \
            open(os.path.join(a.saida, "gabarito.txt"), "w", encoding="utf-8") as fg:
        for i in range(1, a.n + 1):
            d = base(i)
            if R.random() < a.borda:
                for _ in range(R.randint(1, 4)):
                    mutar(d)
            linha = serializar(d) if a.borda else json.dumps(d, separators=(",", ":"))
            fe.write(linha + "\n")
            fg.write(f"id:{i} - regra: {classificar(json.loads(linha))}\n")


if __name__ == "__main__":
    main()
