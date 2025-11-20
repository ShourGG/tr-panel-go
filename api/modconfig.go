package api
import (
	"encoding/json"
	"net/http"
	"strconv"
	"terraria-panel/db"
	"time"
	"github.com/gin-gonic/gin"
)
type ModProfile struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Mods        string    `json:"mods"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
type ModProfileRequest struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Mods        []interface{} `json:"mods"`
}
func GetModProfiles(c *gin.Context) {
	rows, err := db.DB.Query(`
		SELECT id, name, description, mods, created_at, updated_at
		FROM mod_profiles
		ORDER BY created_at DESC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to query mod profiles: " + err.Error(),
		})
		return
	}
	defer rows.Close()
	profiles := []ModProfile{}
	for rows.Next() {
		var profile ModProfile
		if err := rows.Scan(&profile.ID, &profile.Name, &profile.Description, &profile.Mods, &profile.CreatedAt, &profile.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to scan mod profile: " + err.Error(),
			})
			return
		}
		profiles = append(profiles, profile)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    profiles,
	})
}
func CreateModProfile(c *gin.Context) {
	var req ModProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Name is required",
		})
		return
	}
	modsJSON, err := json.Marshal(req.Mods)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to marshal mods: " + err.Error(),
		})
		return
	}
	result, err := db.DB.Exec(`
		INSERT INTO mod_profiles (name, description, mods, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, req.Name, req.Description, string(modsJSON), time.Now(), time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to create mod profile: " + err.Error(),
		})
		return
	}
	id, _ := result.LastInsertId()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":          id,
			"name":        req.Name,
			"description": req.Description,
			"mods":        string(modsJSON),
			"createdAt":   time.Now(),
			"updatedAt":   time.Now(),
		},
		"message": "Mod profile created successfully",
	})
}
func UpdateModProfile(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid profile ID",
		})
		return
	}
	var req ModProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}
	var exists bool
	err = db.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM mod_profiles WHERE id = ?)", id).Scan(&exists)
	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Mod profile not found",
		})
		return
	}
	modsJSON, err := json.Marshal(req.Mods)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to marshal mods: " + err.Error(),
		})
		return
	}
	_, err = db.DB.Exec(`
		UPDATE mod_profiles
		SET name = ?, description = ?, mods = ?, updated_at = ?
		WHERE id = ?
	`, req.Name, req.Description, string(modsJSON), time.Now(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update mod profile: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Mod profile updated successfully",
	})
}
func DeleteModProfile(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid profile ID",
		})
		return
	}
	var exists bool
	err = db.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM mod_profiles WHERE id = ?)", id).Scan(&exists)
	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Mod profile not found",
		})
		return
	}
	_, err = db.DB.Exec("DELETE FROM mod_profiles WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to delete mod profile: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Mod profile deleted successfully",
	})
}
func InitModProfilesTable() error {
	_, err := db.DB.Exec(`
		CREATE TABLE IF NOT EXISTS mod_profiles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT,
			mods TEXT NOT NULL DEFAULT '[]',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`)
	return err
}
