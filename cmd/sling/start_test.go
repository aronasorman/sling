package main

import "testing"

func TestParseGitHubRepoFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://github.com/owner/repo.git", "owner/repo"},
		{"https://github.com/owner/repo", "owner/repo"},
		{"git@github.com:owner/repo.git", "owner/repo"},
		{"git@github.com:owner/repo", "owner/repo"},
		{"https://gitlab.com/owner/repo.git", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := parseGitHubRepoFromURL(tt.url)
		if got != tt.want {
			t.Errorf("parseGitHubRepoFromURL(%q) = %q; want %q", tt.url, got, tt.want)
		}
	}
}
