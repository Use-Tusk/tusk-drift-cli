package cmd

import "testing"

func TestParseRepoSlugFromRemote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		remoteURL string
		want      string
	}{
		{
			name:      "github https",
			remoteURL: "https://github.com/use-tusk/tusk.git",
			want:      "use-tusk/tusk",
		},
		{
			name:      "github ssh",
			remoteURL: "git@github.com:use-tusk/tusk.git",
			want:      "use-tusk/tusk",
		},
		{
			name:      "prefixed https path",
			remoteURL: "https://git.example.com/scm/team/use-tusk/tusk.git",
			want:      "use-tusk/tusk",
		},
		{
			name:      "prefixed ssh path",
			remoteURL: "git@git.example.com:scm/team/use-tusk/tusk.git",
			want:      "use-tusk/tusk",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseRepoSlugFromRemote(tt.remoteURL)
			if err != nil {
				t.Fatalf("parseRepoSlugFromRemote returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseRepoSlugFromRemote = %q, want %q", got, tt.want)
			}
		})
	}
}
