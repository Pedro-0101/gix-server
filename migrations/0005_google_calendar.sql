-- 0005_google_calendar.sql — integração com Google Calendar.
--
-- Cada alerta pode ter um evento correspondente no Google Calendar do usuário,
-- rastreado por google_calendar_event_id. Os tokens OAuth2 vivem em google_tokens.

ALTER TABLE alerts ADD COLUMN IF NOT EXISTS google_calendar_event_id TEXT;

CREATE TABLE IF NOT EXISTS google_tokens (
    user_id       BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    access_token  TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    expires_at    TIMESTAMPTZ NOT NULL
);
