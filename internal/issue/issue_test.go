package issue

import "testing"

func TestLooksLikeLinear(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		{"LIN-123", true},
		{"ENG-42", true},
		{"FOO-1", true},
		{"42", false},
		{"owner/repo#42", false},
		{"lin-123", false}, // lowercase prefix
		{"ENG-", false},    // empty suffix
		{"-123", false},    // empty prefix
		{"ENG-abc", false}, // non-numeric suffix
	}
	for _, tt := range tests {
		got := looksLikeLinear(tt.ref)
		if got != tt.want {
			t.Errorf("looksLikeLinear(%q) = %v; want %v", tt.ref, got, tt.want)
		}
	}
}

func TestParseGitHubRef(t *testing.T) {
	tests := []struct {
		ref         string
		defaultRepo string
		wantOwner   string
		wantRepo    string
		wantNumber  int
		wantErr     bool
	}{
		{"owner/repo#42", "", "owner", "repo", 42, false},
		{"42", "myowner/myrepo", "myowner", "myrepo", 42, false},
		{"notanumber", "myowner/myrepo", "", "", 0, true},
		{"42", "", "", "", 0, true}, // no default repo
	}
	for _, tt := range tests {
		owner, repo, num, err := parseGitHubRef(tt.ref, tt.defaultRepo)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseGitHubRef(%q, %q) error = %v; wantErr %v", tt.ref, tt.defaultRepo, err, tt.wantErr)
			continue
		}
		if err != nil {
			continue
		}
		if owner != tt.wantOwner || repo != tt.wantRepo || num != tt.wantNumber {
			t.Errorf("parseGitHubRef(%q, %q) = (%q, %q, %d); want (%q, %q, %d)",
				tt.ref, tt.defaultRepo, owner, repo, num,
				tt.wantOwner, tt.wantRepo, tt.wantNumber)
		}
	}
}
