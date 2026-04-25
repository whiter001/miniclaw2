package cli

import "testing"

func TestBrowserOpenCommand(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		url     string
		cmd     string
		args    []string
		wantErr bool
	}{
		{
			name: "darwin",
			goos: "darwin",
			url:  "https://example.com/q?id=1",
			cmd:  "open",
			args: []string{"https://example.com/q?id=1"},
		},
		{
			name: "linux",
			goos: "linux",
			url:  "https://example.com/q?id=1",
			cmd:  "xdg-open",
			args: []string{"https://example.com/q?id=1"},
		},
		{
			name: "windows",
			goos: "windows",
			url:  "https://example.com/q?id=1",
			cmd:  "rundll32",
			args: []string{"url.dll,FileProtocolHandler", "https://example.com/q?id=1"},
		},
		{
			name:    "unsupported",
			goos:    "plan9",
			url:     "https://example.com/q?id=1",
			wantErr: true,
		},
		{
			name:    "empty",
			goos:    "darwin",
			url:     "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args, err := browserOpenCommand(tt.goos, tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cmd != tt.cmd {
				t.Fatalf("cmd = %q, want %q", cmd, tt.cmd)
			}
			if len(args) != len(tt.args) {
				t.Fatalf("args len = %d, want %d", len(args), len(tt.args))
			}
			for i := range args {
				if args[i] != tt.args[i] {
					t.Fatalf("args[%d] = %q, want %q", i, args[i], tt.args[i])
				}
			}
		})
	}
}