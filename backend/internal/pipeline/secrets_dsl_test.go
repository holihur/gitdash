package pipeline

import "testing"

func TestParseSecretsInline(t *testing.T) {
	cfg, err := Parse([]byte("secrets: [TOKEN, DB_PASS]\nsteps:\n  - run: echo hi\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cfg.Secrets) != 2 || cfg.Secrets[0] != "TOKEN" || cfg.Secrets[1] != "DB_PASS" {
		t.Fatalf("secrets = %v", cfg.Secrets)
	}
}

func TestParseSecretsBlock(t *testing.T) {
	cfg, err := Parse([]byte("secrets:\n  - TOKEN\n  - API_KEY\nsteps:\n  - run: echo hi\n"))
	if err != nil {
		t.Fatalf("parse block: %v", err)
	}
	if len(cfg.Secrets) != 2 || cfg.Secrets[0] != "TOKEN" || cfg.Secrets[1] != "API_KEY" {
		t.Fatalf("secrets = %v", cfg.Secrets)
	}
}

func TestParseSecretsRejectsInvalid(t *testing.T) {
	if _, err := Parse([]byte("secrets: [BAD-NAME]\nsteps:\n  - run: echo hi\n")); err == nil {
		t.Fatal("invalid secret name should fail")
	}
	if _, err := Parse([]byte("secrets: [A, A]\nsteps:\n  - run: echo hi\n")); err == nil {
		t.Fatal("duplicate secret should fail")
	}
}
