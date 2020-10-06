package pkg

import (
	"testing"

	siteapi "github.com/drud/ddev-live-sdk/golang/pkg/site/apis/site/v1beta1"
)

type sis struct {
	org  string
	repo string
	url  string
	rev  string
}

func TestSisMatches(t *testing.T) {
	tests := []struct {
		sis          sis
		url          string
		originBranch string
		match        bool
	}{
		{ // 0
			sis:   sis{url: "https://github.com/org1/repo", rev: "branch1"},
			url:   "",
			match: false,
		},
		{ // 1
			sis:          sis{url: "https://github.com/org1/repo", rev: "branch1"},
			url:          "https://github.com/org1/repo",
			originBranch: "branch1",
			match:        true,
		},
		{ // 2
			sis:   sis{url: "https://github.com/org1/repo"},
			url:   "https://github.com/org1/repo1",
			match: false,
		},
		{ // 3
			sis:   sis{url: "https://github.com/org1/repo1"},
			url:   "https://github.com/org1/repo",
			match: false,
		},
		{ // 4
			sis:   sis{url: "https://github.com/org1/repo1"},
			url:   "https://github.com/org1/repo1",
			match: true,
		},
		{ // 5
			sis:          sis{url: "https://github.com/org1/repo1", rev: "branch1"},
			url:          "https://github.com/org1/repo1",
			originBranch: "branch2",
			match:        false,
		},
		{ // 6
			sis:   sis{org: "org1", repo: "repo1"},
			url:   "https://github.com/org1/repo1",
			match: true,
		},
		{ // 7
			sis:          sis{org: "org1", repo: "repo1", rev: "branch1"},
			url:          "https://github.com/org1/repo1",
			originBranch: "branch1",
			match:        true,
		},
		{ // 8
			sis:          sis{org: "org1", repo: "repo1", rev: "branch2"},
			url:          "https://github.com/org1/repo1",
			originBranch: "branch1",
			match:        false,
		},
		{ // 9
			sis:   sis{org: "org1", repo: "repo"},
			url:   "https://github.com/org1/repo1",
			match: false,
		},
		{ // 10
			sis:   sis{org: "org", repo: "repo1"},
			url:   "https://github.com/org1/repo1",
			match: false,
		},
	}
	for i, tt := range tests {
		sis := &siteapi.SiteImageSource{
			Spec: siteapi.SiteImageSourceSpec{
				GitHubSourceDeprecated: siteapi.GitHubSource{
					RepoName: tt.sis.repo,
					OrgName:  tt.sis.org,
					Branch:   tt.sis.rev,
				},
				GitSource: siteapi.GitSource{
					URL:      tt.sis.url,
					Revision: tt.sis.rev,
				},
			},
		}
		if tt.match != sisMatches(sis, tt.url, tt.originBranch) {
			t.Errorf("failed %v. test: %v <=> %v#%v expected %v", i, tt.sis, tt.url, tt.originBranch, tt.match)
		}
	}
}
