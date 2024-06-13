package monitoring

var (
	// Logging Formats
	TaskLogger = "task=%s message=%s"
)

func NewLogger(logFields map[string]interface{}) {
}

func NewTaskFormatter(formats ...string) string {
	logFormat := "task=%s"
	for _, format := range formats {
		logFormat = logFormat + " " + format
	}
	return logFormat
}
