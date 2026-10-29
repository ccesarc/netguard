// Package auth cuida de usuários, senhas e sessões.
//
// O armazenamento fica atrás da interface Armazem; o pacote armazenamento
// implementa com PostgreSQL e ArmazemMemoria serve aos testes.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"time"
	"unicode/utf8"
)

var (
	ErrNaoEncontrado  = errors.New("não encontrado")
	ErrJaExiste       = errors.New("já existe")
	ErrCredenciais    = errors.New("usuário ou senha incorretos")
	ErrBloqueado      = errors.New("muitas tentativas; espere um minuto")
	ErrSessaoInvalida = errors.New("sessão inválida ou expirada")
)

type Usuario struct {
	ID   int64  `json:"-"`
	Nome string `json:"nome"`
}

type Armazem interface {
	CriarUsuario(ctx context.Context, nome, senhaHash string) (int64, error)
	TrocarSenha(ctx context.Context, nome, senhaHash string) (int64, error)
	BuscarUsuario(ctx context.Context, nome string) (id int64, senhaHash string, err error)
	CriarSessao(ctx context.Context, tokenHash []byte, usuarioID int64, expira time.Time, ip, agente string) error
	BuscarSessao(ctx context.Context, tokenHash []byte) (Usuario, time.Time, error)
	RenovarSessao(ctx context.Context, tokenHash []byte, expira time.Time) error
	ApagarSessao(ctx context.Context, tokenHash []byte) error
	ApagarSessoesDoUsuario(ctx context.Context, usuarioID int64) error
	ApagarSessoesExpiradas(ctx context.Context, agora time.Time) (int64, error)
}

var nomeValido = regexp.MustCompile(`^[a-z0-9._-]{3,32}$`)

func ValidarNome(nome string) error {
	if !nomeValido.MatchString(nome) {
		return errors.New("nome: 3 a 32 caracteres, só letras minúsculas, números, ponto, hífen e sublinhado")
	}
	return nil
}

func ValidarSenha(senha string) error {
	n := utf8.RuneCountInString(senha)
	if n < 10 || n > 128 {
		return errors.New("senha: de 10 a 128 caracteres")
	}
	return nil
}

type Servico struct {
	armazem   Armazem
	validade  time.Duration
	limitador *Limitador
	agora     func() time.Time
	hashFalso string // usado quando o usuário não existe, para o tempo de resposta não denunciar
}

func NovoServico(a Armazem, validade time.Duration) *Servico {
	falso, err := HashSenha("senha-que-nunca-confere")
	if err != nil {
		panic(err) // só falha se o sistema não tiver fonte de aleatoriedade
	}
	return &Servico{
		armazem:   a,
		validade:  validade,
		limitador: NovoLimitador(5, time.Minute),
		agora:     time.Now,
		hashFalso: falso,
	}
}

func (s *Servico) CriarUsuario(ctx context.Context, nome, senha string) error {
	if err := ValidarNome(nome); err != nil {
		return err
	}
	if err := ValidarSenha(senha); err != nil {
		return err
	}
	h, err := HashSenha(senha)
	if err != nil {
		return err
	}
	_, err = s.armazem.CriarUsuario(ctx, nome, h)
	return err
}

// TrocarSenha também derruba as sessões abertas do usuário.
func (s *Servico) TrocarSenha(ctx context.Context, nome, senha string) error {
	if err := ValidarSenha(senha); err != nil {
		return err
	}
	h, err := HashSenha(senha)
	if err != nil {
		return err
	}
	id, err := s.armazem.TrocarSenha(ctx, nome, h)
	if err != nil {
		return err
	}
	return s.armazem.ApagarSessoesDoUsuario(ctx, id)
}

// Entrar confere a senha e abre uma sessão. Devolve o token (vai no cookie).
func (s *Servico) Entrar(ctx context.Context, nome, senha, ip, agente string) (Usuario, string, time.Time, error) {
	if s.limitador.Bloqueado(ip) {
		return Usuario{}, "", time.Time{}, ErrBloqueado
	}
	id, hash, err := s.armazem.BuscarUsuario(ctx, nome)
	if errors.Is(err, ErrNaoEncontrado) {
		VerificarSenha(senha, s.hashFalso)
		s.limitador.Falha(ip)
		return Usuario{}, "", time.Time{}, ErrCredenciais
	}
	if err != nil {
		return Usuario{}, "", time.Time{}, err
	}
	if !VerificarSenha(senha, hash) {
		s.limitador.Falha(ip)
		return Usuario{}, "", time.Time{}, ErrCredenciais
	}
	s.limitador.Sucesso(ip)

	bruto := make([]byte, 32)
	if _, err := rand.Read(bruto); err != nil {
		return Usuario{}, "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(bruto)
	expira := s.agora().Add(s.validade)
	if err := s.armazem.CriarSessao(ctx, hashToken(token), id, expira, ip, limitar(agente, 200)); err != nil {
		return Usuario{}, "", time.Time{}, fmt.Errorf("criar sessão: %w", err)
	}
	return Usuario{ID: id, Nome: nome}, token, expira, nil
}

// Validar devolve o dono da sessão. Se a sessão passou da metade da validade,
// ela é renovada e novaExpira vem preenchida (o cookie precisa ser regravado).
func (s *Servico) Validar(ctx context.Context, token string) (u Usuario, novaExpira time.Time, err error) {
	if len(token) != 43 { // 32 bytes em base64 sem preenchimento
		return Usuario{}, time.Time{}, ErrSessaoInvalida
	}
	th := hashToken(token)
	u, expira, err := s.armazem.BuscarSessao(ctx, th)
	if errors.Is(err, ErrNaoEncontrado) {
		return Usuario{}, time.Time{}, ErrSessaoInvalida
	}
	if err != nil {
		return Usuario{}, time.Time{}, err
	}
	agora := s.agora()
	if !expira.After(agora) {
		_ = s.armazem.ApagarSessao(ctx, th)
		return Usuario{}, time.Time{}, ErrSessaoInvalida
	}
	if expira.Sub(agora) < s.validade/2 {
		nova := agora.Add(s.validade)
		if err := s.armazem.RenovarSessao(ctx, th, nova); err == nil {
			novaExpira = nova
		}
	}
	return u, novaExpira, nil
}

func (s *Servico) Sair(ctx context.Context, token string) error {
	return s.armazem.ApagarSessao(ctx, hashToken(token))
}

func (s *Servico) LimparExpiradas(ctx context.Context) (int64, error) {
	return s.armazem.ApagarSessoesExpiradas(ctx, s.agora())
}

// No banco só fica o hash do token: quem ler o banco não consegue entrar.
func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func limitar(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
