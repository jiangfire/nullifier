package persistence

import (
	"anonvote/internal/domain/entity"

	"gorm.io/gorm"
)

func AutoMigrate(db *gorm.DB) error {
	if err := migrateLegacyCredentialSchema(db); err != nil {
		return err
	}
	if err := db.AutoMigrate(
		&entity.Vote{},
		&entity.Proposal{},
		&entity.Credential{},
		&entity.VoteParticipation{},
		&entity.DecryptionSession{},
		&entity.TallyResult{},
		&entity.AuditAnchor{},
	); err != nil {
		return err
	}

	// Backfill old SQLite/Postgres rows created before Proposal.OptionsJSON existed.
	return db.Model(&entity.Proposal{}).
		Where("options_json IS NULL OR options_json = ''").
		Update("options_json", "[]").Error
}

func migrateLegacyCredentialSchema(db *gorm.DB) error {
	if !db.Migrator().HasTable(&entity.Credential{}) {
		return nil
	}
	if !db.Migrator().HasColumn(&entity.Credential{}, "proposal_id") {
		if err := db.Exec("ALTER TABLE credentials ADD COLUMN proposal_id text DEFAULT ''").Error; err != nil {
			return err
		}
	}
	if err := db.Exec("UPDATE credentials SET proposal_id = '' WHERE proposal_id IS NULL").Error; err != nil {
		return err
	}
	if db.Migrator().HasIndex(&entity.Credential{}, "idx_credentials_user_id") {
		if err := db.Migrator().DropIndex(&entity.Credential{}, "idx_credentials_user_id"); err != nil {
			return err
		}
	}
	return db.Exec(
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_credential_proposal_user ON credentials(proposal_id, user_id)",
	).Error
}
