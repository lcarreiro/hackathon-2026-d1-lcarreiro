# Motor de regras · duplicatas escriturais

Hackathon Ideen 2026 · Desafio 1 · enunciado em [ENUNCIADO.md](ENUNCIADO.md)

O motor lê um arquivo JSON Lines e, para cada linha, devolve a **primeira regra satisfeita**:

```
id:1 - regra: R07
id:2 - regra: SEM_REGRA
```

As regras ficam em [regras/duplicatas.json](regras/duplicatas.json), fora do código. Para mudar, incluir ou reordenar uma regra, basta editar esse arquivo.

## Como rodar

Requer Go 1.24 ou mais recente. Todos os comandos rodam a partir da raiz do repositório.

```bash
go build -o bin/motor ./cmd/motor
bin/motor --entrada dados/pequeno/entrada.jsonl
```

Conferir acertos e tempo com o script do kit:

```bash
python3 medir.py --set pequeno --cmd "bin/motor --entrada {entrada}"
python3 medir.py --set grande  --cmd "bin/motor --entrada {entrada}"
```

Com Docker, como a banca vai rodar:

```bash
docker build -t motor-duplicatas .
python3 medir.py --set grande --cmd "docker run --rm --cpus=1 -v {dir}:/dados motor-duplicatas --entrada /dados/{arquivo}"
```

Testes automatizados:

```bash
go test ./...
```

> **No Windows:** use `python` em vez de `python3`, compile com `go build -o bin/motor.exe ./cmd/motor` e use `\` no caminho do `--cmd` (`bin\motor.exe`), porque o `medir.py` executa o comando pelo `cmd.exe`.

### Opções

| Opção | O que faz |
|---|---|
| `--entrada arquivo.jsonl` | Arquivo de entrada (obrigatório). |
| `--regras arquivo.json` | Outro arquivo de regras. Padrão: `regras/duplicatas.json`. |
| `--explicar` | Mostra, em cada linha, todas as regras que disparam, além da escolhida. |

O mesmo motor funciona com outro domínio, sem mudar código:

```bash
bin/motor --entrada dados/exemplo_pedidos/entrada.jsonl --regras regras/pedidos.json
```

## Como funciona

```
arquivo de regras ──► compilação ──► predicados prontos
                                           │
entrada.jsonl ──► leitor JSON ──► valores da linha ──► 1ª regra verdadeira ──► saída
```

1. **Compilação** ([motor/regras.go](motor/regras.go)): lê o JSON de regras uma vez, transforma cada condição numa função já tipada e anota quais campos as regras usam.
2. **Leitura** ([motor/leitor.go](motor/leitor.go)): percorre cada linha uma única vez e extrai só esses campos. O resto é pulado sem alocar memória.
3. **Classificação** ([motor/condicoes.go](motor/condicoes.go)): avalia as regras em ordem e para na primeira verdadeira.

## Estrutura do repositório

```
cmd/motor/          comando de linha (flags, leitura do arquivo, saída)
motor/              o motor, sem nada específico de duplicatas
  motor.go            laço principal: lê cada linha e classifica
  regras.go           formato do arquivo de regras e compilação
  condicoes.go        operadores (=, <, em, vazio...) e regras de tipo
  leitor.go           leitor JSON seletivo
  numero.go           conversão de números do JSON
  motor_test.go       testes
regras/             duplicatas.json (desafio) e pedidos.json (segundo domínio)
dados/              conjuntos do kit (pequeno, grande) e exemplo de pedidos
testes/             gerador de casos de borda e oráculo independente (Python)
evidencias/         resultados de medição e de testes
medir.py            script de medição do kit
```

## Formato das regras

Cada regra tem um `codigo` e uma condição `se`:

```json
{ "codigo": "R05", "categoria": "Vencida em aberto",
  "se": { "todas": [
    { "campo": "data_vencimento", "op": "<", "tipo": "data", "valor": "$data_referencia" },
    { "campo": "financeiro.valor_pago", "op": "<", "tipo": "numero", "campo_ref": "financeiro.valor" } ] } }
```

| Elemento | Como usar |
|---|---|
| Campo | Caminho com pontos: `financeiro.valor`. |
| Operadores | `=` `!=` `<` `<=` `>` `>=` `em` (lista) `comeca_com` `vazio` |
| Combinar condições | `todas` (E), `qualquer` (OU), `nao` |
| Tipo | Vem do `valor`: texto, número ou booleano. Para datas, use `"tipo": "data"`. |
| Comparar dois campos | `campo_ref` no lugar de `valor`, sempre com `tipo` (`numero`, `texto`, `data`, `booleano`). |
| Variáveis | `"$nome"`, definidas em `variaveis` (ex.: a data de referência). |
| Sem regra | `padrao` define a saída quando nada é satisfeito (`SEM_REGRA`). |

Um arquivo inválido (operador desconhecido, variável inexistente, `campo_ref` sem tipo...) é rejeitado na carga com mensagem de erro.

## Casos de borda

É assim que o motor interpreta a seção "Semântica" do enunciado:

| Situação | Comportamento |
|---|---|
| Campo ausente, nulo ou de outro tipo | Toda comparação é falsa, inclusive `!=`. Vale também quando o objeto pai é `null`, lista ou texto. |
| `vazio` | Verdadeiro para ausente, `null` ou `""`. |
| Texto | Igualdade exata, diferencia maiúsculas. Escapes `\uXXXX` são decodificados. |
| Booleano | Só `true`/`false` do JSON. `"false"`, `0` e `1` não contam. |
| Data | Só `AAAA-MM-DD` com data real (bissexto conferido). O resto conta como ausente. |
| `comeca_com` | Só para texto. O CFOP numérico `5102` não é texto. |
| Limites | `>=` e `<=` incluem o limite. `5e5`, `500000` e `500000.0` são o mesmo número. |
| Chave repetida | Vale a última, como no `json` do Python. Um objeto pai repetido descarta os filhos anteriores. |
| Linha que o leitor rápido não aceita | É lida de novo com `encoding/json`. Se for JSON inválido, é avisada no stderr e ignorada. |

## Testes e evidências

| Conjunto | Acertos | Mediana nativa (1 CPU) | Mediana Docker `--cpus=1` |
|---|---|---|---|
| Pequeno do kit (100) | 100/100 | 0,003 s | 0,30 s |
| Sintético (200 mil, 161 MB) | 200 000/200 000 | 0,28 s | 0,63 s |
| Borda (300 mil, 70% com mutações) | 300 000/300 000 | 0,44 s | não medido |
| Grande do kit (200 mil)¹ | 200 000/200 000 | 0,74 s | 4,52 s (antes 12,06 s) |

Os detalhes de cada medição estão em [evidencias/](evidencias/).

- No Docker, cerca de 0,27 s vêm só da inicialização do contêiner. A imagem final tem 3 MB (`FROM scratch`).
- ¹ Medido em outra máquina (Windows 11 + Docker Desktop), em 24/10. Nesse ambiente, o `-v` expõe a pasta do Windows à VM Linux por um compartilhamento 9p. Com `mmap`, a leitura buscava o arquivo página por página e levava cerca de 10 s. Com `os.ReadFile`, que lê em blocos grandes, o Docker caiu de 12,06 s para 4,52 s. O que sobra é a leitura pelo compartilhamento (~1,5 s) e a subida do contêiner (~1,2 s). O nativo variou entre 0,51 s e 0,84 s em medições do mesmo binário, por carga da máquina.
- Os tempos da tabela foram medidos antes da reorganização do código de 23/10. A versão atual gera saída idêntica byte a byte nesses conjuntos, e o processamento ficou cerca de 15% mais rápido no benchmark (`go test -bench`).

**Como a correção foi verificada:** [testes/oraculo.py](testes/oraculo.py) traz as 12 regras escritas à mão em Python, direto do enunciado, sem ler o arquivo de regras. [testes/gerar.py](testes/gerar.py) gera casos com campo ausente, nulo, tipo trocado, valor no limite, data inválida, caixa diferente, escape unicode, chave duplicada e espaços/CRLF. Motor e oráculo concordam em 100% das linhas geradas.

```bash
python3 testes/gerar.py --n 300000 --borda 0.7 --seed 7 --saida dados/borda
```

**Limitação:** o oráculo é a minha leitura do enunciado. Se eu interpretei errado uma ambiguidade, motor e oráculo erram juntos. As interpretações estão na tabela de casos de borda.

## Desempenho

- **Leitor JSON próprio.** Não monta mapa nem usa reflexão: extrai só os campos das regras e descarta as outras chaves pelo tamanho do nome.
- **Busca de 8 em 8 bytes (SWAR)** para achar `"`, `\` e fim de linha dentro de textos.
- **Números convertidos numa passada só.** Valores comuns (até 15 dígitos, como `39089.35`) usam um caminho exato e rápido. Os demais vão para `strconv.ParseFloat`.
- **Entrada lida de uma vez (`os.ReadFile`), GC desligado e saída em blocos de 1 MB.** O `mmap` foi descartado porque fica muito lento quando o arquivo chega por volume do Docker Desktop.
- **Uma thread só.** A banca roda com 1 CPU, então paralelismo só acrescentaria custo.

## Uso de IA

O motor foi escrito com Claude. Erros encontrados durante o trabalho:

1. **Biblioteca errada.** A IA sugeriu `buger/jsonparser`/`fastjson`, que devolvem a **primeira** ocorrência de uma chave repetida. O `json` do Python usa a **última**. A solução foi trocar por um leitor próprio, com teste dedicado.
2. **Centavos/decimal desnecessários.** A IA sugeriu comparar dinheiro em centavos, o que divergiria do gabarito (que compara floats). O motor manteve `float64` com conversão exata.
3. **Precedência no gerador de testes.** Em `t == 4 and A or B` faltavam parênteses. Foi pego na revisão, antes de usar a massa.
4. **Dockerfile não testado de início.** O ambiente bloqueava o Docker Hub. Depois, o build completo rodou no Docker Desktop e acertou 200 000/200 000 no set grande.
5. **Refatoração que deixou o motor mais lento.** Na reorganização do código, a IA unificou as comparações numa função genérica e removeu um atalho inline do leitor. O perfil mostrou o processamento cerca de 20% mais lento. As duas mudanças foram revertidas, e a versão final foi conferida contra a anterior por benchmark lado a lado e por saída idêntica em 700 mil linhas.

Os erros listados acima foram identificados e corrigidos pela própria IA. A minha atuação foi direcionar o trabalho: pedi a reorganização das pastas e dos arquivos e uma otimização para reduzir o tempo de processamento. Rodei o medir.py nos dois modos (nativo e Docker) e conferi os acertos no set grande. Também questionei a diferença de tempo entre eles (0,5 s contra 9,6 s), o que levou à troca do mmap por os.ReadFile e reduziu o tempo no Docker para 4,5 s.
