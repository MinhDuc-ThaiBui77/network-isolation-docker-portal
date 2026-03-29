CREATE TABLE IF NOT EXISTS events (
    id         SERIAL PRIMARY KEY,
    agent_id   INT          NOT NULL,
    command    VARCHAR(50)  NOT NULL,
    payload    TEXT         NOT NULL DEFAULT '',
    response   TEXT         NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_events_agent_id ON events (agent_id);
CREATE INDEX idx_events_created_at ON events (created_at);
