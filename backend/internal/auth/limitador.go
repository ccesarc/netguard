package auth

import (
	"sync"
	"time"
)

// Limitador conta falhas de login por chave (IP) numa janela deslizante.
type Limitador struct {
	mu     sync.Mutex
	max    int
	janela time.Duration
	agora  func() time.Time
	falhas map[string][]time.Time
}

func NovoLimitador(max int, janela time.Duration) *Limitador {
	return &Limitador{max: max, janela: janela, agora: time.Now, falhas: map[string][]time.Time{}}
}

func (l *Limitador) Bloqueado(chave string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.podar(chave)) >= l.max
}

func (l *Limitador) Falha(chave string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.falhas[chave] = append(l.podar(chave), l.agora())
}

func (l *Limitador) Sucesso(chave string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.falhas, chave)
}

func (l *Limitador) podar(chave string) []time.Time {
	limite := l.agora().Add(-l.janela)
	lista := l.falhas[chave]
	i := 0
	for i < len(lista) && !lista[i].After(limite) {
		i++
	}
	lista = lista[i:]
	if len(lista) == 0 {
		delete(l.falhas, chave)
		return nil
	}
	l.falhas[chave] = lista
	return lista
}
