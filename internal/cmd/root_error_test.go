package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
)

func TestWriteCommandErrorUsesStructuredJSONForGraphFailure(t *testing.T) {
	graphErr := odataerrors.NewODataError()
	mainErr := odataerrors.NewMainError()
	code := "ErrorItemNotFound"
	message := "The specified object was not found."
	mainErr.SetCode(&code)
	mainErr.SetMessage(&message)
	graphErr.SetErrorEscaped(mainErr)
	graphErr.SetStatusCode(404)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	writeCommandError(
		true,
		fmt.Errorf("getting message: %w", graphErr),
		&stdout,
		&stderr,
	)

	var got map[string]map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("JSON error output: %v", err)
	}
	if got["error"]["code"] != code {
		t.Fatalf("error code = %v, want %q", got["error"]["code"], code)
	}
	if got["error"]["status"] != float64(404) {
		t.Fatalf("error status = %v, want 404", got["error"]["status"])
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestWriteCommandErrorKeepsHumanModeOnStderr(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := errors.New("sentinel failure")

	writeCommandError(false, err, &stdout, &stderr)

	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.String() != "Error: sentinel failure\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestWriteCommandErrorDoesNotExposeLocalFailureMessageInJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	writeCommandError(
		true,
		errors.New("private local diagnostic"),
		&stdout,
		&stderr,
	)

	if stdout.String() != "{\"error\":{\"code\":\"CommandFailed\",\"status\":0}}\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestImmutableIDsFlagUsesDocumentedSpelling(t *testing.T) {
	cli := &CLI{}
	parser, err := newKongParser(cli)
	if err != nil {
		t.Fatalf("newKongParser: %v", err)
	}
	if _, err := parser.Parse([]string{"--immutable-ids", "mail", "list"}); err != nil {
		t.Fatalf("parse --immutable-ids: %v", err)
	}
	if !cli.ImmutableIDs {
		t.Fatal("--immutable-ids did not set RootFlags.ImmutableIDs")
	}
}
