package pipeline

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseValid(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "valid.yml"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	want := Config{
		Image:          "node:22",
		TimeoutSeconds: 300,
		Steps: []Step{
			{Name: "Install", Command: "npm ci"},
			{Name: "Test", Command: "npm test\nnpm run build\n"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseInvalid(t *testing.T) {
	cases := []struct {
		file    string
		wantErr string
	}{
		{"missing-image.yml", "image is required"},
		{"blank-image.yml", "image is required"},
		{"empty-steps.yml", "steps must contain at least one step"},
		{"zero-timeout.yml", "timeoutSeconds must be greater than zero"},
		{"negative-timeout.yml", "timeoutSeconds must be greater than zero"},
		{"missing-timeout.yml", "timeoutSeconds must be greater than zero"},
		{"null-timeout.yml", "timeoutSeconds must be greater than zero"},
		{"fractional-timeout.yml", "timeoutSeconds must be an integer"},
		{"float-timeout.yml", "timeoutSeconds must be an integer"},
		{"string-timeout.yml", "timeoutSeconds must be an integer"},
		{"blank-name.yml", "step 2: name is required"},
		{"blank-command.yml", "step 2: command is required"},
		{"unknown-field.yml", "field unexpected not found"},
		{"unknown-step-field.yml", "field unexpected not found"},
		{"multiple-documents.yml", "pipeline must contain exactly one YAML document"},
		{"empty.yml", "decode pipeline: EOF"},
		{"malformed.yml", "decode pipeline:"},
		{"malformed-trailing.yml", "decode trailing content:"},
	}

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tc.file))
			if err != nil {
				t.Fatal(err)
			}

			_, err = Parse(bytes.NewReader(data))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Parse() error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}
