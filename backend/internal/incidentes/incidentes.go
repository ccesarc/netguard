// Package incidentes transforma a sequência de ciclos em quedas (início, fim,
// causa). Só abre depois de N ciclos seguidos "fora" e só fecha depois de M
// ciclos bons, para um pacote perdido não virar queda.
package incidentes

import (
	"slices"
	"time"

	"netguard/internal/diagnostico"
)

type Incidente struct {
	ID      int64               `json:"id"`
	Inicio  time.Time           `json:"inicio"`
	Fim     *time.Time          `json:"fim"`
	Causa   diagnostico.Causa   `json:"causa"`
	Causas  []diagnostico.Causa `json:"causas"`
	Detalhe string              `json:"detalhe,omitempty"`
}

type Tipo int

const (
	Abriu Tipo = iota
	MudouCausa
	Fechou
)

type Mudanca struct {
	Tipo      Tipo
	Incidente Incidente
}

type Rastreador struct {
	ParaAbrir  int
	ParaFechar int

	aberto *Incidente

	falhas      int
	inicioFalha time.Time
	causasFalha []diagnostico.Causa

	oks        int
	primeiroOK time.Time
}

func NovoRastreador(paraAbrir, paraFechar int) *Rastreador {
	return &Rastreador{ParaAbrir: paraAbrir, ParaFechar: paraFechar}
}

// Aberto devolve uma cópia da queda em andamento, ou nil.
func (r *Rastreador) Aberto() *Incidente {
	if r.aberto == nil {
		return nil
	}
	c := *r.aberto
	c.Causas = slices.Clone(r.aberto.Causas)
	return &c
}

// DefinirID guarda o id que o banco deu à queda aberta.
func (r *Rastreador) DefinirID(id int64) {
	if r.aberto != nil {
		r.aberto.ID = id
	}
}

// Observar recebe o resultado de cada ciclo, em ordem, e diz o que mudou.
func (r *Rastreador) Observar(em time.Time, res diagnostico.Resultado) []Mudanca {
	caiu := res.Estado == diagnostico.Fora

	if r.aberto == nil {
		if !caiu {
			r.falhas, r.causasFalha = 0, nil
			return nil
		}
		if r.falhas == 0 {
			r.inicioFalha = em
		}
		r.falhas++
		r.causasFalha = juntar(r.causasFalha, res.Causa)
		if r.falhas < r.ParaAbrir {
			return nil
		}
		r.aberto = &Incidente{
			Inicio:  r.inicioFalha, // a queda começou no primeiro ciclo ruim, não no terceiro
			Causa:   res.Causa,
			Causas:  r.causasFalha,
			Detalhe: res.Detalhe,
		}
		r.falhas, r.causasFalha, r.oks = 0, nil, 0
		return []Mudanca{{Abriu, *r.Aberto()}}
	}

	if caiu {
		r.oks = 0
		if res.Causa == r.aberto.Causa && res.Detalhe == r.aberto.Detalhe {
			return nil
		}
		r.aberto.Causa = res.Causa
		r.aberto.Causas = juntar(r.aberto.Causas, res.Causa)
		r.aberto.Detalhe = res.Detalhe
		return []Mudanca{{MudouCausa, *r.Aberto()}}
	}

	if r.oks == 0 {
		r.primeiroOK = em
	}
	r.oks++
	if r.oks < r.ParaFechar {
		return nil
	}
	fim := r.primeiroOK // voltou no primeiro ciclo bom
	r.aberto.Fim = &fim
	fechado := *r.Aberto()
	r.aberto, r.oks = nil, 0
	return []Mudanca{{Fechou, fechado}}
}

func juntar(lista []diagnostico.Causa, c diagnostico.Causa) []diagnostico.Causa {
	if slices.Contains(lista, c) {
		return lista
	}
	return append(lista, c)
}
