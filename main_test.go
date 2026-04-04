package main

import "testing"

func TestParseModeDefaultsToServer(t *testing.T) {
	mode, err := parseMode(nil)
	if err != nil {
		t.Fatalf("parse mode: %v", err)
	}
	if mode != modeServer {
		t.Fatalf("expected %s, got %s", modeServer, mode)
	}
}

func TestParseModeAcceptsAdmin(t *testing.T) {
	mode, err := parseMode([]string{modeAdmin})
	if err != nil {
		t.Fatalf("parse mode: %v", err)
	}
	if mode != modeAdmin {
		t.Fatalf("expected %s, got %s", modeAdmin, mode)
	}
}

func TestParseModeRejectsUnknownValue(t *testing.T) {
	if _, err := parseMode([]string{"oops"}); err == nil {
		t.Fatal("expected parse error for unknown mode")
	}
}
