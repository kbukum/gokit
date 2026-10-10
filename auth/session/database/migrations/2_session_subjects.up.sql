CREATE INDEX auth_session_families_subject ON auth_session_families (subject, kind) WHERE revoked = FALSE;
