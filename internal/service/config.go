package service

// Config holds battle configuration loaded from MySQL key_storage table.
type Config struct {
	DemoLimit             int
	WordQuestionCount     int
	WordOptionCount       int
	ExerciseQuestionCount int
	TranslateForeignText  string
	TranslateOriginText   string
}

// DefaultConfig returns Config with PHP default values.
func DefaultConfig() Config {
	return Config{
		DemoLimit:             5,
		WordQuestionCount:     10,
		WordOptionCount:       4,
		ExerciseQuestionCount: 10,
		TranslateForeignText:  "Berilgan so'zning O'zbekcha tarjimasini toping",
		TranslateOriginText:   "Berilgan so'zning Inglizcha tarjimasini toping",
	}
}
