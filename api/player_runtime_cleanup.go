package api

import (
	"log"
	"time"

	"terraria-panel/db"
)

func finalizeRoomPlayerActivity(roomID int) {
	if roomID <= 0 || db.DB == nil {
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		log.Printf("[WARN] 无法开始玩家会话清理事务，room=%d err=%v", roomID, err)
		return
	}

	now := time.Now()
	rows, err := tx.Query(`
		SELECT id, player_id, join_time
		FROM player_sessions
		WHERE room_id = ? AND leave_time IS NULL
	`, roomID)
	if err != nil {
		_ = tx.Rollback()
		log.Printf("[WARN] 查询活跃会话失败，room=%d err=%v", roomID, err)
		return
	}

	type activeSession struct {
		id       int
		playerID int
		joinTime time.Time
	}

	sessions := make([]activeSession, 0)
	for rows.Next() {
		var session activeSession
		if err := rows.Scan(&session.id, &session.playerID, &session.joinTime); err != nil {
			rows.Close()
			_ = tx.Rollback()
			log.Printf("[WARN] 读取活跃会话失败，room=%d err=%v", roomID, err)
			return
		}
		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		_ = tx.Rollback()
		log.Printf("[WARN] 遍历活跃会话失败，room=%d err=%v", roomID, err)
		return
	}
	rows.Close()

	for _, session := range sessions {
		duration := int(now.Sub(session.joinTime).Seconds())
		if duration < 0 {
			duration = 0
		}

		if _, err := tx.Exec(`
			UPDATE player_sessions
			SET leave_time = ?, duration = ?
			WHERE id = ?
		`, now, duration, session.id); err != nil {
			_ = tx.Rollback()
			log.Printf("[WARN] 更新会话离线时间失败，room=%d session=%d err=%v", roomID, session.id, err)
			return
		}

		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO player_stats (player_id, total_play_time, login_count, first_seen)
			VALUES (?, 0, 0, CURRENT_TIMESTAMP)
		`, session.playerID); err != nil {
			_ = tx.Rollback()
			log.Printf("[WARN] 初始化玩家统计失败，room=%d player=%d err=%v", roomID, session.playerID, err)
			return
		}

		if _, err := tx.Exec(`
			UPDATE player_stats
			SET total_play_time = total_play_time + ?, last_logout_time = ?, updated_at = CURRENT_TIMESTAMP
			WHERE player_id = ?
		`, duration, now, session.playerID); err != nil {
			_ = tx.Rollback()
			log.Printf("[WARN] 更新玩家统计失败，room=%d player=%d err=%v", roomID, session.playerID, err)
			return
		}
	}

	if _, err := tx.Exec(`
		UPDATE players
		SET status = 'offline', last_seen = CURRENT_TIMESTAMP
		WHERE room_id = ? AND status != 'offline'
	`, roomID); err != nil {
		_ = tx.Rollback()
		log.Printf("[WARN] 清理玩家在线状态失败，room=%d err=%v", roomID, err)
		return
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[WARN] 提交玩家会话清理事务失败，room=%d err=%v", roomID, err)
	}
}
