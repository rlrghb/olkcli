package cmd

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestVersionJSONAdvertisesProviderBodyFormatCapability(t *testing.T) {
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
	if want := []string{"mail.provider-body-format-v1"}; !reflect.DeepEqual(got.Capabilities, want) {
		t.Fatalf("capabilities = %v, want %v", got.Capabilities, want)
	}
}
