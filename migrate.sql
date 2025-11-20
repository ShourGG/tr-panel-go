-- 玩家统计功能数据库迁移脚本
-- 执行方法：sqlite3 /root/terraria-panel/data/panel.db < migrate.sql

-- 1. 添加 players 表的新字段
ALTER TABLE players ADD COLUMN room_id INTEGER DEFAULT 0;
ALTER TABLE players ADD COLUMN status TEXT DEFAULT 'offline';

-- 2. 创建索引
CREATE INDEX IF NOT EXISTS idx_players_room_id ON players(room_id);
CREATE INDEX IF NOT EXISTS idx_players_status ON players(status);

-- 3. 创建玩家会话记录表
CREATE TABLE IF NOT EXISTS player_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    player_id INTEGER NOT NULL,
    room_id INTEGER NOT NULL,
    join_time DATETIME NOT NULL,
    leave_time DATETIME,
    duration INTEGER DEFAULT 0,
    ip_address TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (player_id) REFERENCES players(id) ON DELETE CASCADE,
    FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE
);

-- 4. 创建玩家统计数据表
CREATE TABLE IF NOT EXISTS player_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    player_id INTEGER NOT NULL UNIQUE,
    total_play_time INTEGER DEFAULT 0,
    login_count INTEGER DEFAULT 0,
    last_login_time DATETIME,
    last_logout_time DATETIME,
    first_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (player_id) REFERENCES players(id) ON DELETE CASCADE
);

-- 5. 创建每日统计表
CREATE TABLE IF NOT EXISTS player_daily_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date DATE NOT NULL,
    total_players INTEGER DEFAULT 0,
    active_players INTEGER DEFAULT 0,
    new_players INTEGER DEFAULT 0,
    total_play_time INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 6. 创建所有索引
CREATE INDEX IF NOT EXISTS idx_player_sessions_player_id ON player_sessions(player_id);
CREATE INDEX IF NOT EXISTS idx_player_sessions_room_id ON player_sessions(room_id);
CREATE INDEX IF NOT EXISTS idx_player_sessions_join_time ON player_sessions(join_time);
CREATE INDEX IF NOT EXISTS idx_player_stats_player_id ON player_stats(player_id);
CREATE INDEX IF NOT EXISTS idx_player_stats_total_play_time ON player_stats(total_play_time);
CREATE INDEX IF NOT EXISTS idx_player_stats_login_count ON player_stats(login_count);
CREATE INDEX IF NOT EXISTS idx_player_stats_last_login_time ON player_stats(last_login_time);
CREATE UNIQUE INDEX IF NOT EXISTS idx_player_daily_stats_date ON player_daily_stats(date);

