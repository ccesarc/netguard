package armazenamento

import (
	"context"
	"time"
)

// ResumoRede resume as verificações de um período para a tela Saúde da rede.
type ResumoRede struct {
	LatenciaMediana  *float64 // até os IPs de fora, só os que responderam
	PerdaMedia       *float64
	DNSSistemaMS     *float64
	DNSExternoMS     *float64
	DNSSistemaFalhas *float64 // fração das consultas que falharam
	DNSExternoFalhas *float64
	Amostras         int // verificações de IP de fora no período
}

func (r *Repositorio) ResumoRede(ctx context.Context, de, ate time.Time) (ResumoRede, error) {
	var x ResumoRede
	err := r.pool.QueryRow(ctx, `
		SELECT
		  percentile_cont(0.5) WITHIN GROUP (ORDER BY latencia_ms) FILTER (WHERE tipo = 'externo' AND ok),
		  avg(perda) FILTER (WHERE tipo = 'externo'),
		  avg(latencia_ms) FILTER (WHERE tipo = 'dns_sistema' AND ok),
		  avg(latencia_ms) FILTER (WHERE tipo = 'dns_externo' AND ok),
		  avg((NOT ok)::int) FILTER (WHERE tipo = 'dns_sistema'),
		  avg((NOT ok)::int) FILTER (WHERE tipo = 'dns_externo'),
		  count(*) FILTER (WHERE tipo = 'externo')
		FROM verificacoes WHERE em >= $1 AND em < $2`, de, ate).Scan(
		&x.LatenciaMediana, &x.PerdaMedia, &x.DNSSistemaMS, &x.DNSExternoMS,
		&x.DNSSistemaFalhas, &x.DNSExternoFalhas, &x.Amostras)
	return x, err
}
