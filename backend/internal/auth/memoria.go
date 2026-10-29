package auth

import (
	"context"
	"sync"
	"time"
)

// ArmazemMemoria guarda tudo em memória. Usado nos testes (deste pacote e da API).
type ArmazemMemoria struct {
	mu       sync.Mutex
	proximo  int64
	usuarios map[string]usuarioMem
	sessoes  map[string]sessaoMem
}

type usuarioMem struct {
	id   int64
	hash string
}

type sessaoMem struct {
	usuarioID int64
	expira    time.Time
}

func NovoArmazemMemoria() *ArmazemMemoria {
	return &ArmazemMemoria{usuarios: map[string]usuarioMem{}, sessoes: map[string]sessaoMem{}}
}

func (a *ArmazemMemoria) CriarUsuario(_ context.Context, nome, hash string) (int64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.usuarios[nome]; ok {
		return 0, ErrJaExiste
	}
	a.proximo++
	a.usuarios[nome] = usuarioMem{id: a.proximo, hash: hash}
	return a.proximo, nil
}

func (a *ArmazemMemoria) TrocarSenha(_ context.Context, nome, hash string) (int64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	u, ok := a.usuarios[nome]
	if !ok {
		return 0, ErrNaoEncontrado
	}
	u.hash = hash
	a.usuarios[nome] = u
	return u.id, nil
}

func (a *ArmazemMemoria) BuscarUsuario(_ context.Context, nome string) (int64, string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	u, ok := a.usuarios[nome]
	if !ok {
		return 0, "", ErrNaoEncontrado
	}
	return u.id, u.hash, nil
}

func (a *ArmazemMemoria) CriarSessao(_ context.Context, th []byte, uid int64, expira time.Time, _, _ string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sessoes[string(th)] = sessaoMem{usuarioID: uid, expira: expira}
	return nil
}

func (a *ArmazemMemoria) BuscarSessao(_ context.Context, th []byte) (Usuario, time.Time, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessoes[string(th)]
	if !ok {
		return Usuario{}, time.Time{}, ErrNaoEncontrado
	}
	for nome, u := range a.usuarios {
		if u.id == s.usuarioID {
			return Usuario{ID: u.id, Nome: nome}, s.expira, nil
		}
	}
	return Usuario{}, time.Time{}, ErrNaoEncontrado
}

func (a *ArmazemMemoria) RenovarSessao(_ context.Context, th []byte, expira time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessoes[string(th)]
	if !ok {
		return ErrNaoEncontrado
	}
	s.expira = expira
	a.sessoes[string(th)] = s
	return nil
}

func (a *ArmazemMemoria) ApagarSessao(_ context.Context, th []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessoes, string(th))
	return nil
}

func (a *ArmazemMemoria) ApagarSessoesDoUsuario(_ context.Context, uid int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, s := range a.sessoes {
		if s.usuarioID == uid {
			delete(a.sessoes, k)
		}
	}
	return nil
}

func (a *ArmazemMemoria) ApagarSessoesExpiradas(_ context.Context, agora time.Time) (int64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var n int64
	for k, s := range a.sessoes {
		if !s.expira.After(agora) {
			delete(a.sessoes, k)
			n++
		}
	}
	return n, nil
}

// QuantasSessoes ajuda os testes a conferir o efeito de sair/trocar senha.
func (a *ArmazemMemoria) QuantasSessoes() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.sessoes)
}
