-- Saúde da rede: resposta da internet durante o teste de velocidade ("fila cheia" / bufferbloat).
ALTER TABLE testes_velocidade
    ADD COLUMN latencia_download_ms real,
    ADD COLUMN latencia_upload_ms real;
