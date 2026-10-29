package ajustes

import (
	"context"
	"slices"
	"sync"
)

type Armazem interface {
	LerAjustes(ctx context.Context) (Ajustes, bool, error)
	SalvarAjustes(ctx context.Context, a Ajustes, por string) error
}

// Gerente guarda a configuração em uso e avisa quem precisa quando ela muda.
type Gerente struct {
	armazem Armazem
	padrao  Ajustes

	mu      sync.RWMutex
	atual   Ajustes
	aoMudar []func(Ajustes)
}

// NovoGerente carrega do banco. Se o que está salvo não passa mais na
// validação (regra mudou numa versão nova), usa o padrão e avisa em invalido.
func NovoGerente(ctx context.Context, a Armazem, padrao Ajustes) (g *Gerente, invalido error, err error) {
	g = &Gerente{armazem: a, padrao: padrao, atual: padrao}
	salvo, ok, err := a.LerAjustes(ctx)
	if err != nil {
		return nil, nil, err
	}
	if ok {
		if invalido = salvo.Validar(); invalido == nil {
			g.atual = salvo
		}
	}
	return g, invalido, nil
}

func (g *Gerente) Atuais() Ajustes {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return copiar(g.atual)
}

func (g *Gerente) Padrao() Ajustes { return copiar(g.padrao) }

func (g *Gerente) Velocidade() Velocidade {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.atual.Velocidade
}

// AoMudar registra quem precisa saber da configuração nova (e chama já com a atual).
func (g *Gerente) AoMudar(f func(Ajustes)) {
	g.mu.Lock()
	g.aoMudar = append(g.aoMudar, f)
	atual := copiar(g.atual)
	g.mu.Unlock()
	f(atual)
}

// Salvar valida, grava e aplica. Inválido não grava nada.
func (g *Gerente) Salvar(ctx context.Context, a Ajustes, por string) error {
	if err := a.Validar(); err != nil {
		return err
	}
	if err := g.armazem.SalvarAjustes(ctx, a, por); err != nil {
		return err
	}
	g.mu.Lock()
	g.atual = copiar(a)
	avisar := slices.Clone(g.aoMudar)
	g.mu.Unlock()
	for _, f := range avisar {
		f(copiar(a))
	}
	return nil
}

func copiar(a Ajustes) Ajustes {
	a.Externos = slices.Clone(a.Externos)
	a.URLs = slices.Clone(a.URLs)
	return a
}
