-- Fase 3: aparelhos observados na rede e quando cada um esteve presente.

CREATE TABLE dispositivos (
    id              bigserial PRIMARY KEY,
    mac             macaddr NOT NULL UNIQUE,
    ip              inet,
    nome_descoberto text,              -- DNS reverso no roteador (nome que o aparelho deu no DHCP)
    apelido         text CHECK (apelido IS NULL OR length(apelido) <= 60),
    notas           text CHECK (notas IS NULL OR length(notas) <= 500),
    fabricante      text,
    mac_aleatorio   boolean NOT NULL DEFAULT false,
    confiavel       boolean NOT NULL DEFAULT false,
    este_aparelho   boolean NOT NULL DEFAULT false,
    -- aparelho que apareceu depois da primeira coleta: merece aviso até o dono olhar
    alertar_novo    boolean NOT NULL DEFAULT false,
    primeiro_visto  timestamptz NOT NULL,
    ultimo_visto    timestamptz NOT NULL
);
CREATE INDEX dispositivos_ultimo_visto ON dispositivos (ultimo_visto DESC);

-- Intervalos de presença: um novo começa quando o aparelho volta depois de sumir.
CREATE TABLE aparicoes (
    id             bigserial PRIMARY KEY,
    dispositivo_id bigint NOT NULL REFERENCES dispositivos(id) ON DELETE CASCADE,
    inicio         timestamptz NOT NULL,
    fim            timestamptz NOT NULL,
    ip             inet,
    CHECK (fim >= inicio)
);
CREATE INDEX aparicoes_dispositivo_fim ON aparicoes (dispositivo_id, fim DESC);
CREATE INDEX aparicoes_fim ON aparicoes (fim);
