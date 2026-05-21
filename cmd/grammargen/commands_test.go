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
