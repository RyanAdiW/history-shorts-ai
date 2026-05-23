package imagegen

import "testing"

func TestValidateQuality(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty defaults to low", input: "", want: "low"},
		{name: "low", input: "low", want: "low"},
		{name: "medium", input: "medium", want: "medium"},
		{name: "high", input: "high", want: "high"},
		{name: "trims and normalizes", input: " Medium ", want: "medium"},
		{name: "invalid", input: "ultra", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ValidateQuality(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.want {
				t.Fatalf("quality = %q, want %q", got, test.want)
			}
		})
	}
}
