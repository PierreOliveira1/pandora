package registry_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/httptool"
	"github.com/PierreOliveira1/pandora/internal/registry"
)

func addHTTPService(t *testing.T, f *fixture, service string, urls map[string]string, secretEnv map[string]string) {
	t.Helper()
	if err := f.reg.Add(registry.AddInput{
		Service:   service,
		Account:   "store",
		Kind:      config.KindHTTP,
		URLs:      urls,
		SecretEnv: secretEnv,
	}); err != nil {
		t.Fatalf("adding HTTP service: %v", err)
	}
}

func TestAnHTTPServiceNeedsAtLeastOneHTTPSURL(t *testing.T) {
	f := newFixture(t)
	err := f.reg.Add(registry.AddInput{Service: "ig", Account: "store", Kind: config.KindHTTP})
	want := "service ig needs at least one URL: use --http name=URL"
	if err == nil || err.Error() != want {
		t.Fatalf("Add() without URLs error = %v, want %q", err, want)
	}

	err = f.reg.Add(registry.AddInput{
		Service: "ig",
		Account: "store",
		Kind:    config.KindHTTP,
		URLs:    map[string]string{"api": "http://example.com"},
	})
	if err == nil || !strings.Contains(err.Error(), "the URL must use https") {
		t.Fatalf("Add() with an http URL error = %v, want the https requirement", err)
	}

	if err := f.reg.Add(registry.AddInput{
		Service: "ig",
		Account: "store",
		Kind:    config.KindHTTP,
		URLs:    map[string]string{"api": "https://example.com/"},
	}); err != nil {
		t.Fatalf("Add() with a valid URL error = %v", err)
	}
	cfg := loadConfig(t, f)
	if got := cfg.Services["ig"].URLs["api"]; got != "https://example.com" {
		t.Errorf("stored URL = %q, want the trailing slash removed", got)
	}
}

func TestACommandServiceCannotTakeHTTPURLsOrTools(t *testing.T) {
	f := newFixture(t)
	if err := f.reg.Add(registry.AddInput{Service: "cmd", Account: "local", Kind: config.KindCommand}); err != nil {
		t.Fatalf("adding command service: %v", err)
	}

	err := f.reg.SetServiceURL("cmd", "api", "https://example.com")
	want := "service cmd is not an HTTP service: create one with pandora add cmd --as <account> --http name=URL"
	if err == nil || err.Error() != want {
		t.Fatalf("SetServiceURL() error = %v, want %q", err, want)
	}

	err = f.reg.AddCommandTool(registry.CommandToolInput{
		Service: "cmd",
		Name:    "get",
		HTTP:    &config.HTTPRequest{Method: "GET", Path: "/"},
	})
	want = "service cmd has no URL: add one with pandora http url cmd <name> <url>"
	if err == nil || err.Error() != want {
		t.Fatalf("AddCommandTool() error = %v, want %q", err, want)
	}
}

func TestAnHTTPServiceRefusesToLoseItsLastURLOrAURLUsedByATool(t *testing.T) {
	f := newFixture(t)
	addHTTPService(t, f, "ig", map[string]string{
		"graph": "https://graph.example.com",
		"other": "https://other.example.com",
	}, nil)
	if err := f.reg.AddCommandTool(registry.CommandToolInput{
		Service: "ig",
		Name:    "posts",
		HTTP:    &config.HTTPRequest{URL: "graph", Method: "GET", Path: "/media"},
	}); err != nil {
		t.Fatalf("adding HTTP tool: %v", err)
	}

	err := f.reg.RemoveServiceURL("ig", "graph")
	want := "tool posts uses URL graph: remove the tool first"
	if err == nil || err.Error() != want {
		t.Fatalf("RemoveServiceURL() error = %v, want %q", err, want)
	}
	if err := f.reg.RemoveServiceURL("ig", "other"); err != nil {
		t.Fatalf("removing unused URL: %v", err)
	}
	err = f.reg.RemoveServiceURL("ig", "graph")
	want = "an HTTP service needs at least one URL; remove the service with pandora remove ig --yes instead"
	if err == nil || err.Error() != want {
		t.Fatalf("RemoveServiceURL() last URL error = %v, want %q", err, want)
	}
}

func TestAToolCannotTakeASecretIntoTheQuery(t *testing.T) {
	f := newFixture(t)
	addHTTPService(t, f, "ig", map[string]string{"graph": "https://graph.example.com"}, map[string]string{"META_TOKEN": "s3cr3t"})

	err := f.reg.AddCommandTool(registry.CommandToolInput{
		Service: "ig",
		Name:    "posts",
		HTTP: &config.HTTPRequest{
			URL:    "graph",
			Method: "GET",
			Path:   "/",
			Query:  map[string]string{"q": "{{env.META_TOKEN}}"},
		},
	})
	want := "tool posts: the secret META_TOKEN can only be used in the service headers"
	if err == nil || err.Error() != want {
		t.Fatalf("AddCommandTool() error = %v, want %q", err, want)
	}
}

func TestAGETHTTPToolIsReadOnly(t *testing.T) {
	f := newFixture(t)
	addHTTPService(t, f, "ig", map[string]string{"graph": "https://graph.example.com"}, nil)
	if err := f.reg.AddCommandTool(registry.CommandToolInput{
		Service: "ig",
		Name:    "get",
		HTTP:    &config.HTTPRequest{URL: "graph", Method: "get", Path: "/media"},
	}); err != nil {
		t.Fatalf("adding GET tool: %v", err)
	}
	if err := f.reg.AddCommandTool(registry.CommandToolInput{
		Service: "ig",
		Name:    "post",
		HTTP:    &config.HTTPRequest{URL: "graph", Method: "POST", Path: "/media"},
	}); err != nil {
		t.Fatalf("adding POST tool: %v", err)
	}

	cfg := loadConfig(t, f)
	for _, tool := range cfg.Services["ig"].Tools {
		switch tool.Name {
		case "get":
			if !tool.ReadOnly || tool.HTTP.Method != "GET" {
				t.Errorf("GET tool = %+v, want read-only and uppercase method", tool)
			}
		case "post":
			if tool.ReadOnly {
				t.Errorf("POST tool = %+v, want writable", tool)
			}
		}
	}
}

func TestAnAccountValueCannotOverwriteASecret(t *testing.T) {
	f := newFixture(t)
	addHTTPService(t, f, "ig", map[string]string{"graph": "https://graph.example.com"}, map[string]string{"META_TOKEN": "s3cr3t"})

	err := f.reg.SetAccountEnv(registry.Ref{Service: "ig", Account: "store"}, "META_TOKEN", "value")
	want := "META_TOKEN is a secret of ig/store: change it with pandora remove and add"
	if err == nil || err.Error() != want {
		t.Fatalf("SetAccountEnv() error = %v, want %q", err, want)
	}
}

func TestAnHTTPServiceAcceptsSecretHeadersButNotOAuth(t *testing.T) {
	f := newFixture(t)
	if err := f.reg.Add(registry.AddInput{
		Service:       "ig",
		Account:       "store",
		Kind:          config.KindHTTP,
		URLs:          map[string]string{"graph": "https://graph.example.com"},
		SecretHeaders: map[string]string{"X-Api-Key": "s3cr3t"},
	}); err != nil {
		t.Fatalf("Add() with a secret header error = %v", err)
	}
	cfg := loadConfig(t, f)
	if headers := cfg.Services["ig"].Accounts["store"].SecretHeaders; !slices.Contains(headers, "X-Api-Key") {
		t.Errorf("secret headers = %v, want X-Api-Key", headers)
	}

	oauth := newFixture(t)
	err := oauth.reg.Add(registry.AddInput{
		Service: "ig",
		Account: "store",
		Kind:    config.KindHTTP,
		URLs:    map[string]string{"graph": "https://graph.example.com"},
		Auth:    config.AuthOAuth,
	})
	if err == nil || !strings.Contains(err.Error(), "oauth is not supported for http services") {
		t.Fatalf("Add() with OAuth error = %v, want the oauth restriction", err)
	}
}

func TestAnHTTPServiceStoresCanonicalHeadersAndURLs(t *testing.T) {
	f := newFixture(t)
	addHTTPService(t, f, "ig", map[string]string{"graph": "https://graph.example.com"}, nil)

	if err := f.reg.SetServiceHeader("ig", "authorization", "OAuth {{env.META_TOKEN}}"); err != nil {
		t.Fatalf("SetServiceHeader() error = %v", err)
	}
	if err := f.reg.SetServiceURL("ig", "other", "https://other.example.com/"); err != nil {
		t.Fatalf("SetServiceURL() error = %v", err)
	}
	cfg := loadConfig(t, f)
	if got := cfg.Services["ig"].Headers["Authorization"]; got != "OAuth {{env.META_TOKEN}}" {
		t.Errorf("header = %q, want the canonical name and template", got)
	}
	if got := cfg.Services["ig"].URLs["other"]; got != "https://other.example.com" {
		t.Errorf("URL = %q, want the trailing slash removed", got)
	}
	if err := f.reg.RemoveServiceHeader("ig", "Authorization"); err != nil {
		t.Fatalf("RemoveServiceHeader() error = %v", err)
	}
	if err := f.reg.RemoveServiceHeader("ig", "Authorization"); err == nil {
		t.Fatal("RemoveServiceHeader() accepted a missing header")
	}
}

func TestAccountValuesAreStoredAndRemoved(t *testing.T) {
	f := newFixture(t)
	addHTTPService(t, f, "ig", map[string]string{"graph": "https://graph.example.com"}, nil)

	if err := f.reg.SetAccountEnv(registry.Ref{Service: "ig", Account: "store"}, "IG_USER_ID", "42"); err != nil {
		t.Fatalf("SetAccountEnv() error = %v", err)
	}
	info, err := f.reg.GetAccountEnv(registry.Ref{Service: "ig", Account: "store"})
	if err != nil {
		t.Fatalf("GetAccountEnv() error = %v", err)
	}
	if info.Env["IG_USER_ID"] != "42" {
		t.Errorf("env = %v, want IG_USER_ID=42", info.Env)
	}
	if err := f.reg.UnsetAccountEnv(registry.Ref{Service: "ig", Account: "store"}, "IG_USER_ID"); err != nil {
		t.Fatalf("UnsetAccountEnv() error = %v", err)
	}
	if err := f.reg.UnsetAccountEnv(registry.Ref{Service: "ig", Account: "store"}, "IG_USER_ID"); err == nil {
		t.Fatal("UnsetAccountEnv() accepted a missing value")
	}
}

func TestAToolKeepsItsURLWhenTheServiceGetsASecondOne(t *testing.T) {
	f := newFixture(t)
	addHTTPService(t, f, "ig", map[string]string{"graph": "https://graph.example.com"}, nil)
	if err := f.reg.AddCommandTool(registry.CommandToolInput{
		Service: "ig",
		Name:    "posts",
		HTTP:    &config.HTTPRequest{Method: "GET", Path: "/media"},
	}); err != nil {
		t.Fatalf("adding HTTP tool: %v", err)
	}
	if err := f.reg.SetServiceURL("ig", "other", "https://other.example.com"); err != nil {
		t.Fatalf("adding a second URL: %v", err)
	}

	service := loadConfig(t, f).Services["ig"]
	for _, tool := range service.Tools {
		if err := httptool.Validate(tool, service, nil); err != nil {
			t.Errorf("tool %s after a second URL: %v", tool.Name, err)
		}
	}
}
