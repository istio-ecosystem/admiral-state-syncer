package monitoring

import "testing"

func TestNewTaskFormatter(t *testing.T) {
	cases := []struct {
		name           string
		format         string
		expectedFormat string
	}{
		{
			name: "Given format contains a message, " +
				"When NewTaskFormatter is invoked, " +
				"It should return message in the format",
			format:         "message=%s",
			expectedFormat: "task=%s message=%s",
		},
		{
			name: "Given format contains a message and a status, " +
				"When NewTaskFormatter is invoked, " +
				"It should return message and status in the format",
			format:         "message=%s status=%v",
			expectedFormat: "task=%s message=%s status=%v",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := NewTaskFormatter(c.format)
			if result != c.expectedFormat {
				t.Errorf("got=%v, want=%v", result, c.expectedFormat)
			}
		})
	}
}
