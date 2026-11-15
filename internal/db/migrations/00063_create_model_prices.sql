-- +goose Up
-- Prices entered in the dashboard, for the models no provider catalogue prices.
-- A catalogue is still the authority: this table is only consulted for models
-- that no source knows, so a provider that starts publishing a rate takes over
-- without anyone deleting the row here.
CREATE TABLE model_prices (
    id              BIGSERIAL           PRIMARY KEY,
    model           TEXT                NOT NULL,
    -- The cost lookup treats model names case-insensitively, so two rows that
    -- differ only in capitalisation would fight over the same price. Keying the
    -- uniqueness on the folded name makes that impossible.
    model_key       TEXT                GENERATED ALWAYS AS (lower(model)) STORED,
    input_per_mtok  DOUBLE PRECISION    NOT NULL DEFAULT 0,
    output_per_mtok DOUBLE PRECISION    NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ         NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ         NOT NULL DEFAULT now(),
    CONSTRAINT model_prices_rates_non_negative CHECK (input_per_mtok >= 0 AND output_per_mtok >= 0),
    -- A row with two zero rates would price a call at nothing, which reads as a
    -- free model instead of a missing price.
    CONSTRAINT model_prices_rates_present CHECK (input_per_mtok > 0 OR output_per_mtok > 0),
    CONSTRAINT model_prices_model_unique UNIQUE (model_key)
);

-- +goose Down
DROP TABLE IF EXISTS model_prices;
