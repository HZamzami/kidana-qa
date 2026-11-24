-- Runs once, when the Postgres volume is first created.
-- A separate database for integration tests so `just test` never touches dev data.
CREATE DATABASE kidana_quality_test OWNER kidana;
