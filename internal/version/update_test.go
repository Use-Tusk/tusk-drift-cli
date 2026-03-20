package version

import "testing"

func TestIsHomebrewPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "macos apple silicon cellar path",
			path: "/opt/homebrew/Cellar/tusk/1.2.3/bin/tusk",
			want: true,
		},
		{
			name: "macos intel cellar path",
			path: "/usr/local/Cellar/tusk/1.2.3/bin/tusk",
			want: true,
		},
		{
			name: "linuxbrew cellar path",
			path: "/home/linuxbrew/.linuxbrew/Cellar/tusk/1.2.3/bin/tusk",
			want: true,
		},
		{
			name: "non homebrew binary path",
			path: "/usr/local/bin/tusk",
			want: false,
		},
		{
			name: "different formula in cellar",
			path: "/opt/homebrew/Cellar/other/1.2.3/bin/other",
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isHomebrewPath(tc.path)
			if got != tc.want {
				t.Fatalf("isHomebrewPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestGetDownloadURLLinuxAMD64(t *testing.T) {
	t.Parallel()

	originalGoos := runtimeGOOS
	originalGoarch := runtimeGOARCH
	runtimeGOOS = "linux"
	runtimeGOARCH = "amd64"
	defer func() {
		runtimeGOOS = originalGoos
		runtimeGOARCH = originalGoarch
	}()

	got := getDownloadURL("v1.2.3")
	want := "https://github.com/Use-Tusk/tusk-cli/releases/download/v1.2.3/tusk-cli_1.2.3_Linux_x86_64.tar.gz"

	if got != want {
		t.Fatalf("getDownloadURL() = %q, want %q", got, want)
	}
}

func TestGetDownloadURLDarwinARM64(t *testing.T) {
	t.Parallel()

	originalGoos := runtimeGOOS
	originalGoarch := runtimeGOARCH
	runtimeGOOS = "darwin"
	runtimeGOARCH = "arm64"
	defer func() {
		runtimeGOOS = originalGoos
		runtimeGOARCH = originalGoarch
	}()

	got := getDownloadURL("v1.2.3")
	want := "https://github.com/Use-Tusk/tusk-cli/releases/download/v1.2.3/tusk-cli_1.2.3_Darwin_arm64.tar.gz"

	if got != want {
		t.Fatalf("getDownloadURL() = %q, want %q", got, want)
	}
}
