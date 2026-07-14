package dingtalk

import "testing"

func TestNormalizeConfiguredRedirectURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "empty stays empty",
			raw:  "",
			want: "",
		},
		{
			name: "root domain appends callback",
			raw:  "https://peopleops.example.com",
			want: "https://peopleops.example.com/callback",
		},
		{
			name: "trailing slash appends callback",
			raw:  "https://peopleops.example.com/",
			want: "https://peopleops.example.com/callback",
		},
		{
			name: "existing callback preserved",
			raw:  "https://peopleops.example.com/callback",
			want: "https://peopleops.example.com/callback",
		},
		{
			name: "query preserved while appending callback",
			raw:  "https://peopleops.example.com/?from=dingtalk",
			want: "https://peopleops.example.com/callback?from=dingtalk",
		},
		{
			name: "invalid uri returned as is",
			raw:  "peopleops.example.com/callback",
			want: "peopleops.example.com/callback",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeConfiguredRedirectURI(tt.raw); got != tt.want {
				t.Fatalf("normalizeConfiguredRedirectURI(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestBuildCallbackURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		appHome string
		want    string
	}{
		{
			name:    "empty",
			appHome: "",
			want:    "",
		},
		{
			name:    "root path",
			appHome: "https://peopleops.example.com",
			want:    "https://peopleops.example.com/callback",
		},
		{
			name:    "nested path trims trailing slash",
			appHome: "https://peopleops.example.com/app/",
			want:    "https://peopleops.example.com/app/callback",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := buildCallbackURL(tt.appHome); got != tt.want {
				t.Fatalf("buildCallbackURL(%q) = %q, want %q", tt.appHome, got, tt.want)
			}
		})
	}
}
