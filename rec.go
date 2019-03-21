package radigo

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/kangaechu/radigo/internal"
	"github.com/mitchellh/cli"
	"github.com/olekukonko/tablewriter"
	"github.com/yyoshiki41/go-radiko"
)

type recCommand struct {
	ui cli.Ui
}

func (c *recCommand) Run(args []string) int {
	var stationID, start, areaID, fileType string

	f := flag.NewFlagSet("rec", flag.ContinueOnError)
	f.StringVar(&stationID, "id", "", "id")
	f.StringVar(&start, "start", "", "start")
	f.StringVar(&start, "s", "", "start")
	f.StringVar(&areaID, "area", "", "area")
	f.StringVar(&areaID, "a", "", "area")
	f.StringVar(&fileType, "output", AudioFormatM4A, "output")
	f.StringVar(&fileType, "o", AudioFormatM4A, "output")
	f.Usage = func() { c.ui.Error(c.Help()) }
	if err := f.Parse(args); err != nil {
		return 1
	}

	if stationID == "" {
		c.ui.Error("StationID is empty.")
		return 1
	}
	startTime, err := time.ParseInLocation(datetimeLayout, start, location)
	if err != nil {
		c.ui.Error(fmt.Sprintf(
			"Invalid start time format '%s': %s", start, err))
		return 1
	}
	if fileType != AudioFormatM4A && fileType != AudioFormatMP3 {
		c.ui.Error(fmt.Sprintf(
			"Unsupported audio format: %s", fileType))
		return 1
	}

	output, err := NewOutputConfig(
		fmt.Sprintf("%s-%s", startTime.In(location).Format(datetimeLayout), stationID),
		fileType)
	if err != nil {
		c.ui.Error(fmt.Sprintf(
			"Failed to configure output: %s", err))
		return 1
	}
	if err := output.SetupDir(); err != nil {
		c.ui.Error(fmt.Sprintf(
			"Failed to setup the output dir: %s", err))
		return 1
	}
	if output.IsExist() {
		c.ui.Error(fmt.Sprintf(
			"the output file already exists: %s", output.AbsPath()))
		return 1
	}

	c.ui.Output("Now downloading.. ")
	spin := spinner.New(spinner.CharSets[9], time.Second)
	spin.Start()
	defer spin.Stop()

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	client, err := getClient(ctx, areaID)
	if err != nil {
		c.ui.Error(fmt.Sprintf(
			"Failed to construct a radiko Client: %s", err))
		return 1
	}

	_, err = client.AuthorizeToken(ctx)
	if err != nil {
		c.ui.Error(fmt.Sprintf(
			"Failed to get auth_token: %s", err))
		return 1
	}

	pg, err := client.GetProgramByStartTime(ctx, stationID, startTime)
	if err != nil {
		ctxCancel()
		c.ui.Error(fmt.Sprintf(
			"Failed to get the program: %s", err))
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"STATION ID", "TITLE"})
	table.Append([]string{stationID, pg.Title})
	fmt.Print("\n")
	table.Render()

	uri, err := client.TimeshiftPlaylistM3U8(ctx, stationID, startTime)
	if err != nil {
		c.ui.Error(fmt.Sprintf(
			"Failed to get playlist.m3u8: %s", err))
		return 1
	}

	chunklist, err := radiko.GetChunklistFromM3U8(uri)
	if err != nil {
		c.ui.Error(fmt.Sprintf(
			"Failed to get chunklist: %s", err))
		return 1
	}

	m4aDir, err := output.TempM4ADir()
	if err != nil {
		c.ui.Error(fmt.Sprintf(
			"Failed to create the m4a dir: %s", err))
		return 1
	}
	defer os.RemoveAll(m4aDir) // clean up

	if err := internal.BulkDownload(chunklist, m4aDir); err != nil {
		c.ui.Error(fmt.Sprintf(
			"Failed to download m4a files: %s", err))
		return 1
	}

	tag := CreateTagFromPg(pg, stationID)
	output.FileBaseName = tag.toFileName()
	metadata := tag.toFfMpegOption()
	concatedFile, err := ConcatM4AFilesFromList(ctx, m4aDir, metadata)
	if err != nil {
		c.ui.Error(fmt.Sprintf(
			"Failed to concat m4a files: %s", err))
		return 1
	}

	var retErr error
	switch output.AudioFormat() {
	case AudioFormatM4A:
		retErr = os.Rename(concatedFile, output.AbsPath())
	case AudioFormatMP3:
		retErr = ConvertM4AtoMP3(ctx, concatedFile, output.AbsPath())
	}
	if retErr != nil {
		c.ui.Error(fmt.Sprintf(
			"Failed to output a result file: %s", retErr))
		return 1
	}

	c.ui.Output(fmt.Sprintf("Completed!\n%s", output.AbsPath()))
	return 0
}

func (c *recCommand) Synopsis() string {
	return "Record a radiko program"
}

func (c *recCommand) Help() string {
	return strings.TrimSpace(`
Usage: radigo rec [options]
  Record a radiko program.
Options:
  -id=name                 Station id
  -start,s=201610101000    Start time
  -area,a=name             Area id
  -output,o=m4a            Output file type (m4a, mp3)
`)
}
