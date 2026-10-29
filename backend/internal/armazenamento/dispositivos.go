package armazenamento

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"netguard/internal/dispositivos"
)

// RegistrarObservacoes grava uma coleta: cria ou atualiza cada aparelho e
// estende (ou abre) a aparição dele. Ninguém é "novo" na primeira coleta de
// todas nem quando retrato=true (primeira procura ativa: aparelhos que já
// estavam em casa, só não tinham sido achados). Devolve quantos novos surgiram.
func (r *Repositorio) RegistrarObservacoes(ctx context.Context, em time.Time, obs []dispositivos.Observacao, janela time.Duration, retrato bool) (int, error) {
	novos := 0
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var vazio bool
		if err := tx.QueryRow(ctx, "SELECT NOT EXISTS (SELECT 1 FROM dispositivos)").Scan(&vazio); err != nil {
			return err
		}
		vazio = vazio || retrato
		for _, o := range obs {
			var id int64
			var inserido bool
			err := tx.QueryRow(ctx, `
				INSERT INTO dispositivos (mac, ip, nome_descoberto, fabricante, mac_aleatorio, este_aparelho, alertar_novo, primeiro_visto, ultimo_visto)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
				ON CONFLICT (mac) DO UPDATE SET
				  ip = excluded.ip,
				  nome_descoberto = coalesce(excluded.nome_descoberto, dispositivos.nome_descoberto),
				  fabricante = coalesce(excluded.fabricante, dispositivos.fabricante),
				  este_aparelho = dispositivos.este_aparelho OR excluded.este_aparelho,
				  ultimo_visto = greatest(dispositivos.ultimo_visto, excluded.ultimo_visto)
				RETURNING id, (xmax = 0)`,
				o.MAC, o.IP, vazioNulo(o.Nome), vazioNulo(o.Fabricante), o.Aleatorio, o.EsteAparelho,
				!vazio && !o.EsteAparelho, em).Scan(&id, &inserido)
			if err != nil {
				return err
			}
			if inserido && !vazio && !o.EsteAparelho {
				novos++
			}
			tag, err := tx.Exec(ctx, `
				UPDATE aparicoes SET fim = $2, ip = $3
				WHERE id = (SELECT id FROM aparicoes WHERE dispositivo_id = $1 ORDER BY fim DESC LIMIT 1)
				  AND fim >= $4`,
				id, em, o.IP, em.Add(-janela))
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				if _, err := tx.Exec(ctx, "INSERT INTO aparicoes (dispositivo_id, inicio, fim, ip) VALUES ($1, $2, $2, $3)", id, em, o.IP); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return novos, err
}

func (r *Repositorio) PrimeiraVarreduraFeita(ctx context.Context) (bool, error) {
	var feita bool
	err := r.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM configuracao WHERE chave = 'primeira_varredura')").Scan(&feita)
	return feita, err
}

func (r *Repositorio) MarcarPrimeiraVarredura(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO configuracao (chave, valor) VALUES ('primeira_varredura', to_jsonb(now()))
		ON CONFLICT (chave) DO NOTHING`)
	return err
}

type Dispositivo struct {
	ID             int64     `json:"id"`
	MAC            string    `json:"mac"`
	IP             string    `json:"ip"`
	NomeDescoberto string    `json:"nome_descoberto"`
	Apelido        string    `json:"apelido"`
	Notas          string    `json:"notas"`
	Fabricante     string    `json:"fabricante"`
	MACAleatorio   bool      `json:"mac_aleatorio"`
	Confiavel      bool      `json:"confiavel"`
	EsteAparelho   bool      `json:"este_aparelho"`
	AlertarNovo    bool      `json:"alertar_novo"`
	PrimeiroVisto  time.Time `json:"primeiro_visto"`
	UltimoVisto    time.Time `json:"ultimo_visto"`
}

const colunasDispositivo = `id, mac::text, coalesce(host(ip), ''), coalesce(nome_descoberto, ''), coalesce(apelido, ''),
	coalesce(notas, ''), coalesce(fabricante, ''), mac_aleatorio, confiavel, este_aparelho, alertar_novo,
	primeiro_visto, ultimo_visto`

func lerDispositivo(row pgx.Row) (Dispositivo, error) {
	var d Dispositivo
	err := row.Scan(&d.ID, &d.MAC, &d.IP, &d.NomeDescoberto, &d.Apelido, &d.Notas, &d.Fabricante,
		&d.MACAleatorio, &d.Confiavel, &d.EsteAparelho, &d.AlertarNovo, &d.PrimeiroVisto, &d.UltimoVisto)
	return d, err
}

func (r *Repositorio) Dispositivos(ctx context.Context) ([]Dispositivo, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+colunasDispositivo+" FROM dispositivos ORDER BY ultimo_visto DESC, ip LIMIT 1000")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lista []Dispositivo
	for rows.Next() {
		d, err := lerDispositivo(rows)
		if err != nil {
			return nil, err
		}
		lista = append(lista, d)
	}
	return lista, rows.Err()
}

var ErrNaoEncontrado = errors.New("não encontrado")

func (r *Repositorio) Dispositivo(ctx context.Context, id int64) (Dispositivo, error) {
	d, err := lerDispositivo(r.pool.QueryRow(ctx, "SELECT "+colunasDispositivo+" FROM dispositivos WHERE id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNaoEncontrado
	}
	return d, err
}

type Aparicao struct {
	Inicio time.Time `json:"inicio"`
	Fim    time.Time `json:"fim"`
	IP     string    `json:"ip"`
}

func (r *Repositorio) Aparicoes(ctx context.Context, id int64, limite int) ([]Aparicao, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT inicio, fim, coalesce(host(ip), '') FROM aparicoes WHERE dispositivo_id = $1 ORDER BY fim DESC LIMIT $2", id, limite)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lista []Aparicao
	for rows.Next() {
		var a Aparicao
		if err := rows.Scan(&a.Inicio, &a.Fim, &a.IP); err != nil {
			return nil, err
		}
		lista = append(lista, a)
	}
	return lista, rows.Err()
}

// Mudanca traz só o que o dono mudou na tela (nil = não mexe).
type Mudanca struct {
	Apelido   *string
	Notas     *string
	Confiavel *bool
	VistoNovo bool // dono já olhou o aviso de aparelho novo
}

func (r *Repositorio) AtualizarDispositivo(ctx context.Context, id int64, m Mudanca) (Dispositivo, error) {
	d, err := lerDispositivo(r.pool.QueryRow(ctx, `
		UPDATE dispositivos SET
		  apelido = CASE WHEN $2::boolean THEN nullif($3, '') ELSE apelido END,
		  notas = CASE WHEN $4::boolean THEN nullif($5, '') ELSE notas END,
		  confiavel = coalesce($6, confiavel),
		  alertar_novo = alertar_novo AND NOT $7 AND NOT coalesce($6, false)
		WHERE id = $1 RETURNING `+colunasDispositivo,
		id, m.Apelido != nil, deref(m.Apelido), m.Notas != nil, deref(m.Notas), m.Confiavel, m.VistoNovo))
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNaoEncontrado
	}
	return d, err
}

// AparelhosNovos lista os que apareceram no período (para o histórico de eventos).
func (r *Repositorio) AparelhosNovos(ctx context.Context, de, ate time.Time) ([]Dispositivo, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+colunasDispositivo+` FROM dispositivos
		WHERE primeiro_visto >= $1 AND primeiro_visto < $2 AND NOT este_aparelho ORDER BY primeiro_visto DESC LIMIT 200`, de, ate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lista []Dispositivo
	for rows.Next() {
		d, err := lerDispositivo(rows)
		if err != nil {
			return nil, err
		}
		lista = append(lista, d)
	}
	return lista, rows.Err()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Movimento é um aparelho entrando ou saindo da rede, ou o próprio NetGuard
// parado (tipo "desligado", de Em até Ate, sem aparelho).
type Movimento struct {
	Tipo          string     `json:"tipo"` // entrou, saiu ou desligado
	Em            time.Time  `json:"em"`
	Ate           *time.Time `json:"ate,omitempty"`
	DispositivoID int64      `json:"dispositivo_id"`
	IP            string     `json:"ip"`
}

// cteParadas: buracos de mais de 2 min entre uma execução do monitor e a
// seguinte (PC desligado, sessão encerrada). Atualizar pelo menu leva segundos
// e não conta. $3 é a janela de ausência.
//
// Quem estava na rede até pouco antes de uma parada não "saiu": o NetGuard é
// que parou de olhar. Do mesmo jeito, quem já estava antes e responde nos
// primeiros minutos depois de religar não "entrou".
const cteParadas = `
	WITH ex AS (
	  SELECT fim, lead(inicio) OVER (ORDER BY inicio) AS prox FROM monitor_execucoes
	), paradas AS (
	  SELECT fim AS de, prox AS ate FROM ex WHERE prox > fim + interval '2 minutes'
	)`

// saidaNaParada e entradaAposParada recebem o alias da aparição ("a").
const saidaNaParada = `EXISTS (SELECT 1 FROM paradas p WHERE p.de >= a.fim AND p.de < a.fim + $3::interval)`

const entradaAposParada = `EXISTS (SELECT 1 FROM paradas p
	  WHERE a.inicio >= p.ate AND a.inicio < p.ate + $3::interval
	    AND EXISTS (SELECT 1 FROM aparicoes b WHERE b.dispositivo_id = a.dispositivo_id
	                AND b.fim <= p.de AND b.fim > p.de - $3::interval))`

// Movimentos lista entradas e saídas no período, da mais nova para a mais velha.
// "Saiu" só vale para aparição que terminou há mais que a janela (antes disso o
// aparelho ainda conta como presente: celular dormindo some e volta). Paradas do
// NetGuard aparecem como uma linha só, no lugar de todo mundo saindo e entrando.
func (r *Repositorio) Movimentos(ctx context.Context, de, ate time.Time, janela time.Duration, limite int) ([]Movimento, error) {
	rows, err := r.pool.Query(ctx, cteParadas+`
		SELECT tipo, em, ate, dispositivo_id, ip FROM (
		  SELECT 'entrou' AS tipo, a.inicio AS em, NULL::timestamptz AS ate, a.dispositivo_id, coalesce(host(a.ip), '') AS ip
		    FROM aparicoes a WHERE a.inicio >= $1 AND a.inicio < $2 AND NOT `+entradaAposParada+`
		  UNION ALL
		  SELECT 'saiu', a.fim, NULL, a.dispositivo_id, coalesce(host(a.ip), '')
		    FROM aparicoes a WHERE a.fim >= $1 AND a.fim < $2 AND a.fim < $4 AND NOT `+saidaNaParada+`
		  UNION ALL
		  SELECT 'desligado', p.de, p.ate, 0, '' FROM paradas p WHERE p.ate >= $1 AND p.de < $2
		) m ORDER BY em DESC, (tipo = 'saiu') DESC, dispositivo_id LIMIT $5`, de, ate, janela, ate.Add(-janela), limite)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lista []Movimento
	for rows.Next() {
		var m Movimento
		if err := rows.Scan(&m.Tipo, &m.Em, &m.Ate, &m.DispositivoID, &m.IP); err != nil {
			return nil, err
		}
		lista = append(lista, m)
	}
	return lista, rows.Err()
}

// Presenca resume quantas vezes um aparelho entrou e quanto tempo ficou num período.
type Presenca struct {
	Entradas int     `json:"entradas"`
	Segundos float64 `json:"segundos"`
}

// Presencas calcula a presença de todos os aparelhos de uma vez (para a lista).
// Conta como entrada cada aparição que começou no período (menos a volta depois
// de uma parada do NetGuard); o tempo soma só o pedaço de cada aparição que cai
// dentro dele.
func (r *Repositorio) Presencas(ctx context.Context, de, ate time.Time, janela time.Duration) (map[int64]Presenca, error) {
	rows, err := r.pool.Query(ctx, cteParadas+`
		SELECT dispositivo_id,
		  count(*) FILTER (WHERE inicio >= $1 AND NOT `+entradaAposParada+`),
		  coalesce(sum(extract(epoch FROM least(fim, $2) - greatest(inicio, $1))), 0)
		FROM aparicoes a WHERE fim >= $1 AND inicio < $2
		GROUP BY dispositivo_id`, de, ate, janela)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[int64]Presenca{}
	for rows.Next() {
		var id int64
		var p Presenca
		if err := rows.Scan(&id, &p.Entradas, &p.Segundos); err != nil {
			return nil, err
		}
		m[id] = p
	}
	return m, rows.Err()
}
