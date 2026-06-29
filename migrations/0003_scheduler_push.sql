-- 0003_scheduler_push.sql — fuso por usuário + outbox de entregas de alerta.
--
-- Scheduler server-side: o servidor dispara os alertas (mesmo com o desktop
-- fechado) avançando recorrência no fuso do dono. A entrega é desacoplada do
-- disparo por um outbox (alert_deliveries): cada disparo vira uma linha; o push
-- ao vivo (SSE) entrega na hora e marca delivered_at, e um cliente que estava
-- offline recebe as pendentes ao reconectar. Assim nenhum disparo se perde.
--
-- user_channels (preferência de canal p/ WhatsApp/Telegram) fica p/ a fase 4,
-- quando existir um 2º canal — com só o desktop (SSE) não há o que preferir.

-- Fuso do usuário, usado pelo scheduler p/ avançar recorrência na parede certa.
ALTER TABLE user_prefs ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'UTC';

CREATE TABLE IF NOT EXISTS alert_deliveries (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    alert_id     BIGINT REFERENCES alerts(id) ON DELETE SET NULL,
    message      TEXT NOT NULL,
    note_id      BIGINT,
    fire_at      TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at TIMESTAMPTZ                       -- NULL = ainda não entregue
);
-- Índice parcial: o flush no connect só varre as pendentes do usuário.
CREATE INDEX IF NOT EXISTS idx_alert_deliveries_pending
    ON alert_deliveries(user_id) WHERE delivered_at IS NULL;
