package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type PostgresProfileStore struct{}

func (s *PostgresProfileStore) Create(ctx context.Context, db DBTX, req CreateProfileRequest) (Profile, error) {
	var p Profile
	err := db.QueryRow(ctx,
		`INSERT INTO profiles (full_name, bio, hourly_rate)
		 VALUES ($1, $2, $3)
		 RETURNING id, full_name, bio, hourly_rate, available, created_at, updated_at`,
		req.FullName, req.Bio, req.HourlyRate,
	).Scan(&p.ID, &p.FullName, &p.Bio, &p.HourlyRate, &p.Available, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return Profile{}, fmt.Errorf("profile create: %w", err)
	}
	p.Skills = []Skill{}
	return p, nil
}

func (s *PostgresProfileStore) GetByID(ctx context.Context, db DBTX, id uuid.UUID) (Profile, error) {
	var p Profile
	err := db.QueryRow(ctx,
		`SELECT id, full_name, bio, hourly_rate, available, created_at, updated_at
		 FROM profiles WHERE id = $1`, id,
	).Scan(&p.ID, &p.FullName, &p.Bio, &p.HourlyRate, &p.Available, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return Profile{}, fmt.Errorf("profile get by id: %w", err)
	}

	rows, err := db.Query(ctx,
		`SELECT s.id, s.name, s.category, s.created_at
		 FROM skills s
		 JOIN profile_skills ps ON ps.skill_id = s.id
		 WHERE ps.profile_id = $1
		 ORDER BY s.name`, id,
	)
	if err != nil {
		return Profile{}, fmt.Errorf("profile get skills: %w", err)
	}

	skills, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Skill])
	if err != nil {
		return Profile{}, fmt.Errorf("profile get skills collect: %w", err)
	}

	if skills == nil {
		skills = []Skill{}
	}
	p.Skills = skills
	return p, nil
}

func (s *PostgresProfileStore) UpdateSkills(ctx context.Context, db DBTX, profileID uuid.UUID, req UpdateProfileSkillsRequest) error {
	_, err := db.Exec(ctx, `DELETE FROM profile_skills WHERE profile_id = $1`, profileID)
	if err != nil {
		return fmt.Errorf("profile update skills delete: %w", err)
	}

	for _, skillID := range req.SkillIDs {
		_, err := db.Exec(ctx,
			`INSERT INTO profile_skills (profile_id, skill_id) VALUES ($1, $2)`,
			profileID, skillID,
		)
		if err != nil {
			return fmt.Errorf("profile update skills insert: %w", err)
		}
	}

	_, err = db.Exec(ctx, `UPDATE profiles SET updated_at = now() WHERE id = $1`, profileID)
	if err != nil {
		return fmt.Errorf("profile update skills timestamp: %w", err)
	}

	return nil
}
