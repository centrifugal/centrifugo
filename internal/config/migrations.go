package config

func applyConfigMigrations(cfg Config) (Config, []string) {
	var warnings []string
	return cfg, warnings
}
