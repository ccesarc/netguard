package incidentes

import (
	"testing"
	"time"

	d "netguard/internal/diagnostico"
)

var t0 = time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)

func em(i int) time.Time { return t0.Add(time.Duration(i) * 10 * time.Second) }

var (
	bom     = d.Resultado{Estado: d.OK, Causa: d.CausaNenhuma}
	lento   = d.Resultado{Estado: d.Degradado, Causa: d.CausaLatencia}
	semNet  = d.Resultado{Estado: d.Fora, Causa: d.CausaExterno}
	semWifi = d.Resultado{Estado: d.Fora, Causa: d.CausaRedeLocal}
	semDNS  = d.Resultado{Estado: d.Fora, Causa: d.CausaDNS}
)

// rodar alimenta a sequência e devolve todas as mudanças.
func rodar(r *Rastreador, seq ...d.Resultado) []Mudanca {
	var tudo []Mudanca
	for i, res := range seq {
		tudo = append(tudo, r.Observar(em(i), res)...)
	}
	return tudo
}

func TestFalhaCurtaNaoViraQueda(t *testing.T) {
	r := NovoRastreador(3, 2)
	if m := rodar(r, bom, semNet, semNet, bom, semNet, bom); len(m) != 0 {
		t.Fatalf("duas falhas seguidas não deveriam abrir queda: %+v", m)
	}
}

func TestQuedaERecuperacao(t *testing.T) {
	r := NovoRastreador(3, 2)
	m := rodar(r, bom, semNet, semNet, semNet, semNet, bom, bom, bom)
	if len(m) != 2 || m[0].Tipo != Abriu || m[1].Tipo != Fechou {
		t.Fatalf("esperava abrir e fechar: %+v", m)
	}
	q := m[1].Incidente
	if !q.Inicio.Equal(em(1)) {
		t.Errorf("início deveria ser o primeiro ciclo ruim (%v), veio %v", em(1), q.Inicio)
	}
	if q.Fim == nil || !q.Fim.Equal(em(5)) {
		t.Errorf("fim deveria ser o primeiro ciclo bom (%v), veio %v", em(5), q.Fim)
	}
	if d := q.Fim.Sub(q.Inicio); d != 40*time.Second {
		t.Errorf("duração: %v", d)
	}
	if r.Aberto() != nil {
		t.Error("não deveria ter queda aberta")
	}
}

func TestDegradadoNaoEQueda(t *testing.T) {
	r := NovoRastreador(3, 2)
	if m := rodar(r, lento, lento, lento, lento); len(m) != 0 {
		t.Fatalf("internet lenta não é queda: %+v", m)
	}
}

func TestUmCicloBomNoMeioNaoFecha(t *testing.T) {
	r := NovoRastreador(3, 2)
	m := rodar(r, semNet, semNet, semNet, bom, semNet, bom, bom)
	if len(m) != 2 || m[1].Tipo != Fechou {
		t.Fatalf("mudanças: %+v", m)
	}
	if !m[1].Incidente.Fim.Equal(em(5)) {
		t.Errorf("fim deveria contar só os bons seguidos: %v", m[1].Incidente.Fim)
	}
}

func TestMudancaDeCausa(t *testing.T) {
	r := NovoRastreador(3, 2)
	m := rodar(r, semWifi, semNet, semNet, semDNS, semDNS)
	if len(m) != 2 || m[0].Tipo != Abriu || m[1].Tipo != MudouCausa {
		t.Fatalf("mudanças: %+v", m)
	}
	if m[0].Incidente.Causa != d.CausaExterno {
		t.Errorf("causa ao abrir = a do ciclo que abriu: %s", m[0].Incidente.Causa)
	}
	causas := m[1].Incidente.Causas
	if len(causas) != 3 || causas[0] != d.CausaRedeLocal || causas[2] != d.CausaDNS {
		t.Errorf("causas registradas: %v", causas)
	}
}

func TestIDGuardado(t *testing.T) {
	r := NovoRastreador(1, 1)
	r.Observar(em(0), semNet)
	r.DefinirID(42)
	m := r.Observar(em(1), bom)
	if m[0].Incidente.ID != 42 {
		t.Fatalf("id perdido: %+v", m[0].Incidente)
	}
}

func TestAbertoEhCopia(t *testing.T) {
	r := NovoRastreador(1, 2)
	r.Observar(em(0), semNet)
	a := r.Aberto()
	a.Causas[0] = "mexido"
	if r.Aberto().Causas[0] != d.CausaExterno {
		t.Fatal("Aberto() expôs o estado interno")
	}
}
