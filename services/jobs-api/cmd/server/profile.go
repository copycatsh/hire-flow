package main

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Profile struct {
	ID         uuid.UUID `json:"id"`
	FullName   string    `json:"full_name"`
	Bio        string    `json:"bio"`
	HourlyRate int       `json:"hourly_rate"`
	Available  bool      `json:"available"`
	Skills     []Skill   `json:"skills"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type CreateProfileRequest struct {
	FullName   string `json:"full_name"`
	Bio        string `json:"bio,omitzero"`
	HourlyRate int    `json:"hourly_rate,omitzero"`
}

type UpdateProfileSkillsRequest struct {
	SkillIDs []uuid.UUID `json:"skill_ids"`
}

type ProfileStore interface {
	Create(ctx context.Context, db DBTX, req CreateProfileRequest) (Profile, error)
	GetByID(ctx context.Context, db DBTX, id uuid.UUID) (Profile, error)
	UpdateSkills(ctx context.Context, db DBTX, profileID uuid.UUID, req UpdateProfileSkillsRequest) error
}
