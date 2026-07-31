package cmd

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestVersionJSONAdvertisesStructuredMailCapabilities(t *testing.T) {
	flags := &RootFlags{JSON: true}
	stdout, _, err := captureStd(func() error {
		return (&VersionCmd{}).Run(&RunContext{Ctx: context.Background(), Flags: flags})
	})
	if err != nil {
		t.Fatalf("VersionCmd.Run() error = %v", err)
	}

	var got struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &got); err != nil {
		t.Fatalf("decoding version JSON: %v\n%s", err, stdout)
	}
	if want := []string{
		"cli.json-error-v1",
		"mail.folders.well-known-v1",
		"mail.get.parent-folder-v1",
		"mail.ids.immutable-v1",
		"mail.message-observations-v1",
		"mail.move.structured-receipt-v1",
		"mail.provider-body-format-v1",
		"mail.thread.complete-v1",
	}; !reflect.DeepEqual(got.Capabilities, want) {
		t.Fatalf("capabilities = %v, want %v", got.Capabilities, want)
	}
}
