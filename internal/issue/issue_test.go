package issue

import (
	"context"
	"testing"
)

func TestDescriptionSource(t *testing.T) {
	src := NewDescriptionSource("My Title", "My body text.")

	iss, err := src.Fetch(context.Background(), "local:1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if iss.ID != "local:1" {
		t.Errorf("ID = %q; want %q", iss.ID, "local:1")
	}
	if iss.Title != "My Title" {
		t.Errorf("Title = %q; want %q", iss.Title, "My Title")
	}
	if iss.Body != "My body text." {
		t.Errorf("Body = %q; want %q", iss.Body, "My body text.")
	}
	if iss.URL != "" {
		t.Errorf("URL = %q; want empty string", iss.URL)
	}
}

func TestDescriptionSourceEmptyFields(t *testing.T) {
	src := NewDescriptionSource("", "")

	iss, err := src.Fetch(context.Background(), "ref-xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if iss.ID != "ref-xyz" {
		t.Errorf("ID = %q; want %q", iss.ID, "ref-xyz")
	}
	if iss.Title != "" {
		t.Errorf("Title = %q; want empty string", iss.Title)
	}
	if iss.Body != "" {
		t.Errorf("Body = %q; want empty string", iss.Body)
	}
}

func TestDescriptionSourceImplementsSource(t *testing.T) {
	// Compile-time check: *DescriptionSource must satisfy Source interface.
	var _ Source = (*DescriptionSource)(nil)
}

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
