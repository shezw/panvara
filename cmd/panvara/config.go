/*
   Panvara
   cmd/panvara/config.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package main

import (
	"fmt"
	"path/filepath"
	"strings"

	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

type serverConfig struct {
	databaseURL   string
	moduleSource  string
	moduleFormat  string
	projectID     string
	projectKey    string
	projectLocale string
	projectZone   string
	projectMoney  string
	adminToken    string
}

func (config serverConfig) validate() error {
	missing := make([]string, 0, 4)
	if strings.TrimSpace(config.databaseURL) == "" {
		missing = append(missing, "database URL")
	}
	if strings.TrimSpace(config.moduleSource) == "" {
		missing = append(missing, "module source")
	}
	if strings.TrimSpace(config.projectID) == "" {
		missing = append(missing, "project ID")
	}
	if config.adminToken == "" {
		missing = append(missing, "administrator token")
	}
	if len(missing) > 0 {
		return fmt.Errorf("server profile requires %s", strings.Join(missing, ", "))
	}
	return nil
}

func parseModuleFormat(value, sourcePath string) (spec.Format, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "json":
		return spec.FormatJSON, nil
	case "yaml", "yml":
		return spec.FormatYAML, nil
	case "", "auto":
		switch strings.ToLower(filepath.Ext(sourcePath)) {
		case ".json":
			return spec.FormatJSON, nil
		case ".yaml", ".yml":
			return spec.FormatYAML, nil
		default:
			return "", fmt.Errorf("cannot infer module format from %q; select json or yaml", sourcePath)
		}
	default:
		return "", fmt.Errorf("unsupported module format %q; select json or yaml", value)
	}
}
