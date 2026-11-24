package cli

import (
	"strings"
	"testing"
)

func TestAnHTTPServiceAndToolAreVisibleInHTTPShow(t *testing.T) {
	newCLIHome(t)
	runPandora(t, "", "add", "instagram", "--as", "store",
		"--http", "graph=https://graph.example.com",
		"--env", "IG_USER_ID=42",
		"--secret-env", "META_TOKEN=s3cr3t",
	)

	stdout, _ := runPandora(t, "", "tool", "add", "instagram", "posts",
		"--http", "GET /{{env.IG_USER_ID}}/media",
		"--url", "graph",
		"--param", "limit:integer",
		"--query", "limit={{limit}}",
		"--readonly",
	)
	want := "Added tool instagram/posts (GET graph /{{env.IG_USER_ID}}/media)"
	if !strings.Contains(stdout, want) {
		t.Fatalf("tool add output = %q, want %q", stdout, want)
	}

	runPandora(t, "", "http", "header", "instagram", "Authorization", "OAuth {{env.META_TOKEN}}")

	stdout, _ = runPandora(t, "", "http", "show", "instagram")
	for _, want := range []string{
		"graph",
		"https://graph.example.com",
		"Authorization",
		"OAuth {{env.META_TOKEN}}",
		"posts",
		"GET",
		"/{{env.IG_USER_ID}}/media",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("http show output does not contain %q:\n%s", want, stdout)
		}
	}
}

func TestAnHTTPURLWithoutANameIsCalledAPI(t *testing.T) {
	newCLIHome(t)
	runPandora(t, "", "add", "api", "--as", "default", "--http", "https://api.example.com")

	stdout, _ := runPandora(t, "", "http", "show", "api")
	if !strings.Contains(stdout, "api") || !strings.Contains(stdout, "https://api.example.com") {
		t.Fatalf("http show output = %q, want the api URL", stdout)
	}
}

func TestEnvListsValuesWithoutShowingSecrets(t *testing.T) {
	newCLIHome(t)
	runPandora(t, "", "add", "instagram", "--as", "store",
		"--http", "graph=https://graph.example.com",
		"--env", "IG_USER_ID=42",
		"--secret-env", "META_TOKEN=s3cr3t",
	)
	runPandora(t, "", "env", "instagram/store", "EXTRA=value")

	stdout, _ := runPandora(t, "", "env", "instagram/store")
	for _, want := range []string{"IG_USER_ID=42", "EXTRA=value", "META_TOKEN (secret)"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("env output does not contain %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "s3cr3t") {
		t.Errorf("env output contains the secret value:\n%s", stdout)
	}
}
