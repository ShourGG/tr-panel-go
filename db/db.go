package db
import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	_ "github.com/glebarez/go-sqlite"
)
//go:embed schema.sql
var schemaSQL string
var DB *sql.DB
func Init(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	var err error
	DB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(5)
	if _, err := DB.Exec(schemaSQL); err != nil {
		return err
	}
	if err := migrateDatabase(); err != nil {
		log.Printf("⚠️ 数据库迁移警告: %v", err)
	}
	if err := applyPerformanceIndexes(); err != nil {
		log.Printf("⚠️ 性能索引创建警告: %v", err)
	}
	log.Println("✅ 数据库初始化成功:", dbPath)
	return nil
}
func migrateDatabase() error {
	migrations := []string{
		"ALTER TABLE rooms ADD COLUMN world_size TEXT DEFAULT 'medium'",
		"ALTER TABLE rooms ADD COLUMN difficulty TEXT DEFAULT 'normal'",
		"ALTER TABLE rooms ADD COLUMN evil_type TEXT DEFAULT 'corruption'",
		"ALTER TABLE rooms ADD COLUMN start_time DATETIME",
		"ALTER TABLE rooms ADD COLUMN admin_token TEXT",
		"ALTER TABLE players ADD COLUMN room_id INTEGER DEFAULT 0",
		"ALTER TABLE players ADD COLUMN status TEXT DEFAULT 'offline'",
	}
	for _, migration := range migrations {
		if _, err := DB.Exec(migration); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") &&
				!strings.Contains(err.Error(), "already exists") {
				log.Printf("迁移执行失败（可能列已存在）: %v", err)
			}
		}
	}
	if err := ensurePluginServerTable(); err != nil {
		log.Printf("⚠️ 插件服表迁移失败: %v", err)
	}
	if err := addServerModeColumn(); err != nil {
		log.Printf("⚠️ server_mode 字段添加失败: %v", err)
	}
	log.Println("✅ 数据库迁移检查完成")
	return nil
}
func addServerModeColumn() error {
	_, err := DB.Exec("ALTER TABLE users ADD COLUMN server_mode TEXT DEFAULT 'rooms'")
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	log.Println("✅ users.server_mode 字段检查完成")
	return nil
}
func ensurePluginServerTable() error {
	var tableName string
	err := DB.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='plugin_server'").Scan(&tableName)
	if err == sql.ErrNoRows {
		log.Println("📦 创建 plugin_server 表...")
		createTableSQL := `
			CREATE TABLE IF NOT EXISTS plugin_server (
				id INTEGER PRIMARY KEY CHECK (id = 1),
				name TEXT NOT NULL DEFAULT 'TShock Plugin Server',
				port INTEGER NOT NULL DEFAULT 7777,
				max_players INTEGER DEFAULT 8,
				password TEXT DEFAULT '',
				world_file TEXT DEFAULT 'plugin-test.wld',
				status TEXT DEFAULT 'stopped',
				pid INTEGER DEFAULT 0,
				start_time DATETIME,
				admin_token TEXT DEFAULT '',
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				world_size INTEGER DEFAULT 2,
				world_name TEXT DEFAULT 'Plugin Test World',
				difficulty INTEGER DEFAULT 0,
				seed TEXT DEFAULT '',
				world_evil TEXT DEFAULT 'random',
				server_name TEXT DEFAULT 'TShock Plugin Server'
			)
		`
		if _, err := DB.Exec(createTableSQL); err != nil {
			return err
		}
		log.Println("✅ plugin_server 表创建成功")
	} else {
		log.Println("📦 检查 plugin_server 表字段...")
		addColumnIfNotExists("plugin_server", "world_size", "INTEGER DEFAULT 2")
		addColumnIfNotExists("plugin_server", "world_name", "TEXT DEFAULT 'Plugin Test World'")
		addColumnIfNotExists("plugin_server", "difficulty", "INTEGER DEFAULT 0")
		addColumnIfNotExists("plugin_server", "seed", "TEXT DEFAULT ''")
		addColumnIfNotExists("plugin_server", "world_evil", "TEXT DEFAULT 'random'")
		addColumnIfNotExists("plugin_server", "server_name", "TEXT DEFAULT 'TShock Plugin Server'")
	}
	var count int
	err = DB.QueryRow("SELECT COUNT(*) FROM plugin_server WHERE id = 1").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		log.Println("📦 插入默认插件服配置...")
		insertSQL := `
			INSERT INTO plugin_server (
				id, name, port, world_file,
				world_size, world_name, difficulty,
				seed, world_evil, server_name
			)
			VALUES (
				1, 'TShock Plugin Server', 7777, 'plugin-test.wld',
				2, 'Plugin Test World', 0,
				'', 'random', 'TShock Plugin Server'
			)
		`
		if _, err := DB.Exec(insertSQL); err != nil {
			return err
		}
		log.Println("✅ 默认插件服配置插入成功")
	}
	return nil
}
func addColumnIfNotExists(tableName, columnName, columnDef string) {
	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name='%s'", tableName, columnName)
	err := DB.QueryRow(query).Scan(&count)
	if err != nil || count == 0 {
		alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, columnDef)
		if _, err := DB.Exec(alterSQL); err != nil {
			log.Printf("⚠️  添加字段 %s.%s 失败: %v", tableName, columnName, err)
		} else {
			log.Printf("✅ 添加字段 %s.%s 成功", tableName, columnName)
		}
	}
}
func applyPerformanceIndexes() error {
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_player_sessions_player_time ON player_sessions(player_id, join_time DESC)",
		"CREATE INDEX IF NOT EXISTS idx_player_sessions_room_time ON player_sessions(room_id, join_time DESC)",
		"CREATE INDEX IF NOT EXISTS idx_player_stats_updated ON player_stats(updated_at DESC)",
		"CREATE INDEX IF NOT EXISTS idx_activity_logs_type_time ON activity_logs(type, created_at DESC)",
		"CREATE INDEX IF NOT EXISTS idx_task_execution_status ON task_execution_logs(status, started_at DESC)",
	}
	for _, indexSQL := range indexes {
		if _, err := DB.Exec(indexSQL); err != nil {
			log.Printf("⚠️ 索引创建失败: %v", err)
		}
	}
	log.Println("✅ 性能索引创建完成")
	return nil
}

func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
