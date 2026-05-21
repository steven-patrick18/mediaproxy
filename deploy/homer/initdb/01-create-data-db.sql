-- The postgres image only auto-creates the database named in POSTGRES_DB.
-- HOMER 7 needs two: homer_config (metadata, users, settings) and
-- homer_data (the actual SIP captures). This init script runs once when
-- the homer-db volume is empty, creating the second one.
CREATE DATABASE homer_data OWNER homer;
