package main

import (
	"time"

	"github.com/google/uuid"
)

type Skill struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Category  string    `json:"category"`
	CreatedAt time.Time `json:"created_at"`
}
