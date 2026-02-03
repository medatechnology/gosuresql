package client

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const MIGRATION_TABLE = "_client_migrations"

// MigrationService handles database migrations
type MigrationService struct {
	client *Client
}

// NewMigrationService creates a new migration service
func NewMigrationService(client *Client) *MigrationService {
	return &MigrationService{client: client}
}

// Migrate scans the provided directory for .sql files and applies them
// if they haven't been applied yet.
func (m *MigrationService) Migrate(dir string) error {
	// 1. Ensure migration table exists
	err := m.ensureMigrationTable()
	if err != nil {
		return fmt.Errorf("failed to ensure migration table: %w", err)
	}

	// 2. Read migration files
	files, err := m.readMigrationFiles(dir)
	if err != nil {
		return fmt.Errorf("failed to read migration files: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("No migration files found.")
		return nil
	}

	// 3. Get applied migrations
	applied, err := m.getAppliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// 4. Apply pending migrations
	for _, file := range files {
		// Check if already applied
		if _, exists := applied[file.Name]; exists {
			continue
		}

		fmt.Printf("Applying migration: %s... ", file.Name)
		err := m.applyMigration(file)
		if err != nil {
			fmt.Printf("FAILED\n")
			return fmt.Errorf("failed to apply migration %s: %w", file.Name, err)
		}
		fmt.Printf("OK\n")
	}

	return nil
}

// ensureMigrationTable creates the tracking table if it doesn't exist
func (m *MigrationService) ensureMigrationTable() error {
	sql := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`, MIGRATION_TABLE)

	res := m.client.ExecOneSQL(sql)
	if res.Error != nil {
		return res.Error
	}
	return nil
}

type migrationFile struct {
	Name    string
	Path    string
	Content string
}

// readMigrationFiles reads and strictly sorts SQL files
func (m *MigrationService) readMigrationFiles(dir string) ([]migrationFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []migrationFile
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		if !entry.IsDir() && strings.HasSuffix(name, ".sql") && !strings.HasSuffix(name, ".down.sql") {
			path := filepath.Join(dir, entry.Name())
			content, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}

			files = append(files, migrationFile{
				Name:    entry.Name(),
				Path:    path,
				Content: string(content),
			})
		}
	}

	// Sort by name (versions should be prefixed like 00001, 00002)
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})

	return files, nil
}

// getAppliedMigrations returns a set of applied migration names
func (m *MigrationService) getAppliedMigrations() (map[string]bool, error) {
	// Structure to match query result
	type MigrationRecord struct {
		Name string `json:"name"`
	}

	sql := fmt.Sprintf("SELECT name FROM %s", MIGRATION_TABLE)
	// We use SelectManySQL to get raw records map, or if we had a struct we could use that.
	// SelectManySQL returns []orm.DBRecords.
	
	// Since we don't have a ready-made struct mapped in the library for this internal table,
	// let's use SelectManySQL (generic) and parse manually.
	
	// Note: SelectManySQL returns ([]orm.DBRecords, error)
	// orm.DBRecords is []orm.DBRecord
	// orm.DBRecord is struct { Data map[string]interface{} }
	
	result, err := m.client.SelectOneSQL(sql)
	if err != nil {
		// If table doesn't exist yet (edge case where ensure failed but didn't error?), return empty
        // But ensureMigrationTable should have handled it.
        // Actually, if no rows found (empty table), it returns ErrSQLNoRows.
        if err.Error() == "sql: no rows in result set" || strings.Contains(err.Error(), "no rows") {
             return map[string]bool{}, nil
        }
		return nil, nil // Assume empty if error is "no rows" 
	}

	applied := make(map[string]bool)
	for _, rec := range result {
		if name, ok := rec.Data["name"].(string); ok {
			applied[name] = true
		}
	}
	return applied, nil
}

// applyMigration executes the SQL content and records it
func (m *MigrationService) applyMigration(file migrationFile) error {
	// 1. Execute the migration SQL
    // We execute it as a single batch if possible, or statement by statement?
    // ExecOneSQL takes a string. Ideally transactions support.
    
	res := m.client.ExecOneSQL(file.Content)
	if res.Error != nil {
		return res.Error
	}

	// 2. Record it
	insertSQL := fmt.Sprintf("INSERT INTO %s (name) VALUES ('%s')", MIGRATION_TABLE, file.Name)
	res = m.client.ExecOneSQL(insertSQL)
	if res.Error != nil {
		return fmt.Errorf("failed to record migration: %v", res.Error)
	}

	return nil
}
