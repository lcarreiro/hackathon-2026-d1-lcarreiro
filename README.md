# Motor de regras · duplicatas escriturais (Hackathon Ideen 2026 · Desafio 1)

## O problema

Classificar cada duplicata de um arquivo JSON Lines pela primeira regra satisfeita, com as regras fora do código, acertando os casos de borda e no menor tempo total de comando.

## O que eu fiz

Um binário Go sem dependências externas. Ele tem três peças:

| Peça | Arquivo | O que faz |
|---|---|---|
| Regras | `regras/duplicatas.json` | As 12 regras do enunciado numa árvore declarativa (`todas`, `qualquer`, `nao`, comparação). |
| Compilador | `regras.go` | Lê o JSON uma vez e transforma cada condição num predicado Go já tipado. Também descobre quais campos as regras usam. |
| Leitor | `scanner.go` | Percorre cada linha uma única vez. Extrai só os campos usados e pula o resto sem alocar memória. |

Fluxo: `mmap` do arquivo → para cada linha, o leitor preenche um vetor de ~20 valores → as regras são avaliadas em ordem e a primeira verdadeira vence → a linha vai para um buffer de saída de 1 MB.

### Formato das regras

```json
{ "codigo": "R05", "se": { "todas": [
  { "campo": "data_vencimento", "op": "<", "tipo": "data", "valor": "$data_referencia" },
  { "campo": "financeiro.valor_pago", "op": "<", "tipo": "numero", "campo_ref": "financeiro.valor" } ] } }
```

| Elemento | Opções |
|---|---|
| Operadores | `=` `!=` `<` `<=` `>` `>=` `em` `comeca_com` `vazio` |
| Combinadores | `todas` (E), `qualquer` (OU), `nao` |
| Tipo da comparação | Vem do literal: texto, número ou booleano. Use `"tipo": "data"` para datas. Comparação entre dois campos (`campo_ref`) exige `tipo`. |
| Variáveis | `"$nome"`, definidas em `variaveis` (ex.: a data de referência). |
| Resultado padrão | O campo `padrao` define o que sai quando nenhuma regra é satisfeita (`SEM_REGRA`). |

Um arquivo de regras inválido (operador desconhecido, variável inexistente, `campo_ref` sem tipo) é rejeitado na carga, com mensagem de erro.

### Semântica (seção 1–8 do enunciado)

| Caso | Comportamento |
|---|---|
| Campo ausente, nulo ou de outro tipo | Toda comparação é falsa, inclusive `!=`. Isso vale também quando o objeto pai é `null`, uma lista ou um texto. |
| `vazio` | Verdadeiro para ausente, `null` ou `""`. |
| Texto | Igualdade exata, diferenciando maiúsculas. Escapes `\uXXXX` são decodificados (`"ATIVA"` = `"ATIVA"`). |
| Booleano | Só `true`/`false` do JSON. `"false"`, `0` e `1` não contam. |
| Data | Só `AAAA-MM-DD` com dígitos ASCII e data real (bissexto conferido). O resto é tratado como ausente. |
| `comeca_com` | Só se aplica a texto. O CFOP numérico `5102` não é texto. |
| Limites | `>=` e `<=` incluem o valor-limite. `5e5` e `500000.0` são o mesmo número. |
| Chave repetida no JSON | Vale a última, como no `json` do Python. Um objeto pai repetido descarta os filhos que vieram antes. |
| Linha que o leitor rápido rejeita | Passa pelo `encoding/json`. Se mesmo assim for JSON inválido, é avisada no stderr e ignorada. |

## Estrutura do repositório

Todos os comandos abaixo rodam a partir da raiz do repositório.

```
.
├── main.go, regras.go, scanner.go    # motor (CLI, compilador de regras, leitor JSON)
├── arquivo_unix.go, arquivo_outros.go # leitura da entrada: mmap no Unix, leitura comum nos outros sistemas
├── motor_test.go                     # testes e benchmarks
├── regras/                           # duplicatas.json (desafio) e pedidos.json (segundo domínio)
├── dados/                            # pequeno/, grande/ (kit) e exemplo_pedidos/
├── testes/                           # gerar.py (massa de borda) e oraculo.py (gabarito independente)
├── evidencias/                       # saídas do medir.py e dos testes
├── bin/                              # binário compilado (gerado pelo go build)
├── medir.py                          # script de medição do kit
├── Dockerfile, .dockerignore
└── ENUNCIADO.md
```

## Como reproduzir

Com Go 1.22 ou mais recente:

```bash
go build -o bin/motor .
bin/motor --entrada dados/pequeno/entrada.jsonl > saida.txt
python3 medir.py --set pequeno --cmd "bin/motor --entrada {entrada}"
python3 medir.py --set grande  --cmd "bin/motor --entrada {entrada}"   # com dados/grande do kit
go test -v .
```

No Windows, os comandos mudam em dois pontos:

- Use `python` (ou `py`). Lá, `python3` costuma abrir o atalho da Microsoft Store, que não executa nada.
- Compile com extensão e use `\`, porque o `medir.py` executa o comando pelo `cmd.exe`:

```powershell
go build -o bin/motor.exe .
python medir.py --set grande --cmd "bin\motor.exe --entrada {entrada}"
```

Com Docker, do mesmo jeito que a banca vai rodar:

```bash
docker build -t motor-duplicatas .
python3 medir.py --set grande --cmd "docker run --rm --cpus=1 -v {dir}:/dados motor-duplicatas --entrada /dados/{arquivo}"
```

Outras opções:

- `--regras outro.json` troca o arquivo de regras (padrão: `regras/duplicatas.json` ao lado do binário).
- `--explicar` anexa todas as regras que disparam em cada linha.

Segundo domínio, sem mudar código:

```bash
bin/motor --entrada dados/exemplo_pedidos/entrada.jsonl --regras regras/pedidos.json
```

Massa extra para teste (o gabarito sai do oráculo):

```bash
python3 testes/gerar.py --n 200000 --saida dados/sintetico                  # volume
python3 testes/gerar.py --n 300000 --borda 0.7 --seed 7 --saida dados/borda  # casos de borda
```

## Evidências (pasta `evidencias/`)

| Conjunto | Acertos | Mediana nativa (1 CPU) | Mediana Docker `--cpus=1` |
|---|---|---|---|
| Pequeno do kit (100) | 100/100 | 0,003 s | 0,30 s |
| Sintético (200 mil, 161 MB) | 200 000/200 000 | 0,28 s | 0,63 s |
| Borda (300 mil, 70% com mutações) | 300 000/300 000 | 0,44 s | não medido |
| Grande do kit (200 mil)¹ | 200 000/200 000 | 0,57 s | 12,06 s |

- Só o processamento, sem a inicialização, roda a cerca de 680 MB/s em um núcleo (`go test -bench`).
- No Docker, uns 0,27 s vêm da própria inicialização do contêiner e não dependem do motor. A imagem final tem 3 MB (`FROM scratch`).
- ¹ Medido em outra máquina (Windows 11 com Docker Desktop), então não compare diretamente com as linhas de cima. O tempo no Docker vem quase todo do volume `-v`: ele monta uma pasta do Windows dentro da VM Linux, e ler o arquivo por essa montagem é lento. Detalhes em `evidencias/grande.txt`.

### Como a correção foi verificada

`testes/oraculo.py` traz as 12 regras escritas à mão em Python, direto do enunciado, sem ler `regras/duplicatas.json`. Ele reproduz 100/100 do gabarito do set pequeno.

`testes/gerar.py` gera casos com campo ausente, nulo, tipo trocado, valores no limite, datas inválidas, caixa diferente, escape unicode, chave duplicada, objeto pai inválido e espaços/CRLF. O motor e o oráculo concordam em 100% de 400 mil linhas desse tipo (duas sementes).

Limite honesto: o oráculo é a minha leitura do enunciado. Se eu interpretei errado uma ambiguidade, os dois erram juntos. As interpretações estão na tabela de semântica acima.

### Decisões de desempenho

- **Leitor JSON próprio e seletivo.** Ele não monta mapa nem usa reflexão. Os nomes de chave são descartados pelo tamanho antes de qualquer comparação de texto.
- **Varredura de textos 8 bytes por vez (SWAR)** para achar `"`, `\` e quebra de linha. Funciona em amd64 e arm64.
- **`mmap` na entrada, GC desligado e saída em blocos de 1 MB.**
- **Uma thread só.** A banca roda com 1 CPU, então paralelismo só acrescentaria custo.
- **Números com `strconv.ParseFloat`,** que tem arredondamento correto e evita divergência em `valor_pago < valor`.

Perfil antes/depois: 0,32 s → 0,25 s por execução nativa. O ganho veio de trocar o laço byte a byte por SWAR e da busca de chaves por tamanho.

## Onde a IA errou (e como foi corrigido)

O motor foi escrito com Claude. Erros encontrados durante o trabalho:

1. **Biblioteca errada na primeira recomendação.** A IA sugeriu `buger/jsonparser`/`fastjson` para extrair campos. Essas bibliotecas devolvem a **primeira** ocorrência de uma chave repetida, e o `json` do Python (de onde provavelmente sai o gabarito) usa a **última**. A biblioteca foi trocada por um leitor próprio que zera os filhos quando um objeto pai se repete, com teste dedicado.
2. **Sugestão desnecessária de centavos/decimal.** A IA sugeriu comparar dinheiro em centavos. Na prática isso criaria divergência com o gabarito, que compara floats. Foi mantido `float64` com parse correto.
3. **Bug de precedência no gerador de testes.** Em `t == 4 and A or B` faltavam parênteses, e mutações de limite caíam em campos errados. Apareceu na revisão do código, antes de usar a massa.
4. **Dockerfile multi-stage não testado ponta a ponta aqui.** O ambiente bloqueava o Docker Hub, então de início foi validada só a imagem final (`FROM scratch` + binário), com 100% no set pequeno e no sintético. Depois o `docker build` completo rodou no Docker Desktop (Windows) e acertou 200 000/200 000 no set grande do kit.

Revisão humana: <descreva aqui o que você conferiu e mudou>.
