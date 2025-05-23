package radigo

import (
	"os"

	"github.com/mitchellh/cli"
)

var UI cli.Ui

const (
	infoPrefix  = "INFO: "
	warnPrefix  = "WARN: "
	errorPrefix = "ERROR: "
)

func init() {
	UI = &cli.PrefixedUi{
		InfoPrefix:  infoPrefix,
		WarnPrefix:  warnPrefix,
		ErrorPrefix: errorPrefix,
		Ui: &cli.BasicUi{
			Writer: os.Stdout,
		},
	}
}

func AreaCommandFactory() (cli.Command, error) {
	return &areaCommand{
		ui: UI,
	}, nil
}

func RecCommandFactory() (cli.Command, error) {
	return &recCommand{
		ui: UI,
	}, nil
}

func RecLiveCommandFactory() (cli.Command, error) {
	return &recLiveCommand{
		ui: UI,
	}, nil
}

func BrowseCommandFactory() (cli.Command, error) {
	return &browseCommand{
		ui: UI,
	}, nil
}

func BrowseLiveCommandFactory() (cli.Command, error) {
	return &browseLiveCommand{
		ui: UI,
	}, nil
}
