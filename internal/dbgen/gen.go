package dbgen

import (
	"strings"

	"gorm.io/gen"
	"gorm.io/gen/field"
	"gorm.io/gorm"
)

// Config controls code generation behavior.
type Config struct {
	// OutPath is the output directory for generated query code.
	// Example: "internal/db/query"
	OutPath string

	// ModelPkgPath is the package name for generated models, relative to OutPath.
	// Default: "model"
	ModelPkgPath string

	// Tables lists the tables to generate. If empty, all tables are generated.
	Tables []string

	// StripPrefix removes a prefix from table names when generating model names.
	// Example: "t_" strips "t_user" → "User"
	StripPrefix string
}

// Run executes GORM Gen code generation.
func Run(db *gorm.DB, cfg Config) error {
	if cfg.ModelPkgPath == "" {
		cfg.ModelPkgPath = "model"
	}

	genCfg := gen.Config{
		OutPath:      cfg.OutPath,
		Mode:         gen.WithDefaultQuery | gen.WithQueryInterface,
		ModelPkgPath: cfg.ModelPkgPath,
	}

	if cfg.StripPrefix != "" {
		prefix := cfg.StripPrefix
		genCfg.WithModelNameStrategy(func(tableName string) string {
			name := strings.TrimPrefix(tableName, prefix)
			return ucFirst(camelCase(name))
		})
	}

	g := gen.NewGenerator(genCfg)
	g.UseDB(db)

	tables, err := resolveTables(db, cfg.Tables)
	if err != nil {
		return err
	}

	var models []any
	for _, table := range tables {
		model := g.GenerateModel(table, timestampTags()...)
		models = append(models, model)
	}
	g.ApplyBasic(models...)
	g.Execute()
	return nil
}

// resolveTables returns the explicit table list, or all tables from the database if none specified.
func resolveTables(db *gorm.DB, tables []string) ([]string, error) {
	if len(tables) > 0 {
		return tables, nil
	}
	return db.Migrator().GetTables()
}

// timestampTags returns gen.ModelOpt for created_at/updated_at auto-timestamp tags.
func timestampTags() []gen.ModelOpt {
	return []gen.ModelOpt{
		gen.FieldTag("created_at", func(tag field.Tag) field.Tag {
			tag.Set("gorm", "column:created_at;not null;autoCreateTime")
			return tag
		}),
		gen.FieldTag("updated_at", func(tag field.Tag) field.Tag {
			tag.Set("gorm", "column:updated_at;not null;autoUpdateTime")
			return tag
		}),
	}
}

// camelCase converts snake_case to camelCase.
func camelCase(s string) string {
	parts := strings.Split(s, "_")
	for i := range parts {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// ucFirst upper-cases the first character.
func ucFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
