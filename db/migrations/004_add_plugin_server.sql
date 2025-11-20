-- ============================================
-- 插件服架构重新设计 v3.0
-- ============================================
-- 核心变更：
-- 1. TShock 从"房间类型"变为"全局唯一的插件服"
-- 2. rooms 表只允许 vanilla 和 tmodloader 类型
-- 3. plugin_server 表存储全局唯一的 TShock 插件服配置
-- 4. 插件服使用全局共享的 TShock 程序和插件目录
-- ============================================

-- Step 1: Create plugin_server table (independent from rooms table)
-- The plugin server is NOT a room, it's a global unique service
CREATE TABLE IF NOT EXISTS plugin_server (
    id INTEGER PRIMARY KEY CHECK (id = 1),  -- Only one record allowed (global unique)
    name TEXT NOT NULL DEFAULT 'TShock Plugin Server',
    port INTEGER NOT NULL DEFAULT 7777,     -- Fixed port (changed from 7778 to 7777)
    max_players INTEGER DEFAULT 8,
    password TEXT DEFAULT '',
    world_file TEXT DEFAULT 'plugin-test.wld',
    status TEXT DEFAULT 'stopped',          -- stopped, running
    pid INTEGER DEFAULT 0,
    start_time DATETIME,
    admin_token TEXT DEFAULT '',            -- TShock admin setup token
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Step 2: Insert default plugin server configuration
INSERT OR IGNORE INTO plugin_server (id, name, port, world_file)
VALUES (1, 'TShock Plugin Server', 7777, 'plugin-test.wld');

-- Step 3: Migration strategy for existing TShock rooms (optional)
-- If you have existing TShock rooms, you can choose:
-- A. Delete all TShock rooms (recommended)
--    DELETE FROM rooms WHERE server_type = 'tshock';
--
-- B. Or migrate the first TShock room's config to plugin server
--    UPDATE plugin_server
--    SET
--        name = (SELECT name FROM rooms WHERE server_type = 'tshock' LIMIT 1),
--        port = (SELECT port FROM rooms WHERE server_type = 'tshock' LIMIT 1),
--        max_players = (SELECT max_players FROM rooms WHERE server_type = 'tshock' LIMIT 1),
--        password = (SELECT password FROM rooms WHERE server_type = 'tshock' LIMIT 1)
--    WHERE id = 1 AND EXISTS (SELECT 1 FROM rooms WHERE server_type = 'tshock');
--
--    DELETE FROM rooms WHERE server_type = 'tshock';

-- Step 4: Architecture notes (for documentation)
-- The plugin server is a global shared TShock instance that:
-- 1. Uses the global TShock installation at data/servers/tshock/
-- 2. Shares the global plugin directory at data/servers/tshock/ServerPlugins/
-- 3. Does NOT copy the entire TShock directory (unlike regular TShock rooms)
-- 4. Runs on a fixed port (7777) to avoid conflicts
-- 5. Cannot be deleted by users (system reserved)
-- 6. Used for testing and managing plugins before copying to other rooms

-- Step 5: Constraint enforcement (must be done in application layer)
-- SQLite doesn't support ALTER TABLE ADD CONSTRAINT CHECK
-- The application (Go code) must enforce:
-- - rooms.server_type can only be 'vanilla' or 'tmodloader'
-- - No new TShock rooms can be created
-- - Only one plugin_server record can exist (enforced by CHECK constraint above)

