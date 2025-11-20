-- 用户表
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    role TEXT DEFAULT 'user',
    server_mode TEXT DEFAULT 'rooms',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 房间表
CREATE TABLE IF NOT EXISTS rooms (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    server_type TEXT NOT NULL,
    world_file TEXT NOT NULL,
    port INTEGER NOT NULL,
    max_players INTEGER DEFAULT 8,
    password TEXT,
    mod_profile TEXT,
    world_size TEXT DEFAULT 'medium',
    difficulty TEXT DEFAULT 'normal',
    evil_type TEXT DEFAULT 'corruption',
    status TEXT DEFAULT 'stopped',
    pid INTEGER DEFAULT 0,
    start_time DATETIME,
    admin_token TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 玩家表
CREATE TABLE IF NOT EXISTS players (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    ip TEXT,
    team INTEGER DEFAULT 0,
    is_banned BOOLEAN DEFAULT 0,
    room_id INTEGER DEFAULT 0,
    status TEXT DEFAULT 'offline',
    last_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 玩家会话记录表
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

-- 玩家统计数据表
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

-- 玩家每日统计表
CREATE TABLE IF NOT EXISTS player_daily_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date DATE NOT NULL,
    total_players INTEGER DEFAULT 0,
    active_players INTEGER DEFAULT 0,
    new_players INTEGER DEFAULT 0,
    total_play_time INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 操作日志表
CREATE TABLE IF NOT EXISTS operation_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    action TEXT NOT NULL,
    target_type TEXT,
    target_id INTEGER,
    details TEXT,
    ip_address TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- 登录失败记录（防爆破）
CREATE TABLE IF NOT EXISTS login_attempts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    ip_address TEXT NOT NULL,
    failed_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 定时任务表
CREATE TABLE IF NOT EXISTS scheduled_tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,
    enabled BOOLEAN DEFAULT 1,
    cron_expression VARCHAR(100) NOT NULL,
    params TEXT,
    description TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_run_at DATETIME,
    next_run_at DATETIME,
    last_run_status VARCHAR(20),
    last_run_error TEXT,
    run_count INTEGER DEFAULT 0,
    success_count INTEGER DEFAULT 0,
    failed_count INTEGER DEFAULT 0
);

-- 任务执行日志表
CREATE TABLE IF NOT EXISTS task_execution_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id INTEGER NOT NULL,
    status VARCHAR(20) NOT NULL,
    started_at DATETIME NOT NULL,
    finished_at DATETIME,
    duration INTEGER,
    error_message TEXT,
    output TEXT,
    FOREIGN KEY (task_id) REFERENCES scheduled_tasks(id) ON DELETE CASCADE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_rooms_status ON rooms(status);
CREATE INDEX IF NOT EXISTS idx_players_name ON players(name);
CREATE INDEX IF NOT EXISTS idx_players_room_id ON players(room_id);
CREATE INDEX IF NOT EXISTS idx_players_status ON players(status);
CREATE INDEX IF NOT EXISTS idx_operation_logs_user_id ON operation_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_operation_logs_created_at ON operation_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_login_attempts_username ON login_attempts(username);
CREATE INDEX IF NOT EXISTS idx_login_attempts_ip ON login_attempts(ip_address);
CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_enabled ON scheduled_tasks(enabled);
CREATE INDEX IF NOT EXISTS idx_task_execution_logs_task_id ON task_execution_logs(task_id);
CREATE INDEX IF NOT EXISTS idx_task_execution_logs_started_at ON task_execution_logs(started_at);
CREATE INDEX IF NOT EXISTS idx_player_sessions_player_id ON player_sessions(player_id);
CREATE INDEX IF NOT EXISTS idx_player_sessions_room_id ON player_sessions(room_id);
CREATE INDEX IF NOT EXISTS idx_player_sessions_join_time ON player_sessions(join_time);
CREATE INDEX IF NOT EXISTS idx_player_stats_player_id ON player_stats(player_id);
CREATE INDEX IF NOT EXISTS idx_player_stats_total_play_time ON player_stats(total_play_time);
CREATE INDEX IF NOT EXISTS idx_player_stats_login_count ON player_stats(login_count);
CREATE INDEX IF NOT EXISTS idx_player_stats_last_login_time ON player_stats(last_login_time);
CREATE UNIQUE INDEX IF NOT EXISTS idx_player_daily_stats_date ON player_daily_stats(date);

-- 活动日志表
CREATE TABLE IF NOT EXISTS activity_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    room_id INTEGER,
    player_name TEXT,
    color TEXT DEFAULT 'blue',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE SET NULL
);

-- 创建活动日志索引
CREATE INDEX IF NOT EXISTS idx_activity_logs_created_at ON activity_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_activity_logs_type ON activity_logs(type);
CREATE INDEX IF NOT EXISTS idx_activity_logs_room_id ON activity_logs(room_id);

-- 插件服表（全局唯一的TShock插件服）
CREATE TABLE IF NOT EXISTS plugin_server (
    id INTEGER PRIMARY KEY CHECK (id = 1),  -- Only one record allowed (global unique)
    name TEXT NOT NULL DEFAULT 'TShock Plugin Server',
    port INTEGER NOT NULL DEFAULT 7777,
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

-- 插入默认插件服配置
INSERT OR IGNORE INTO plugin_server (id, name, port, world_file)
VALUES (1, 'TShock Plugin Server', 7777, 'plugin-test.wld');

-- 不再自动创建默认用户，让用户自己注册第一个管理员
