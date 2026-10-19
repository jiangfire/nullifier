package entity

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// dialectPortableTypes lists the explicit column types that exist under the
// same name in both supported dialects (SQLite and PostgreSQL).
//
// Anything outside this list must be left to GORM's own type resolution:
// GORM maps []byte to "blob" on SQLite and "bytea" on PostgreSQL, so pinning
// either name breaks the other. Hardcoding "blob" once made every PostgreSQL
// AutoMigrate fail with `type "blob" does not exist (SQLSTATE 42704)`, and the
// SQLite-backed unit tests could not see it because they only ever ran on
// SQLite.
var dialectPortableTypes = map[string]struct{}{
	"text": {},
}

func TestEntitiesAvoidDialectSpecificColumnTypes(t *testing.T) {
	models := []any{
		Vote{},
		Proposal{},
		Credential{},
		VoteParticipation{},
		DecryptionSession{},
		TallyResult{},
		AuditAnchor{},
	}

	for _, model := range models {
		modelType := reflect.TypeOf(model)
		t.Run(modelType.Name(), func(t *testing.T) {
			for i := 0; i < modelType.NumField(); i++ {
				field := modelType.Field(i)
				tag := field.Tag.Get("gorm")
				if tag == "" {
					continue
				}

				explicitType := gormTagType(tag)
				if explicitType == "" {
					continue
				}

				fieldName := modelType.Name() + "." + field.Name
				if _, ok := dialectPortableTypes[explicitType]; ok {
					continue
				}

				require.Failf(t, "dialect-specific column type",
					"%s pins type:%q, which only exists on one dialect. "+
						"Drop it and let GORM resolve the column type per driver "+
						"(for []byte that is blob on SQLite and bytea on PostgreSQL).",
					fieldName, explicitType,
				)
			}
		})
	}
}

// TestByteSliceFieldsLeaveTypeToGORM pins down the specific case that regressed:
// a []byte column must never carry an explicit type, because the correct name
// differs between SQLite and PostgreSQL.
func TestByteSliceFieldsLeaveTypeToGORM(t *testing.T) {
	models := []any{
		Vote{},
		Credential{},
		DecryptionSession{},
		TallyResult{},
		AuditAnchor{},
	}

	for _, model := range models {
		modelType := reflect.TypeOf(model)
		for i := 0; i < modelType.NumField(); i++ {
			field := modelType.Field(i)
			if field.Type.Kind() != reflect.Slice || field.Type.Elem().Kind() != reflect.Uint8 {
				continue
			}
			if explicitType := gormTagType(field.Tag.Get("gorm")); explicitType != "" {
				require.Failf(t, "byte slice column pins a type",
					"%s is []byte but pins type:%q; leave it unset so GORM picks "+
						"bytea on PostgreSQL and blob on SQLite.",
					modelType.Name()+"."+field.Name, explicitType,
				)
			}
		}
	}
}

func gormTagType(tag string) string {
	for _, part := range strings.Split(tag, ";") {
		value, ok := strings.CutPrefix(strings.TrimSpace(part), "type:")
		if ok {
			return value
		}
	}
	return ""
}
