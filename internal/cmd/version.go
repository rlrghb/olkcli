package cmd

import (
	"encoding/json"
	"fmt"
)

type VersionCmd struct{}

type versionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

func (c *VersionCmd) Run(ctx *RunContext) error {
	if ctx.Flags.JSON {
		info := versionInfo{Version: Version, Commit: Commit, Date: Date}
		data, err := json.Marshal(info)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	fmt.Printf("olkcli %s (commit: %s, built: %s)\n", Version, Commit, Date)
	return nil
}
