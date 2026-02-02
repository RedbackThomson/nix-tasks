# Legacy Makefile - demonstrating migration to nix-tasks
# These targets can be wrapped using lib.compat.make.mkMakeTask
# or lib.compat.make.importMakeTargets

.PHONY: legacy-migrate legacy-seed

legacy-migrate:
	@echo "Running legacy database migrations..."
	@echo "Migration 001: Create users table"
	@echo "Migration 002: Create orders table"
	@echo "Migration 003: Add indexes"
	@echo "Migrations complete!"

legacy-seed:
	@echo "Seeding database with test data..."
	@echo "Creating test users..."
	@echo "Creating test orders..."
	@echo "Seed complete!"
