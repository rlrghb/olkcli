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
	"cli.json-error-v1",
	"mail.folders.well-known-v1",
	"mail.get.parent-folder-v1",
	"mail.ids.immutable-v1",
	"mail.message-observations-v1",
	"mail.move.structured-receipt-v1",
	"mail.provider-body-format-v1",
	"mail.thread.complete-v1",
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
