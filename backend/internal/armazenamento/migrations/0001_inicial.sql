-- Fase 1: login, sessões, auditoria e o resultado bruto das verificações.

CREATE TABLE usuarios (
    id            bigserial PRIMARY KEY,
    nome          text NOT NULL UNIQUE CHECK (nome ~ '^[a-z0-9._-]{3,32}$'),
    senha_hash    text NOT NULL,
    criado_em     timestamptz NOT NULL DEFAULT now(),
    atualizado_em timestamptz NOT NULL DEFAULT now()
);

-- Só o hash SHA-256 do token fica aqui.
CREATE TABLE sessoes (
    token_hash bytea PRIMARY KEY,
    usuario_id bigint NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
    criada_em  timestamptz NOT NULL DEFAULT now(),
    expira_em  timestamptz NOT NULL,
    ip         text,
    agente     text
);
CREATE INDEX sessoes_expira_em ON sessoes (expira_em);

CREATE TABLE auditoria (
    id         bigserial PRIMARY KEY,
    em         timestamptz NOT NULL DEFAULT now(),
    usuario_id bigint REFERENCES usuarios(id) ON DELETE SET NULL,
    usuario    text,
    acao       text NOT NULL,
    alvo       text,
    detalhe    jsonb,
    ip         text
);
CREATE INDEX auditoria_em ON auditoria (em DESC);

-- Uma linha por sonda por ciclo. Limpeza e resumos chegam na fase 2.
CREATE TABLE verificacoes (
    id          bigserial PRIMARY KEY,
    em          timestamptz NOT NULL,
    tipo        text NOT NULL,
    alvo        text NOT NULL,
    ok          boolean NOT NULL,
    latencia_ms real,
    perda       real CHECK (perda IS NULL OR (perda >= 0 AND perda <= 1)),
    erro        text,
    dados       jsonb
);
CREATE INDEX verificacoes_tipo_em ON verificacoes (tipo, em DESC);
