package main

import (
	"reflect"
	"testing"
)

func TestNormalizeSubcommandArgsAllowsGrammarBeforeFlags(t *testing.T) {
	got := normalizeSubcommandArgs([]string{"calc", "-text", "1+2", "-runtime"})
	want := []string{"-text", "1+2", "-runtime", "calc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeSubcommandArgs() = %#v, want %#v", got, want)
	}
}

func TestNormalizeSubcommandArgsKeepsFlagEqualsValues(t *testing.T) {
	got := normalizeSubcommandArgs([]string{"calc", "-text=1+2", "-sample", "sample.txt"})
	want := []string{"-text=1+2", "-sample", "sample.txt", "calc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeSubcommandArgs() = %#v, want %#v", got, want)
	}
}

func TestNormalizeSubcommandArgsHandlesAuthoringValueFlags(t *testing.T) {
	got := normalizeSubcommandArgs([]string{
		"calc",
		"-format", "json",
		"-expect", "want.sexpr",
		"-write-expect", "got.sexpr",
		"-json-out", "grammar.json",
		"-conflicts", "2",
	})
	want := []string{
		"-format", "json",
		"-expect", "want.sexpr",
		"-write-expect", "got.sexpr",
		"-json-out", "grammar.json",
		"-conflicts", "2",
		"calc",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeSubcommandArgs() = %#v, want %#v", got, want)
	}
}
