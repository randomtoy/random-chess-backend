package db

import "embed"

// Migrations holds the embedded SQL migration files.
//
//go:embed migrations/*.sql
var Migrations embed.FS
