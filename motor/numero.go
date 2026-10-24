package motor

import (
	"math"
	"strconv"
)

// potencias10 são exatas em float64 até 1e22.
var potencias10 = [...]float64{
	1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9, 1e10, 1e11,
	1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18, 1e19, 1e20, 1e21, 1e22,
}

// lerNumero valida um número na gramática do JSON (sem "+", sem zero à
// esquerda, sem hexa) e o converte para float64 numa passada só.
//
// Caminho rápido: se os dígitos cabem em 2^53 e o expoente decimal está entre
// -22 e 22, mantissa e potência de 10 são exatas em float64 e uma única
// multiplicação ou divisão dá o resultado corretamente arredondado. Os demais
// casos vão para strconv.ParseFloat. Fora da faixa do float64 vira ±Inf, como
// no Python.
func lerNumero(t []byte) (float64, bool) {
	i, n := 0, len(t)
	negativo := i < n && t[i] == '-'
	if negativo {
		i++
	}
	if i >= n {
		return 0, false
	}

	var mantissa uint64
	digitos, expoente := 0, 0

	// parte inteira
	switch {
	case t[i] == '0':
		i++
	case t[i] >= '1' && t[i] <= '9':
		for i < n && t[i] >= '0' && t[i] <= '9' {
			mantissa = mantissa*10 + uint64(t[i]-'0')
			digitos++
			i++
		}
	default:
		return 0, false
	}

	// fração
	if i < n && t[i] == '.' {
		i++
		inicio := i
		for i < n && t[i] >= '0' && t[i] <= '9' {
			mantissa = mantissa*10 + uint64(t[i]-'0')
			digitos++
			expoente--
			i++
		}
		if i == inicio {
			return 0, false
		}
	}

	// expoente
	if i < n && (t[i] == 'e' || t[i] == 'E') {
		i++
		negE := i < n && t[i] == '-'
		if i < n && (t[i] == '+' || t[i] == '-') {
			i++
		}
		inicio, e := i, 0
		for i < n && t[i] >= '0' && t[i] <= '9' {
			if e < 100000 {
				e = e*10 + int(t[i]-'0')
			}
			i++
		}
		if i == inicio {
			return 0, false
		}
		if negE {
			e = -e
		}
		expoente += e
	}

	if i != n {
		return 0, false
	}

	if digitos <= 19 && mantissa <= 1<<53 && expoente >= -22 && expoente <= 22 {
		f := float64(mantissa)
		if expoente < 0 {
			f /= potencias10[-expoente]
		} else {
			f *= potencias10[expoente]
		}
		if negativo {
			f = -f
		}
		return f, true
	}

	f, err := strconv.ParseFloat(b2s(t), 64)
	if err != nil && !math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}
