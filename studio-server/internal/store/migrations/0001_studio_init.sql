CREATE TABLE IF NOT EXISTS "studioProjects" (
    "id" varchar(36) PRIMARY KEY,
    "revision" integer NOT NULL DEFAULT 1,
    "state" text NOT NULL
);

CREATE TABLE IF NOT EXISTS "studioFiles" (
    "id" varchar(36) PRIMARY KEY,
    "projectId" varchar(36) NOT NULL REFERENCES "studioProjects"("id") ON DELETE CASCADE,
    "checksum" varchar(64) NOT NULL,
    "name" varchar(255) NOT NULL,
    "createdAt" varchar(64) NOT NULL,
    "data" bytea NOT NULL,
    CONSTRAINT "studiofiles_projectid_checksum_unique" UNIQUE ("projectId", "checksum")
);

CREATE TABLE IF NOT EXISTS "studioServiceKeys" (
    "id" varchar(36) PRIMARY KEY,
    "name" varchar(120) NOT NULL,
    "role" varchar(24) NOT NULL,
    "keyHash" varchar(64) NOT NULL UNIQUE,
    "groupsJson" text NOT NULL DEFAULT '[]',
    "createdAt" varchar(64) NOT NULL,
    "createdBy" varchar(255) NOT NULL DEFAULT ''
);
