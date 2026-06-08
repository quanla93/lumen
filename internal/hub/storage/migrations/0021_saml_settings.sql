-- +goose Up
-- SAML2 SSO (RFC 0002) — single-admin scope. No new table; the
-- operator-facing config lives in the existing settings k/v table.
-- Migration seeds defaults so the Settings UI can render placeholder
-- fields from a fresh install; existing operator values survive via
-- INSERT OR IGNORE.
--
-- Sensitive values:
--   saml.sp_private_key_enc  — RSA private key (PEM), AES-GCM with
--                               the SAML-distinct KEK label
--                               (auth/saml_crypto.go: "lumen/saml/v1").
--   saml.idp_metadata_xml    — IdP metadata XML. Stored verbatim so
--                               a URL refetch is an explicit
--                               operator action. Not encrypted:
--                               it's a public key/cert bundle.
INSERT OR IGNORE INTO settings (key, value) VALUES
    ('saml.enabled',                    'false'),
    ('saml.idp_metadata_xml',           ''),
    ('saml.idp_metadata_url',           ''),
    ('saml.sp_entity_id',               ''),
    ('saml.expected_nameid',            ''),
    ('saml.sp_private_key_enc',         ''),
    ('saml.sp_cert',                    ''),
    ('saml.allowed_clock_skew_seconds', '60');

-- +goose Down
DELETE FROM settings WHERE key LIKE 'saml.%';
