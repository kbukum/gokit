CREATE TABLE auth_session_families (
    id VARCHAR(64) PRIMARY KEY,
    current_reference VARCHAR(64) NOT NULL UNIQUE,
    generation BIGINT NOT NULL,
    subject TEXT NOT NULL,
    kind VARCHAR(16) NOT NULL,
    restrictions TEXT NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    retain_until TIMESTAMP NOT NULL,
    revoked BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX auth_session_families_retention ON auth_session_families (retain_until, id);
CREATE TABLE auth_session_generations (
    reference VARCHAR(64) PRIMARY KEY,
    family VARCHAR(64) NOT NULL REFERENCES auth_session_families(id),
    generation BIGINT NOT NULL,
    retain_until TIMESTAMP NOT NULL,
    UNIQUE (family, generation)
);
CREATE INDEX auth_session_generations_retention ON auth_session_generations (retain_until, reference);
