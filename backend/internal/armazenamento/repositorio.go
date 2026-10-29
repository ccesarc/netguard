package armazenamento

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"netguard/internal/auth"
)

type Repositorio struct {
	pool *pgxpool.Pool
}

func NovoRepositorio(pool *pgxpool.Pool) *Repositorio {
	return &Repositorio{pool: pool}
}

func (r *Repositorio) Saude(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

// ---- usuários e sessões (implementa auth.Armazem) ----

func (r *Repositorio) CriarUsuario(ctx context.Context, nome, hash string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		"INSERT INTO usuarios (nome, senha_hash) VALUES ($1, $2) RETURNING id", nome, hash).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return 0, auth.ErrJaExiste
	}
	return id, err
}

func (r *Repositorio) TrocarSenha(ctx context.Context, nome, hash string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		"UPDATE usuarios SET senha_hash = $2, atualizado_em = now() WHERE nome = $1 RETURNING id",
		nome, hash).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, auth.ErrNaoEncontrado
	}
	return id, err
}

func (r *Repositorio) BuscarUsuario(ctx context.Context, nome string) (int64, string, error) {
	var id int64
	var hash string
	err := r.pool.QueryRow(ctx, "SELECT id, senha_hash FROM usuarios WHERE nome = $1", nome).Scan(&id, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", auth.ErrNaoEncontrado
	}
	return id, hash, err
}

func (r *Repositorio) QuantosUsuarios(ctx context.Context) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, "SELECT count(*) FROM usuarios").Scan(&n)
	return n, err
}

func (r *Repositorio) CriarSessao(ctx context.Context, th []byte, uid int64, expira time.Time, ip, agente string) error {
	_, err := r.pool.Exec(ctx,
		"INSERT INTO sessoes (token_hash, usuario_id, expira_em, ip, agente) VALUES ($1, $2, $3, $4, $5)",
		th, uid, expira, ip, agente)
	return err
}

func (r *Repositorio) BuscarSessao(ctx context.Context, th []byte) (auth.Usuario, time.Time, error) {
	var u auth.Usuario
	var expira time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT u.id, u.nome, s.expira_em
		FROM sessoes s JOIN usuarios u ON u.id = s.usuario_id
		WHERE s.token_hash = $1`, th).Scan(&u.ID, &u.Nome, &expira)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Usuario{}, time.Time{}, auth.ErrNaoEncontrado
	}
	return u, expira, err
}

func (r *Repositorio) RenovarSessao(ctx context.Context, th []byte, expira time.Time) error {
	tag, err := r.pool.Exec(ctx, "UPDATE sessoes SET expira_em = $2 WHERE token_hash = $1", th, expira)
	if err == nil && tag.RowsAffected() == 0 {
		return auth.ErrNaoEncontrado
	}
	return err
}

func (r *Repositorio) ApagarSessao(ctx context.Context, th []byte) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM sessoes WHERE token_hash = $1", th)
	return err
}

func (r *Repositorio) ApagarSessoesDoUsuario(ctx context.Context, uid int64) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM sessoes WHERE usuario_id = $1", uid)
	return err
}

func (r *Repositorio) ApagarSessoesExpiradas(ctx context.Context, agora time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx, "DELETE FROM sessoes WHERE expira_em <= $1", agora)
	return tag.RowsAffected(), err
}

// ---- auditoria ----

type Auditoria struct {
	UsuarioID *int64
	Usuario   string
	Acao      string
	Alvo      string
	Detalhe   map[string]any
	IP        string
}

func (r *Repositorio) Auditar(ctx context.Context, a Auditoria) error {
	var detalhe []byte
	if a.Detalhe != nil {
		var err error
		if detalhe, err = json.Marshal(a.Detalhe); err != nil {
			return err
		}
	}
	_, err := r.pool.Exec(ctx,
		"INSERT INTO auditoria (usuario_id, usuario, acao, alvo, detalhe, ip) VALUES ($1, $2, $3, $4, $5, $6)",
		a.UsuarioID, vazioNulo(a.Usuario), a.Acao, vazioNulo(a.Alvo), detalhe, vazioNulo(a.IP))
	return err
}

// ---- verificações ----

type Verificacao struct {
	Em         time.Time
	Tipo       string
	Alvo       string
	OK         bool
	LatenciaMS *float64
	Perda      *float64
	Erro       string
	Dados      map[string]any
}

func (r *Repositorio) SalvarVerificacoes(ctx context.Context, vs []Verificacao) error {
	return salvarVerificacoes(ctx, r.pool, vs)
}

type enviaLote interface {
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

func salvarVerificacoes(ctx context.Context, db enviaLote, vs []Verificacao) error {
	if len(vs) == 0 {
		return nil
	}
	b := &pgx.Batch{}
	for _, v := range vs {
		var dados []byte
		if v.Dados != nil {
			var err error
			if dados, err = json.Marshal(v.Dados); err != nil {
				return err
			}
		}
		b.Queue(`INSERT INTO verificacoes (em, tipo, alvo, ok, latencia_ms, perda, erro, dados)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			v.Em, v.Tipo, v.Alvo, v.OK, v.LatenciaMS, v.Perda, vazioNulo(v.Erro), dados)
	}
	return db.SendBatch(ctx, b).Close()
}

func vazioNulo(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
