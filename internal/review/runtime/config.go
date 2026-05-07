package reviewruntime

import "github.com/aaudin90/opencode-reviewer/internal/shared/config"

type Config struct {
	AppConfig                     *config.Config
	ConfigBaseDir                 string
	CLIConfigDir                  string
	DisableConfigDirAutoDiscovery bool
}
