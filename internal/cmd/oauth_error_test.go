package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/rlrghb/olkcli/internal/msauth"
)

func TestJSONPreservesWrappedOAuthFailureMetadata(t *testing.T) {
	var out, stderr bytes.Buffer
	err := fmt.Errorf("private-wrapper-fixture: %w", &msauth.RefreshError{
		Status: 429, Code: "temporarily_unavailable", AADSTSCodes: []int{40012}, RetryAfterSeconds: 120,
	})
	writeCommandError(true, err, &out, &stderr)
	var value struct {
		Error struct {
			Code              string `json:"code"`
			Status            int    `json:"status"`
			Stage             string `json:"stage"`
			OAuthError        string `json:"oauth_error"`
			AADSTSCodes       []int  `json:"aadsts_codes"`
			RetryAfterSeconds int    `json:"retry_after_seconds"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	got := value.Error
	if got.Code != "OAuthRefreshFailed" || got.Status != 429 || got.Stage != "token_refresh" || got.OAuthError != "temporarily_unavailable" || got.RetryAfterSeconds != 120 || len(got.AADSTSCodes) != 1 || got.AADSTSCodes[0] != 40012 {
		t.Fatalf("missing OAuth metadata: %s", out.String())
	}
	if strings.Contains(out.String(), "private-wrapper-fixture") || stderr.Len() != 0 {
		t.Fatal("JSON error exposed wrapped private text")
	}
}
