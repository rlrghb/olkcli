package cmd

import (
	"fmt"
	"time"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// ConfigCmd manages persistent configuration settings.
type ConfigCmd struct {
	Set ConfigSetCmd `cmd:"" help:"Set a configuration value"`
	Get ConfigGetCmd `cmd:"" help:"Get a configuration value"`
}

// ConfigSetCmd sets a configuration key.
type ConfigSetCmd struct {
	Key   string `arg:"" help:"Configuration key (e.g. timezone)" enum:"timezone"`
	Value string `arg:"" help:"Value to set"`
}

func (c *ConfigSetCmd) Run(ctx *RunContext) error {
	cfg, err := ctx.Config()
	if err != nil {
		return err
	}

	switch c.Key {
	case "timezone":
		if _, err := time.LoadLocation(c.Value); err != nil {
			return fmt.Errorf("invalid timezone %q: %w", c.Value, err)
		}
		cfg.SetTimezone(c.Value)
	default:
		return fmt.Errorf("unknown config key: %s", c.Key)
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("%s = %s\n", c.Key, outfmt.Sanitize(c.Value))
	return nil
}

// ConfigGetCmd reads a configuration key.
type ConfigGetCmd struct {
	Key string `arg:"" help:"Configuration key (e.g. timezone)" enum:"timezone"`
}

func (c *ConfigGetCmd) Run(ctx *RunContext) error {
	cfg, err := ctx.Config()
	if err != nil {
		return err
	}

	switch c.Key {
	case "timezone":
		v := cfg.GetTimezone()
		if v == "" {
			v = "Local (not set)"
		}
		fmt.Println(outfmt.Sanitize(v))
	default:
		return fmt.Errorf("unknown config key: %s", c.Key)
	}

	return nil
}
