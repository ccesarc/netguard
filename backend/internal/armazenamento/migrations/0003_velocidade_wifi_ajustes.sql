-- Fase 2B: configurações editáveis pela tela, teste de velocidade e Wi-Fi.

CREATE TABLE configuracao (
    chave          text PRIMARY KEY,
    valor          jsonb NOT NULL,
    atualizado_em  timestamptz NOT NULL DEFAULT now(),
    atualizado_por text
);

CREATE TABLE testes_velocidade (
    id             bigserial PRIMARY KEY,
    em             timestamptz NOT NULL,
    origem         text NOT NULL CHECK (origem IN ('agendado', 'manual')),
    download_mbps  real,
    upload_mbps    real,
    latencia_ms    real,
    bytes_baixados bigint NOT NULL DEFAULT 0,
    bytes_enviados bigint NOT NULL DEFAULT 0,
    duracao_s      real,
    servidor       text,
    interface      text,
    erro           text
);
CREATE INDEX testes_velocidade_em ON testes_velocidade (em DESC);

CREATE TABLE wifi_amostras (
    em                timestamptz NOT NULL,
    ssid              text,
    sinal             smallint CHECK (sinal BETWEEN 0 AND 100),
    canal             smallint,
    freq_mhz          integer,
    taxa_mbps         integer,
    redes_mesmo_canal smallint
);
CREATE INDEX wifi_amostras_em ON wifi_amostras (em);
