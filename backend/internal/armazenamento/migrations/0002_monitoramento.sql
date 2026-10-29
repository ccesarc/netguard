-- Fase 2: estado de cada ciclo, quedas e cobertura do monitor.

-- Um por ciclo: base da disponibilidade.
CREATE TABLE ciclos (
    id      bigserial PRIMARY KEY,
    em      timestamptz NOT NULL,
    estado  text NOT NULL CHECK (estado IN ('ok', 'degradado', 'fora')),
    causa   text NOT NULL,
    detalhe text
);
CREATE INDEX ciclos_em ON ciclos (em DESC);

-- Períodos em que o monitor estava rodando. Fora deles = "sem dados", não queda.
CREATE TABLE monitor_execucoes (
    id     bigserial PRIMARY KEY,
    inicio timestamptz NOT NULL,
    fim    timestamptz NOT NULL,
    versao text
);
CREATE INDEX monitor_execucoes_fim ON monitor_execucoes (fim DESC);

CREATE TABLE incidentes (
    id      bigserial PRIMARY KEY,
    inicio  timestamptz NOT NULL,
    fim     timestamptz,
    causa   text NOT NULL,           -- causa mais recente
    causas  text[] NOT NULL DEFAULT '{}', -- todas as causas vistas, na ordem
    detalhe text,
    CHECK (fim IS NULL OR fim >= inicio)
);
CREATE INDEX incidentes_inicio ON incidentes (inicio DESC);
-- No máximo uma queda aberta por vez.
CREATE UNIQUE INDEX incidentes_um_aberto ON incidentes ((true)) WHERE fim IS NULL;

CREATE INDEX verificacoes_em ON verificacoes (em);
