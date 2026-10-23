#!/usr/bin/env python3
"""Oráculo: as 12 regras escritas à mão, direto do ENUNCIADO, em Python puro.

É deliberadamente independente do motor (não lê regras/duplicatas.json) para
pegar erro tanto de tradução das regras quanto do motor. Lento e simples.

  python testes/oraculo.py entrada.jsonl > gabarito.txt
"""
import datetime, json, re, sys

REF = datetime.date(2026, 10, 23)
AUS = object()  # ausente
DATA = re.compile(r"[0-9]{4}-[0-9]{2}-[0-9]{2}")


def get(d, caminho):
    for k in caminho.split("."):
        if not isinstance(d, dict) or k not in d:
            return AUS
        d = d[k]
    return d


def num(x): return isinstance(x, (int, float)) and not isinstance(x, bool)
def txt(x): return isinstance(x, str)
def boo(x): return isinstance(x, bool)
def vazio(x): return x is AUS or x is None or x == ""


def data(x):
    if not txt(x) or not DATA.fullmatch(x):
        return None
    try:
        return datetime.date(int(x[:4]), int(x[5:7]), int(x[8:]))
    except ValueError:
        return None


def eq(x, v):  # igualdade tipada: texto com texto, bool com bool
    return txt(x) and x == v


def classificar(d):
    g = lambda c: get(d, c)
    st, ent, man = g("status"), g("logistica.status_entrega"), g("manifestacao.tipo")
    valor, pago = g("financeiro.valor"), g("financeiro.valor_pago")
    migo = g("logistica.migo")

    if txt(st) and st in ("CANCELADA", "LIQUIDADA"): return "R01"
    if eq(ent, "DEVOLVIDA") or eq(man, "RCOP"): return "R02"
    if txt(man) and man in ("RCAV", "RCQQ", "RCVN", "RCVL"): return "R03"
    if eq(ent, "AVARIADA") and vazio(man): return "R04"
    venc = data(g("data_vencimento"))
    if venc is not None and venc < REF and num(pago) and num(valor) and pago < valor: return "R05"
    tn = g("financeiro.tipo_negociacao")
    if txt(tn) and tn in ("CSFD", "PENR") and num(valor) and valor >= 100000: return "R06"
    if num(valor) and (valor >= 500000 or (eq(g("sacado.segmento"), "AGRO") and valor >= 200000)): return "R07"
    o, de, cfop = g("logistica.uf_origem"), g("logistica.uf_destino"), g("fiscal.cfop")
    if txt(o) and txt(de) and o != de and txt(cfop) and cfop.startswith("5"): return "R08"
    peso = g("logistica.peso_kg")
    if eq(g("logistica.modal"), "AEREO") and num(peso) and peso > 500: return "R09"
    if eq(ent, "ENTREGUE") and boo(migo) and migo is False: return "R10"
    if eq(man, "ACTO") and boo(migo) and migo is True: return "R11"
    porte = g("sacador.porte")
    if eq(g("pagamento.instrumento"), "PPIX") and txt(porte) and porte in ("ME", "EPP") \
            and num(valor) and valor <= 5000: return "R12"
    return "SEM_REGRA"


def main():
    out = []
    with open(sys.argv[1], encoding="utf-8") as f:
        for linha in f:
            if not linha.strip():
                continue
            d = json.loads(linha)
            out.append(f"id:{d['id']} - regra: {classificar(d)}\n")
    sys.stdout.write("".join(out))


if __name__ == "__main__":
    main()
