package armazenamento

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"netguard/internal/diagnostico"
	"netguard/internal/incidentes"
)

// Ciclo é um ciclo do monitor: o diagnóstico e as verificações que o geraram.
type Ciclo struct {
	Em           time.Time
	Estado       string
	Causa        string
	Detalhe      string
	Verificacoes []Verificacao
}

// SalvarCiclo grava o ciclo e as verificações numa transação só.
func (r *Repositorio) SalvarCiclo(ctx context.Context, c Ciclo) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO ciclos (em, estado, causa, detalhe) VALUES ($1, $2, $3, $4)",
			c.Em, c.Estado, c.Causa, vazioNulo(c.Detalhe)); err != nil {
			return err
		}
		return salvarVerificacoes(ctx, tx, c.Verificacoes)
	})
}

func (r *Repositorio) IniciarExecucao(ctx context.Context, inicio time.Time, versao string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		"INSERT INTO monitor_execucoes (inicio, fim, versao) VALUES ($1, $1, $2) RETURNING id",
		inicio, vazioNulo(versao)).Scan(&id)
	return id, err
}

func (r *Repositorio) BaterExecucao(ctx context.Context, id int64, em time.Time) error {
	_, err := r.pool.Exec(ctx, "UPDATE monitor_execucoes SET fim = $2 WHERE id = $1 AND fim < $2", id, em)
	return err
}

func (r *Repositorio) UltimaExecucaoFim(ctx context.Context) (time.Time, bool, error) {
	var fim time.Time
	err := r.pool.QueryRow(ctx, "SELECT fim FROM monitor_execucoes ORDER BY fim DESC LIMIT 1").Scan(&fim)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	return fim, err == nil, err
}

const colunasIncidente = "id, inicio, fim, causa, causas, coalesce(detalhe, '')"

func lerIncidente(row pgx.Row) (incidentes.Incidente, error) {
	var inc incidentes.Incidente
	var causas []string
	err := row.Scan(&inc.ID, &inc.Inicio, &inc.Fim, &inc.Causa, &causas, &inc.Detalhe)
	for _, c := range causas {
		inc.Causas = append(inc.Causas, diagnostico.Causa(c))
	}
	return inc, err
}

func (r *Repositorio) IncidenteAberto(ctx context.Context) (*incidentes.Incidente, error) {
	inc, err := lerIncidente(r.pool.QueryRow(ctx, "SELECT "+colunasIncidente+" FROM incidentes WHERE fim IS NULL"))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &inc, nil
}

// SalvarIncidente cria (ID 0) ou atualiza a queda e devolve o id.
func (r *Repositorio) SalvarIncidente(ctx context.Context, inc incidentes.Incidente) (int64, error) {
	causas := make([]string, len(inc.Causas))
	for i, c := range inc.Causas {
		causas[i] = string(c)
	}
	if inc.ID == 0 {
		var id int64
		err := r.pool.QueryRow(ctx,
			"INSERT INTO incidentes (inicio, fim, causa, causas, detalhe) VALUES ($1, $2, $3, $4, $5) RETURNING id",
			inc.Inicio, inc.Fim, string(inc.Causa), causas, vazioNulo(inc.Detalhe)).Scan(&id)
		return id, err
	}
	_, err := r.pool.Exec(ctx,
		"UPDATE incidentes SET fim = $2, causa = $3, causas = $4, detalhe = $5 WHERE id = $1",
		inc.ID, inc.Fim, string(inc.Causa), causas, vazioNulo(inc.Detalhe))
	return inc.ID, err
}

// Incidentes lista as quedas que tocam o período, da mais nova para a mais velha.
func (r *Repositorio) Incidentes(ctx context.Context, de, ate time.Time, limite int) ([]incidentes.Incidente, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+colunasIncidente+` FROM incidentes
		WHERE inicio < $2 AND (fim IS NULL OR fim > $1)
		ORDER BY inicio DESC LIMIT $3`, de, ate, limite)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lista []incidentes.Incidente
	for rows.Next() {
		inc, err := lerIncidente(rows)
		if err != nil {
			return nil, err
		}
		lista = append(lista, inc)
	}
	return lista, rows.Err()
}

type Disponibilidade struct {
	CiclosNoAr       int // ok + degradado
	CiclosTotal      int
	SegundosCobertos float64 // tempo com o monitor rodando
	Quedas           int
	SegundosFora     float64 // soma das quedas, cortadas no período
}

func (r *Repositorio) Disponibilidade(ctx context.Context, de, ate time.Time) (Disponibilidade, error) {
	var d Disponibilidade
	err := r.pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FILTER (WHERE estado <> 'fora') FROM ciclos WHERE em >= $1 AND em < $2),
		  (SELECT count(*) FROM ciclos WHERE em >= $1 AND em < $2),
		  (SELECT coalesce(sum(extract(epoch FROM least(fim, $2) - greatest(inicio, $1))), 0)
		     FROM monitor_execucoes WHERE fim > $1 AND inicio < $2),
		  (SELECT count(*) FROM incidentes WHERE inicio < $2 AND (fim IS NULL OR fim > $1)),
		  (SELECT coalesce(sum(extract(epoch FROM least(coalesce(fim, now()), $2) - greatest(inicio, $1))), 0)
		     FROM incidentes WHERE inicio < $2 AND (fim IS NULL OR fim > $1))`,
		de, ate).Scan(&d.CiclosNoAr, &d.CiclosTotal, &d.SegundosCobertos, &d.Quedas, &d.SegundosFora)
	return d, err
}

// PontoHistorico é a média de um intervalo do gráfico. nil = sem dado.
type PontoHistorico struct {
	Em            time.Time
	GatewayMS     *float64
	GatewayPerda  *float64
	InternetMS    *float64
	InternetPerda *float64
	DNSMS         *float64
	FracaoFora    *float64
}

// Historico agrupa as verificações em intervalos de `passo` entre de e ate.
func (r *Repositorio) Historico(ctx context.Context, de, ate time.Time, passo time.Duration) ([]PontoHistorico, error) {
	seg := int(passo.Seconds())
	rows, err := r.pool.Query(ctx, `
		WITH v AS (
		  SELECT date_bin(make_interval(secs => $3), em, TIMESTAMPTZ '2000-01-01') AS b,
		    avg(latencia_ms) FILTER (WHERE tipo = 'gateway' AND ok)     AS gw,
		    avg(perda)       FILTER (WHERE tipo = 'gateway')            AS gwp,
		    avg(latencia_ms) FILTER (WHERE tipo = 'externo' AND ok)     AS ext,
		    avg(perda)       FILTER (WHERE tipo = 'externo')            AS extp,
		    avg(latencia_ms) FILTER (WHERE tipo = 'dns_sistema' AND ok) AS dns
		  FROM verificacoes WHERE em >= $1 AND em < $2 GROUP BY b
		), c AS (
		  SELECT date_bin(make_interval(secs => $3), em, TIMESTAMPTZ '2000-01-01') AS b,
		    avg((estado = 'fora')::int) AS fora
		  FROM ciclos WHERE em >= $1 AND em < $2 GROUP BY b
		)
		SELECT coalesce(v.b, c.b), v.gw, v.gwp, v.ext, v.extp, v.dns, c.fora
		FROM v FULL JOIN c ON c.b = v.b ORDER BY 1`, de, ate, seg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pts []PontoHistorico
	for rows.Next() {
		var p PontoHistorico
		if err := rows.Scan(&p.Em, &p.GatewayMS, &p.GatewayPerda, &p.InternetMS, &p.InternetPerda, &p.DNSMS, &p.FracaoFora); err != nil {
			return nil, err
		}
		pts = append(pts, p)
	}
	return pts, rows.Err()
}

// LimparAntigos apaga o bruto velho. Ciclos e quedas ficam mais tempo.
func (r *Repositorio) LimparAntigos(ctx context.Context, agora time.Time) (int64, error) {
	t1, err := r.pool.Exec(ctx, "DELETE FROM verificacoes WHERE em < $1", agora.AddDate(0, 0, -35))
	if err != nil {
		return 0, err
	}
	t2, err := r.pool.Exec(ctx, "DELETE FROM ciclos WHERE em < $1", agora.AddDate(0, 0, -400))
	if err != nil {
		return 0, err
	}
	t3, err := r.pool.Exec(ctx, "DELETE FROM wifi_amostras WHERE em < $1", agora.AddDate(0, 0, -400))
	if err != nil {
		return 0, err
	}
	t4, err := r.pool.Exec(ctx, "DELETE FROM aparicoes WHERE fim < $1", agora.AddDate(0, 0, -400))
	return t1.RowsAffected() + t2.RowsAffected() + t3.RowsAffected() + t4.RowsAffected(), err
}
