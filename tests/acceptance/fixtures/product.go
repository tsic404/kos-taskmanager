// Package fixtures boots the pieces the acceptance cases need: the packaged
// artifact installed into a throwaway cluster object set, the audit trail
// stand-ins, the accounts and sessions the panel resolves, and the HTTP surface
// a browser reaches.
package fixtures

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"k8s.io/client-go/dynamic"

	"github.com/tsic404/kosd/pkg/access"
	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/pages"
	"github.com/tsic404/kosd/pkg/write"

	"github.com/tsic404/kos-taskmanager/app"
)

// AuditTrail keeps the records a control-plane write leaves behind (KOSD-C-14).
// It only appends, which is what makes one write distinguishable from another.
type AuditTrail struct {
	mu      sync.Mutex
	records []controlplane.AuditRecord
}

// Record implements controlplane.Auditor.
func (t *AuditTrail) Record(_ context.Context, record controlplane.AuditRecord) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records = append(t.records, record)
	return nil
}

// Records returns the records written so far, oldest first.
func (t *AuditTrail) Records() []controlplane.AuditRecord {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]controlplane.AuditRecord(nil), t.records...)
}

// Product is one installed package served the way a browser reaches it: the
// account middleware of KOSD-C-63 in front of the page shell and the resource
// API of KOSD-C-12. A case submits every write through this surface, so a
// refusal is the one the control plane produces and not a test shortcut.
type Product struct {
	// Cluster is the object set both the page and the API read.
	Cluster dynamic.Interface
	// Registry is the contribution projection the installed package produces.
	Registry *extension.Registry
	// Pages is the page shell that renders the declared pages (KOSD-C-53).
	Pages *pages.Service
	// Server is the resource API the actions write through (KOSD-C-12).
	Server *controlplane.Server
	// Trail is the audit trail the resource API appends to (KOSD-C-14).
	Trail *AuditTrail
	// URL is the base address of the served surface.
	URL string

	tokens map[string]string
	client *http.Client
}

// NewProduct installs the artifact, wires its reader into the page shell and
// serves the page read and the control-plane writes behind the account
// middleware. usage may be nil: the metric source is then absent, not zero.
func NewProduct(t *testing.T, cluster dynamic.Interface, trail *Trail, usage app.UsageSource) *Product {
	t.Helper()
	registry := install(t)
	audit := &AuditTrail{}
	readings := &app.Readings{
		Dynamic: cluster,
		Audit:   trail,
		Usage:   usage,
		Now:     func() time.Time { return Now },
	}
	service := pages.New(registry, pages.Config{
		Readings: pages.Router{app.Identifier: readings},
		Now:      func() time.Time { return Now },
	})
	server, err := controlplane.New(controlplane.Config{
		Dynamic:    cluster,
		Authorizer: identity.Authorizer{Policy: &access.Policy{Objects: controlplane.NewClusterSource(cluster)}},
		Auditor:    audit,
		Now:        func() time.Time { return Now },
	})
	if err != nil {
		t.Fatalf("build the control plane: %v", err)
	}
	t.Cleanup(server.Close)

	accounts, tokens := openAccounts(t)
	pagesHandler := pages.NewAPI(service).Handler()
	mux := http.NewServeMux()
	mux.Handle("/api/v1/pages", pagesHandler)
	mux.Handle("/api/v1/pages/", pagesHandler)
	mux.Handle("/", server.Handler())
	httpServer := httptest.NewServer(accounts.Handler(mux))
	t.Cleanup(httpServer.Close)

	return &Product{
		Cluster: cluster, Registry: registry, Pages: service, Server: server,
		Trail: audit, URL: httpServer.URL, tokens: tokens, client: httpServer.Client(),
	}
}

// PageAction resolves one action the application page declares and fills its
// {name} and {namespace} placeholders. Taking method and path from the
// declaration keeps a case from calling an endpoint the page does not offer
// (KOSD-C-12, KOSD-C-22).
func PageAction(t *testing.T, product *Product, id, name, namespace string) (string, string) {
	t.Helper()
	declared, found := product.Registry.PageFor(app.PageApplications, write.RoleOwner)
	if !found {
		t.Fatalf("the installed package declares no page %q", app.PageApplications)
	}
	for _, action := range declared.Actions {
		if action.ID != id {
			continue
		}
		path := strings.ReplaceAll(action.Path, "{name}", name)
		path = strings.ReplaceAll(path, "{namespace}", namespace)
		if strings.ContainsAny(path, "{}") {
			t.Fatalf("action %q path %q still carries a placeholder", id, path)
		}
		return action.Method, path
	}
	t.Fatalf("the application page declares no action %q", id)
	return "", ""
}

// ReadPage reads one page as one account, exactly as the window does.
func (p *Product) ReadPage(t *testing.T, session identity.Session, id string) pages.Page {
	t.Helper()
	status, body := p.Call(t, session, http.MethodGet, "/api/v1/pages/"+url.PathEscape(id), nil, nil)
	if status != http.StatusOK {
		t.Fatalf("read page %q as %q = %d %v, want 200", id, session.Username, status, body)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode page %q: %v", id, err)
	}
	var page pages.Page
	if err := json.Unmarshal(encoded, &page); err != nil {
		t.Fatalf("decode page %q: %v", id, err)
	}
	return page
}

// Call submits one request as one account and returns the status and the
// decoded body.
func (p *Product) Call(t *testing.T, session identity.Session, method, path string, body any, headers map[string]string) (int, map[string]any) {
	t.Helper()
	token, known := p.tokens[session.Username]
	if !known {
		t.Fatalf("account %q was never created", session.Username)
	}
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode the request body: %v", err)
		}
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, p.URL+path, payload)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := p.client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read the answer of %s %s: %v", method, path, err)
	}
	decoded := map[string]any{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("%s %s answered %q: %v", method, path, raw, err)
		}
	}
	return response.StatusCode, decoded
}

// openAccounts opens the control-plane database and signs in the four accounts
// the cases read as (KOSD-C-59, KOSD-C-63).
func openAccounts(t *testing.T) (*identity.API, map[string]string) {
	t.Helper()
	ctx := context.Background()
	store, err := identity.Open(ctx, identity.OpenOptions{
		Path: filepath.Join(t.TempDir(), "kos.db"),
		Now:  func() time.Time { return Now },
	})
	if err != nil {
		t.Fatalf("open the account database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := identity.New(store, identity.Config{Now: func() time.Time { return Now }})

	bootstrap, err := service.BootstrapOwner(ctx, Owner.Username)
	if err != nil {
		t.Fatalf("bootstrap the first owner: %v", err)
	}
	owner, ownerToken := signIn(t, service, Owner.Username, bootstrap.OneTimePassword)
	tokens := map[string]string{owner.Username: ownerToken}
	for _, account := range []identity.Session{OwningMember, NamedMember, Guest} {
		initial := firstPassword(account.Username)
		if _, err := service.CreateAccount(ctx, owner, identity.NewAccount{
			Username: account.Username, Role: account.Role, OneTimePassword: initial,
		}); err != nil {
			t.Fatalf("create account %q: %v", account.Username, err)
		}
		_, token := signIn(t, service, account.Username, initial)
		tokens[account.Username] = token
	}
	return identity.NewAPI(service), tokens
}

// signIn resolves one account's session token, changing the one-time password
// the first login still carries.
func signIn(t *testing.T, service *identity.Service, username, initial string) (identity.Session, string) {
	t.Helper()
	ctx := context.Background()
	login, err := service.Login(ctx, username, initial)
	if err != nil {
		t.Fatalf("log in %q: %v", username, err)
	}
	if login.Session.MustChangePassword {
		next := sessionPassword(username)
		if err := service.ChangePassword(ctx, login.Token, initial, next); err != nil {
			t.Fatalf("change the password of %q: %v", username, err)
		}
		if login, err = service.Login(ctx, username, next); err != nil {
			t.Fatalf("log in %q after the password change: %v", username, err)
		}
	}
	return login.Session, login.Token
}

// firstPassword is the one-time password an account is created with.
func firstPassword(username string) string { return "first-" + username }

// sessionPassword is the password a case signs in with.
func sessionPassword(username string) string { return "session-" + username }
