-- 0006_gcal_pref.sql — preferência de sincronização com Google Calendar.
--
-- Toggle global: ligado = todo alerta novo vai pro Calendar; desligado = nenhum.

ALTER TABLE user_prefs ADD COLUMN IF NOT EXISTS gcal_sync_enabled BOOLEAN NOT NULL DEFAULT false;
