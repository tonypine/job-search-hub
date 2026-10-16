-- +goose Up
-- Two more kinds of provider: OpenRouter, an OpenAI-compatible service with
-- a key and a price per token, and the owner's Claude subscription, which
-- runs through the Claude CLI and has no address.
ALTER TABLE model_providers DROP CONSTRAINT model_providers_kind_check;
ALTER TABLE model_providers ADD CONSTRAINT model_providers_kind_check
    CHECK (kind IN ('openai_compatible', 'hub_runtime', 'openrouter', 'claude_subscription'));
ALTER TABLE model_providers DROP CONSTRAINT model_providers_base_url_check;
ALTER TABLE model_providers ADD CONSTRAINT model_providers_base_url_check
    CHECK (kind IN ('hub_runtime', 'claude_subscription') OR base_url ~ '^https?://');

-- A model call keeps what its provider said it cost, and the provider it
-- stood in for when it ran on its route's fallback. An agent run keeps the
-- model Claude Code was started with.
ALTER TABLE task_runs
    ADD COLUMN cost_usd     numeric CHECK (cost_usd >= 0),
    ADD COLUMN fallback_for text NOT NULL DEFAULT '';
ALTER TABLE agent_runs ADD COLUMN model text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE agent_runs DROP COLUMN model;
ALTER TABLE task_runs DROP COLUMN fallback_for, DROP COLUMN cost_usd;
DELETE FROM task_routes WHERE provider_id IN (SELECT id FROM model_providers WHERE kind IN ('openrouter', 'claude_subscription'));
DELETE FROM model_providers WHERE kind IN ('openrouter', 'claude_subscription');
ALTER TABLE model_providers DROP CONSTRAINT model_providers_base_url_check;
ALTER TABLE model_providers ADD CONSTRAINT model_providers_base_url_check
    CHECK (kind = 'hub_runtime' OR base_url ~ '^https?://');
ALTER TABLE model_providers DROP CONSTRAINT model_providers_kind_check;
ALTER TABLE model_providers ADD CONSTRAINT model_providers_kind_check CHECK (kind IN ('openai_compatible', 'hub_runtime'));
