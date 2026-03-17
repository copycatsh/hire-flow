package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	port := ":8004"
	slog.Info("starting payments", "port", port)
	if err := r.Run(port); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
