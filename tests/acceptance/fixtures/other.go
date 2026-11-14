package fixtures

import (
	"testing"
	"time"

	"github.com/tsic404/kosd/pkg/extension"
)

// OtherComponent is an unrelated installed component that contributes one page.
// A case installs it beside this package to show that disabling this package
// withdraws only its own surfaces (KOSD-AC-54).
func OtherComponent(t *testing.T, identifier, pageID, title string) extension.Request {
	t.Helper()
	declaration := extension.Declaration{
		Identifier:       identifier,
		Version:          "1.0.0",
		FrameworkVersion: ">=1.0 <2.0",
		Contributions: extension.Contributions{
			Pages: []extension.Page{{
				ID:       pageID,
				Title:    title,
				Location: "panel",
				Items: []extension.PageItem{{
					ID: "notes", Label: "记录", Source: extension.PageSourceLocal, Store: "notes",
					Reason: "只读展示",
				}},
			}},
		},
	}
	archive, digest, err := extension.Pack(declaration, nil)
	if err != nil {
		t.Fatalf("pack the %s component: %v", identifier, err)
	}
	return extension.Request{
		Name:    identifier,
		Created: Now.Add(time.Minute),
		Package: &extension.Package{
			Reference: identifier + "-package",
			Digest:    digest,
			Content:   archive,
		},
	}
}
