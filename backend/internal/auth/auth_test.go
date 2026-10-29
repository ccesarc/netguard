package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

const senhaBoa = "cavalo-correto-bateria"

func TestHashSenha(t *testing.T) {
	h, err := HashSenha(senhaBoa)
	if err != nil {
		t.Fatal(err)
	}
	if !VerificarSenha(senhaBoa, h) {
		t.Fatal("senha certa não conferiu")
	}
	if VerificarSenha("outra-senha-qualquer", h) {
		t.Fatal("senha errada conferiu")
	}
	h2, _ := HashSenha(senhaBoa)
	if h == h2 {
		t.Fatal("dois hashes iguais: salt não está variando")
	}
	for _, ruim := range []string{"", "texto", "$argon2id$v=19$m=1,t=1,p=1$$", "$argon2id$v=19$m=99999999,t=1,p=1$YQ$YQ"} {
		if VerificarSenha(senhaBoa, ruim) {
			t.Fatalf("hash malformado conferiu: %q", ruim)
		}
	}
}

func TestValidacoes(t *testing.T) {
	for _, n := range []string{"dono", "admin.casa", "a_b-c"} {
		if ValidarNome(n) != nil {
			t.Errorf("nome válido recusado: %q", n)
		}
	}
	for _, n := range []string{"", "ab", "Dono", "com espaço", "x'; drop table--", "muitolongomuitolongomuitolongomuito"} {
		if ValidarNome(n) == nil {
			t.Errorf("nome inválido aceito: %q", n)
		}
	}
	if ValidarSenha("curta") == nil || ValidarSenha(senhaBoa) != nil {
		t.Fatal("validação de senha errada")
	}
}

type relogio struct{ t time.Time }

func (r *relogio) agora() time.Time { return r.t }

func novoServicoTeste(t *testing.T) (*Servico, *ArmazemMemoria, *relogio) {
	t.Helper()
	a := NovoArmazemMemoria()
	s := NovoServico(a, 24*time.Hour)
	r := &relogio{t: time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)}
	s.agora = r.agora
	s.limitador.agora = r.agora
	if err := s.CriarUsuario(context.Background(), "dono", senhaBoa); err != nil {
		t.Fatal(err)
	}
	return s, a, r
}

func TestEntrarEValidar(t *testing.T) {
	s, _, _ := novoServicoTeste(t)
	ctx := context.Background()

	u, token, expira, err := s.Entrar(ctx, "dono", senhaBoa, "10.0.0.2", "teste")
	if err != nil {
		t.Fatal(err)
	}
	if u.Nome != "dono" || token == "" || expira.IsZero() {
		t.Fatalf("retorno estranho: %+v %q %v", u, token, expira)
	}
	v, nova, err := s.Validar(ctx, token)
	if err != nil || v.Nome != "dono" {
		t.Fatalf("validar: %v %+v", err, v)
	}
	if !nova.IsZero() {
		t.Fatal("sessão nova não deveria ser renovada")
	}
	if _, _, err := s.Validar(ctx, "token-que-nao-existe-token-que-nao-existe-x"); !errors.Is(err, ErrSessaoInvalida) {
		t.Fatalf("token desconhecido: %v", err)
	}
	if _, _, err := s.Validar(ctx, "curto"); !errors.Is(err, ErrSessaoInvalida) {
		t.Fatalf("token curto: %v", err)
	}
}

func TestCredenciaisErradas(t *testing.T) {
	s, _, _ := novoServicoTeste(t)
	ctx := context.Background()
	if _, _, _, err := s.Entrar(ctx, "dono", "senha-errada-123", "ip", ""); !errors.Is(err, ErrCredenciais) {
		t.Fatalf("senha errada: %v", err)
	}
	if _, _, _, err := s.Entrar(ctx, "fulano", senhaBoa, "ip", ""); !errors.Is(err, ErrCredenciais) {
		t.Fatalf("usuário inexistente deveria dar o mesmo erro: %v", err)
	}
}

func TestBloqueioPorTentativas(t *testing.T) {
	s, _, r := novoServicoTeste(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		s.Entrar(ctx, "dono", "senha-errada-123", "10.0.0.9", "")
	}
	if _, _, _, err := s.Entrar(ctx, "dono", senhaBoa, "10.0.0.9", ""); !errors.Is(err, ErrBloqueado) {
		t.Fatalf("esperava bloqueio, veio %v", err)
	}
	if _, _, _, err := s.Entrar(ctx, "dono", senhaBoa, "10.0.0.10", ""); err != nil {
		t.Fatalf("outro IP não deveria estar bloqueado: %v", err)
	}
	r.t = r.t.Add(61 * time.Second)
	if _, _, _, err := s.Entrar(ctx, "dono", senhaBoa, "10.0.0.9", ""); err != nil {
		t.Fatalf("bloqueio deveria acabar depois de 1 min: %v", err)
	}
}

func TestExpiracaoERenovacao(t *testing.T) {
	s, a, r := novoServicoTeste(t)
	ctx := context.Background()
	_, token, _, _ := s.Entrar(ctx, "dono", senhaBoa, "ip", "")

	r.t = r.t.Add(13 * time.Hour) // passou da metade de 24 h
	_, nova, err := s.Validar(ctx, token)
	if err != nil || nova.IsZero() {
		t.Fatalf("deveria renovar: %v %v", err, nova)
	}
	r.t = r.t.Add(23 * time.Hour) // dentro da nova validade
	if _, _, err := s.Validar(ctx, token); err != nil {
		t.Fatalf("renovada deveria valer: %v", err)
	}
	r.t = r.t.Add(48 * time.Hour)
	if _, _, err := s.Validar(ctx, token); !errors.Is(err, ErrSessaoInvalida) {
		t.Fatalf("expirada: %v", err)
	}
	if a.QuantasSessoes() != 0 {
		t.Fatal("sessão expirada deveria ser apagada")
	}
}

func TestSairETrocarSenha(t *testing.T) {
	s, a, _ := novoServicoTeste(t)
	ctx := context.Background()
	_, t1, _, _ := s.Entrar(ctx, "dono", senhaBoa, "ip", "")
	_, _, _, _ = s.Entrar(ctx, "dono", senhaBoa, "ip2", "")

	if err := s.Sair(ctx, t1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Validar(ctx, t1); !errors.Is(err, ErrSessaoInvalida) {
		t.Fatal("token depois de sair ainda vale")
	}
	if err := s.TrocarSenha(ctx, "dono", "outra-senha-bem-longa"); err != nil {
		t.Fatal(err)
	}
	if a.QuantasSessoes() != 0 {
		t.Fatal("trocar senha deveria derrubar as sessões")
	}
	if _, _, _, err := s.Entrar(ctx, "dono", senhaBoa, "ip3", ""); !errors.Is(err, ErrCredenciais) {
		t.Fatal("senha antiga ainda entra")
	}
}

func TestUsuarioDuplicado(t *testing.T) {
	s, _, _ := novoServicoTeste(t)
	if err := s.CriarUsuario(context.Background(), "dono", senhaBoa); !errors.Is(err, ErrJaExiste) {
		t.Fatalf("duplicado: %v", err)
	}
}
