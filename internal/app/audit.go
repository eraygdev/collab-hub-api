package app

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"
)

// auditLog audit_logs tablosuna kayıt atar.
// Kasıtlı olarak context.Background() kullanır — response iptal olsa bile log yazılmalı.
func auditLog(c *gin.Context, userID int, action, entityType string, entityID int) {
	var uid interface{} = userID
	if userID == 0 {
		uid = nil
	}

	var eid interface{} = entityID
	if entityID == 0 {
		eid = nil
	}

	_, err := db.Exec(context.Background(), `
		INSERT INTO audit_logs (user_id, action, entity_type, entity_id, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, uid, action, entityType, eid, c.ClientIP(), c.Request.UserAgent())

	if err != nil {
		log.Printf("[AUDIT ERROR] %v", err)
	}
}
