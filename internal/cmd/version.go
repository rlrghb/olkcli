package cmd

import (
	"encoding/json"
	"fmt"
)

type VersionCmd struct{}

type versionInfo struct {
	Version      string   `json:"version"`
	Commit       string   `json:"commit"`
	Date         string   `json:"date"`
	Capabilities []string `json:"capabilities"`
}

var advertisedCapabilities = []string{
	"calendar.provider-body-format-v1",
	"calendar.provider-metadata-v1",
	"calendar.structured-recurrence-v1",
	"calendar.structured-attendance-v1",
	"calendar.retry-safe-create-v1",
	"calendar.explicit-update-controls-v1",
	"mail.provider-metadata-v1",
	"contacts.provider-metadata-v1",
	"cli.json-error-v1",
	"mail.folders.well-known-v1",
	"mail.get.parent-folder-v1",
	"mail.ids.immutable-v1",
	"mail.message-observations-v1",
	"mail.move.structured-receipt-v1",
	"mail.provider-body-format-v1",
	"mail.thread.complete-v1",
	"mail.shared-mailbox-send-v1",
	"mcp.delegated-mailbox-v1",
}

func (c *VersionCmd) Run(ctx *RunContext) error {
	if ctx.Flags.JSON {
		info := versionInfo{Version: Version, Commit: Commit, Date: Date, Capabilities: advertisedCapabilities}
		data, err := json.Marshal(info)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	fmt.Printf("olk %s (commit: %s, built: %s)\n", Version, Commit, Date)
	return nil
}
