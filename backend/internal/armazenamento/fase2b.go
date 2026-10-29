package armazenamento

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"netguard/internal/ajustes"
	"netguard/internal/velocidade"
	"netguard/internal/wifi"
)

// ---- configurações ----

const chaveAjustes = "ajustes"

// LerAjustes devolve o que foi salvo pela tela; ok=false se nunca salvaram.
func (r *Repositorio) LerAjustes(ctx context.Context) (ajustes.Ajustes, bool, error) {
	var bruto []byte
	err := r.pool.QueryRow(ctx, "SELECT valor FROM configuracao WHERE chave = $1", chaveAjustes).Scan(&bruto)
	if errors.Is(err, pgx.ErrNoRows) {
		return ajustes.Ajustes{}, false, nil
	}
	if err != nil {
		return ajustes.Ajustes{}, false, err
	}
	var a ajustes.Ajustes
	if err := json.Unmarshal(bruto, &a); err != nil {
		return ajustes.Ajustes{}, false, err
	}
	return a, true, nil
}

func (r *Repositorio) SalvarAjustes(ctx context.Context, a ajustes.Ajustes, por string) error {
	bruto, err := json.Marshal(a)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO configuracao (chave, valor, atualizado_por) VALUES ($1, $2, $3)
		ON CONFLICT (chave) DO UPDATE SET valor = excluded.valor, atualizado_em = now(), atualizado_por = excluded.atualizado_por`,
		chaveAjustes, bruto, vazioNulo(por))
	return err
}

// ---- teste de velocidade ----

func (r *Repositorio) SalvarTesteVelocidade(ctx context.Context, v velocidade.Resultado) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO testes_velocidade (em, origem, download_mbps, upload_mbps, latencia_ms, bytes_baixados,
		  bytes_enviados, duracao_s, servidor, interface, erro, latencia_download_ms, latencia_upload_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		v.Em, v.Origem, v.DownloadMbps, v.UploadMbps, v.LatenciaMS, v.BytesBaixados, v.BytesEnviados,
		v.DuracaoS, vazioNulo(v.Servidor), vazioNulo(v.Interface), vazioNulo(v.Erro), v.LatenciaDownloadMS, v.LatenciaUploadMS)
	return err
}

// TestesVelocidade lista do mais antigo para o mais novo (ordem do gráfico).
func (r *Repositorio) TestesVelocidade(ctx context.Context, de, ate time.Time) ([]velocidade.Resultado, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT em, origem, download_mbps, upload_mbps, latencia_ms, bytes_baixados, bytes_enviados,
		  coalesce(duracao_s, 0), coalesce(servidor, ''), coalesce(interface, ''), coalesce(erro, ''),
		  latencia_download_ms, latencia_upload_ms
		FROM testes_velocidade WHERE em >= $1 AND em < $2 ORDER BY em LIMIT 2000`, de, ate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lista []velocidade.Resultado
	for rows.Next() {
		var v velocidade.Resultado
		var down, up, lat, latD, latU *float32
		var dur float32
		if err := rows.Scan(&v.Em, &v.Origem, &down, &up, &lat, &v.BytesBaixados, &v.BytesEnviados,
			&dur, &v.Servidor, &v.Interface, &v.Erro, &latD, &latU); err != nil {
			return nil, err
		}
		v.DownloadMbps, v.UploadMbps, v.LatenciaMS, v.DuracaoS = f64(down), f64(up), f64(lat), float64(dur)
		v.LatenciaDownloadMS, v.LatenciaUploadMS = f64(latD), f64(latU)
		lista = append(lista, v)
	}
	return lista, rows.Err()
}

// BytesTesteVelocidade soma o que os testes gastaram no período.
func (r *Repositorio) BytesTesteVelocidade(ctx context.Context, de, ate time.Time) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx,
		"SELECT coalesce(sum(bytes_baixados + bytes_enviados), 0) FROM testes_velocidade WHERE em >= $1 AND em < $2",
		de, ate).Scan(&n)
	return n, err
}

func (r *Repositorio) UltimoTesteVelocidade(ctx context.Context) (*velocidade.Resultado, error) {
	lista, err := r.TestesVelocidade(ctx, time.Now().AddDate(0, 0, -30), time.Now().Add(time.Minute))
	if err != nil || len(lista) == 0 {
		return nil, err
	}
	return &lista[len(lista)-1], nil
}

// ---- Wi-Fi ----

func (r *Repositorio) SalvarWifi(ctx context.Context, a wifi.Amostra) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO wifi_amostras (em, ssid, sinal, canal, freq_mhz, taxa_mbps, redes_mesmo_canal)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		a.Em, a.SSID, a.Sinal, a.Canal, a.FreqMHz, a.TaxaMbps, a.RedesMesmoCanal)
	return err
}

type PontoWifi struct {
	Em       time.Time
	Sinal    *float64 // média
	SinalMin *float64
	TaxaMbps *float64
}

func (r *Repositorio) HistoricoWifi(ctx context.Context, de, ate time.Time, passo time.Duration) ([]PontoWifi, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT date_bin(make_interval(secs => $3), em, TIMESTAMPTZ '2000-01-01') AS b,
		  avg(sinal), min(sinal)::float8, avg(taxa_mbps)
		FROM wifi_amostras WHERE em >= $1 AND em < $2 GROUP BY b ORDER BY b`,
		de, ate, int(passo.Seconds()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pts []PontoWifi
	for rows.Next() {
		var p PontoWifi
		if err := rows.Scan(&p.Em, &p.Sinal, &p.SinalMin, &p.TaxaMbps); err != nil {
			return nil, err
		}
		pts = append(pts, p)
	}
	return pts, rows.Err()
}

func f64(v *float32) *float64 {
	if v == nil {
		return nil
	}
	x := float64(int64(float64(*v)*10+0.5)) / 10
	return &x
}
