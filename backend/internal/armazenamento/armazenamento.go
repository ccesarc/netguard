// Package armazenamento fala com o PostgreSQL: conexão, migrações e consultas.
// Toda consulta usa parâmetros ($1, $2...); nada de SQL montado com texto de fora.
package armazenamento

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var arquivosMigracao embed.FS

// Conectar abre o pool e espera o banco responder. O postgres da infra pode
// subir depois do serviço, então tenta de novo com espera crescente até o ctx acabar.
func Conectar(ctx context.Context, url string, log *slog.Logger) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("URL do banco inválida: %w", err)
	}
	return conectarComConfig(ctx, cfg, log)
}

func conectarComConfig(ctx context.Context, cfg *pgxpool.Config, log *slog.Logger) (*pgxpool.Pool, error) {
	cfg.MaxConns = 4
	cfg.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	espera := time.Second
	for {
		pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = pool.Ping(pctx)
		cancel()
		if err == nil {
			return pool, nil
		}
		log.Warn("banco indisponível, tentando de novo", "erro", err, "espera", espera.String())
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(espera):
		}
		espera = min(espera*2, 30*time.Second)
	}
}

type migracao struct {
	versao int
	nome   string
	sql    string
}

func carregarMigracoes() ([]migracao, error) {
	entradas, err := fs.ReadDir(arquivosMigracao, "migrations")
	if err != nil {
		return nil, err
	}
	var ms []migracao
	vistas := map[int]bool{}
	for _, e := range entradas {
		prefixo, _, ok := strings.Cut(e.Name(), "_")
		v, err := strconv.Atoi(prefixo)
		if !ok || err != nil || v <= 0 {
			return nil, fmt.Errorf("nome de migração inválido: %s", e.Name())
		}
		if vistas[v] {
			return nil, fmt.Errorf("versão de migração repetida: %d", v)
		}
		vistas[v] = true
		b, err := fs.ReadFile(arquivosMigracao, "migrations/"+e.Name())
		if err != nil {
			return nil, err
		}
		ms = append(ms, migracao{versao: v, nome: e.Name(), sql: string(b)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].versao < ms[j].versao })
	return ms, nil
}

// chave da trava para dois processos não migrarem ao mesmo tempo.
const chaveTravaMigracao = 7418202610

// Migrar aplica as migrações que faltam, cada uma na sua transação.
// Devolve os nomes das que foram aplicadas agora.
func Migrar(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	ms, err := carregarMigracoes()
	if err != nil {
		return nil, err
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", int64(chaveTravaMigracao)); err != nil {
		return nil, err
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", int64(chaveTravaMigracao))

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migracoes (
		versao      integer PRIMARY KEY,
		nome        text NOT NULL,
		aplicada_em timestamptz NOT NULL DEFAULT now())`); err != nil {
		return nil, err
	}

	aplicadas := map[int]bool{}
	rows, err := conn.Query(ctx, "SELECT versao FROM schema_migracoes")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return nil, err
		}
		aplicadas[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var feitas []string
	for _, m := range ms {
		if aplicadas[m.versao] {
			continue
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return feitas, err
		}
		if _, err := tx.Exec(ctx, m.sql); err != nil {
			tx.Rollback(ctx)
			return feitas, fmt.Errorf("migração %s: %w", m.nome, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migracoes (versao, nome) VALUES ($1, $2)", m.versao, m.nome); err != nil {
			tx.Rollback(ctx)
			return feitas, err
		}
		if err := tx.Commit(ctx); err != nil {
			return feitas, err
		}
		feitas = append(feitas, m.nome)
	}
	return feitas, nil
}
