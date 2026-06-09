package bottlerocket

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	apiclientBin string
	signpostBin  string
}

type OSInfo struct {
	VersionID string `json:"version_id"`
	VariantID string `json:"variant_id"`
	Arch      string `json:"arch"`
}

func New(apiclientBin, signpostBin string) *Client {
	return &Client{
		apiclientBin: apiclientBin,
		signpostBin:  signpostBin,
	}
}

func (c *Client) OSInfo(ctx context.Context) (OSInfo, error) {
	out, err := c.run(ctx, c.apiclientBin, "raw", "-u", "/os")
	if err != nil {
		return OSInfo{}, err
	}
	var info OSInfo
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		return OSInfo{}, err
	}
	return info, nil
}

func (c *Client) SetVersionLock(ctx context.Context, targetVersion string) error {
	_, err := c.run(ctx, c.apiclientBin, "set", "settings.updates.version-lock="+targetVersion)
	return err
}

func (c *Client) CheckUpdate(ctx context.Context) error {
	_, err := c.run(ctx, c.apiclientBin, "update", "check")
	return err
}

func (c *Client) ApplyUpdate(ctx context.Context) error {
	_, err := c.run(ctx, c.apiclientBin, "update", "apply")
	return err
}

func (c *Client) Reboot(ctx context.Context) error {
	_, err := c.run(ctx, c.apiclientBin, "reboot")
	return err
}

func (c *Client) DeactivatePreparedUpdate(ctx context.Context) error {
	_, err := c.run(ctx, c.apiclientBin, "raw", "-u", "/actions/deactivate-update", "-m", "POST")
	return err
}

func (c *Client) RollbackToInactive(ctx context.Context) error {
	if _, err := c.run(ctx, c.signpostBin, "rollback-to-inactive"); err != nil {
		return err
	}
	return c.Reboot(ctx)
}

func (c *Client) run(ctx context.Context, bin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		if output == "" {
			return "", fmt.Errorf("%s %s: %w", bin, strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("%s %s: %w: %s", bin, strings.Join(args, " "), err, output)
	}
	return output, nil
}
