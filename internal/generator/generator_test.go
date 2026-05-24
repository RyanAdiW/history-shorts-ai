package generator

import "testing"

func TestShouldForceBaseSteps(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   bool
	}{
		{
			name:   "no force",
			config: Config{},
			want:   false,
		},
		{
			name:   "force without artifact flags",
			config: Config{Force: true},
			want:   true,
		},
		{
			name:   "force captions only",
			config: Config{Force: true, GenerateCaptions: true},
			want:   false,
		},
		{
			name:   "force render only",
			config: Config{Force: true, GenerateRender: true},
			want:   false,
		},
		{
			name:   "force voice and images",
			config: Config{Force: true, GenerateVoice: true, GenerateImages: true},
			want:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldForceBaseSteps(test.config); got != test.want {
				t.Fatalf("shouldForceBaseSteps() = %v, want %v", got, test.want)
			}
		})
	}
}
