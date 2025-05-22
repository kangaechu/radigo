package radigo

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/mitchellh/cli"
	"github.com/olekukonko/tablewriter"
	"github.com/yyoshiki41/go-radiko"
	"github.com/yyoshiki41/radigo/internal"
)

type recLiveCommand struct {
	ui cli.Ui
}

func (c *recLiveCommand) Run(args []string) int {
	var stationID, duration, areaID, fileType string
	var verbose bool

	f := flag.NewFlagSet("rec-live", flag.ContinueOnError)
	f.StringVar(&stationID, "id", "", "id")
	f.StringVar(&duration, "time", "", "duration")
	f.StringVar(&duration, "t", "", "duration")
	f.StringVar(&areaID, "area", "", "area")
	f.StringVar(&areaID, "a", "", "area")
	f.StringVar(&fileType, "output", AudioFormatAAC, "output")
	f.StringVar(&fileType, "o", AudioFormatAAC, "output")
	f.BoolVar(&verbose, "verbose", false, "verbose")
	f.BoolVar(&verbose, "v", false, "verbose")
	f.Usage = func() { c.ui.Error(c.Help()) }
	if err := f.Parse(args); err != nil {
		return 1
	}

	if stationID == "" {
		c.ui.Error("StationID is empty.")
		return 1
	}
	if duration == "" {
		c.ui.Error("Duration is empty.")
		return 1
	}

	output, err := NewOutputConfig(
		fmt.Sprintf("%s-%s", time.Now().In(location).Format(datetimeLayout), stationID),
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
	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"Station ID", "Duration(sec)"})
	err = table.Append([]string{stationID, duration})
	if err != nil {
		c.ui.Error(fmt.Sprintf(
			"Failed to append table: %s", err))
		return 1
	}
	err = table.Render()
	if err != nil {
		c.ui.Error(fmt.Sprintf(
			"Failed to render table: %s", err))
		return 1
	}

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

	items, err := radiko.GetStreamMultiURL(stationID)
	if err != nil {
		c.ui.Error(fmt.Sprintf(
			"Failed to get a stream url: %s", err))
		return 1
	}

	var streamURL string
	for _, i := range items {
		// Premium user
		if areaID != "" && areaID != currentAreaID {
			if i.Areafree {
				streamURL = i.Item
				break
			}
			continue
		}
		// Normal user
		if !i.Areafree {
			streamURL = i.Item
			break
		}
	}

	tempDir, removeTempDir := internal.CreateTempDir()
	defer removeTempDir()

	swfPlayer := filepath.Join(tempDir, "myplayer.swf")
	if err := radiko.DownloadPlayer(swfPlayer); err != nil {
		c.ui.Error(fmt.Sprintf(
			"Failed to download swf player. %s", err))
		return 1
	}

	rtmpdumpCmd, err := newRtmpdump(ctx, streamURL, client.AuthToken(), duration, swfPlayer)
	if err != nil {
		c.ui.Error(fmt.Sprintf(
			"Failed to construct rtmpdump command: %s", err))
		return 1
	}

	rtmpdumpStdout, err := rtmpdumpCmd.StdoutPipe()
	if err != nil {
		c.ui.Error(fmt.Sprintf("%v", err))
		return 1
	}

	ffmpegCmd, err := newFfmpeg(ctx)
	if err != nil {
		c.ui.Error(fmt.Sprintf(
			"Failed to construct ffmpeg command: %s", err))
		return 1
	}
	ffmpegCmd.setInput("-")

	ffmpegArgs := []string{"-vn", "-acodec"}
	switch fileType {
	case AudioFormatAAC:
		ffmpegArgs = append(ffmpegArgs, "copy")
	case AudioFormatMP3:
		ffmpegArgs = append(ffmpegArgs,
			[]string{"libmp3lame",
				"-ar", "44100",
				"-ab", "64k",
				"-ac", "2"}...)
	}
	ffmpegCmd.setArgs(ffmpegArgs...)

	// For debugging mode
	ffmpegStderr, err := ffmpegCmd.stderrPipe()
	if err != nil {
		c.ui.Warn(fmt.Sprintf("%v", err))
	}

	// need to close
	ffmpegStdin, err := ffmpegCmd.stdinPipe()
	if err != nil {
		c.ui.Error(fmt.Sprintf("%v", err))
		return 1
	}

	err = ffmpegCmd.start(output.AbsPath())
	if err != nil {
		err := ffmpegStdin.Close()
		if err != nil {
			c.ui.Error(fmt.Sprintf(
				"Failed to close ffmpeg stdin: %s", err))
			return 1
		}

		c.ui.Error(fmt.Sprintf(
			"Failed to start ffmpeg command: %s", err))
		return 1
	}

	go func() {
		// Block until catch EOF in rtmpdumpStdout
		_, err := io.Copy(ffmpegStdin, rtmpdumpStdout)
		if err != nil {
			ctxCancel()
			c.ui.Error(fmt.Sprintf("%v", err))
		}
	}()

	go func() {
		defer func(ffmpegStdin io.WriteCloser) {
			err := ffmpegStdin.Close()
			if err != nil {
				c.ui.Error(fmt.Sprintf(
					"Failed to close ffmpeg stdin: %s", err))
			}
		}(ffmpegStdin)

		err := rtmpdumpCmd.Run()
		if err != nil {
			ctxCancel()
			c.ui.Error(fmt.Sprintf(
				"Failed to execute rtmpdump command: %s", err))
		}
	}()

	if verbose {
		b, err := io.ReadAll(ffmpegStderr)
		if err != nil {
			c.ui.Warn(fmt.Sprintf(
				"Failed to read ffmpeg Stderr: %s", err))
		} else {
			c.ui.Info(fmt.Sprintf("ffmpeg Stderr: %s", b))
		}
	}

	err = ffmpegCmd.wait()
	if err != nil {
		c.ui.Error(fmt.Sprintf(
			"Failed to execute ffmpeg command: %s", err))
		return 1
	}

	c.ui.Output(fmt.Sprintf("Completed!\n%s", output.AbsPath()))

	return 0
}

func (c *recLiveCommand) Synopsis() string {
	return "Record a live program"
}

func (c *recLiveCommand) Help() string {
	return strings.TrimSpace(`
Usage: radigo rec-live [options]
  Record a live program.
Options:
  -id=name                 Station id
  -time,t=3600             Time duration (sec)
  -area,a=name             Area id
  -output,o=aac            Output file type (aac, mp3)
  -verbose,v               Verbose mode
`)
}
