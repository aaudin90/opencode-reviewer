package reviewruntime

import "github.com/aaudin90/opencode-reviewer/internal/config"

type Config struct {
	AppConfig                     *config.Config
	ConfigBaseDir                 string
	CLIConfigDir                  string
	DisableConfigDirAutoDiscovery bool
}
