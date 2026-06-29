-- 0002_refresh_tokens.sql — tokens de refresh para renovar o access token.
--
-- O access token (JWT) é curto (24h) e não-revogável por design. O refresh
-- token é opaco, de vida longa, guardado só pelo HASH (sha256) — nunca em claro,
-- como senha. É rotacionado a cada uso (/v1/auth/refresh): o antigo é revogado e
-- um novo é emitido, então um refresh roubado vale por uma janela curta e some
-- assim que o dono renovar. Escopado por user_id, cai junto com a conta.

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,           -- sha256 hex do token opaco
    expires_at TIMESTAMPTZ NOT NULL,
    revoked    BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);
