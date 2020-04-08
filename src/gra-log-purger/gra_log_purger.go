package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	var (
		graLogDir        string
		graLogDaysToKeep int
		timeFormat       string
	)

	flag.StringVar(&graLogDir,
		"graLogDir",
		"",
		"Specifies the directory from which to purge GRA log files.",
	)

	flag.IntVar(&graLogDaysToKeep,
		"graLogDaysToKeep",
		60,
		"Specifies the maximum age of the GRA log files allowed.",
	)

	flag.StringVar(&timeFormat,
		"timeFormat",
		"",
		"Format for timestamp in logs. Valid values are 'rfc3339', 'unix-epoch'.",
	)

	flag.Parse()

	if graLogDir == "" {
		logErrorWithTimestamp(fmt.Errorf("No gra log directory supplied"), timeFormat)
		os.Exit(1)
	}

	if graLogDaysToKeep < 0 {
		logErrorWithTimestamp(fmt.Errorf("graLogDaysToKeep should be >= 0"), timeFormat)
		os.Exit(1)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)

	logWithTimestamp("Will purge old GRA logs once every hour\n", timeFormat)
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	cleanup := func() {
		ageCutoff := time.Duration(graLogDaysToKeep*24) * time.Hour
		deleted, failed, err := PurgeGraLogs(graLogDir, timeFormat, ageCutoff)
		if err != nil {
			logErrorWithTimestamp(err, timeFormat)
		} else {
			logWithTimestamp(fmt.Sprintf("Deleted %v files, failed to delete %v files\n", deleted, failed), timeFormat)
		}

		logWithTimestamp("Sleeping for one hour\n", timeFormat)
	}

	cleanup()

	for {
		select {
		case sig := <-sigCh:
			logWithTimestamp("%s", timeFormat, sig)
			os.Exit(0)
		case <-ticker.C:
			cleanup()
		}
	}
}

func isOldGraLog(file os.FileInfo, oldestTime time.Time) bool {
	if file.IsDir() == false &&
		strings.HasPrefix(file.Name(), "GRA_") &&
		strings.HasSuffix(file.Name(), ".log") &&
		file.ModTime().Before(oldestTime) {
		return true
	}

	return false
}

func PurgeGraLogs(dir string, timeFormat string, ageCutoff time.Duration) (int, int, error) {
	succeeded := 0
	failed := 0

	handle, err := os.Open(dir)
	if err != nil {
		return succeeded, failed, err
	}

	oldestTime := time.Now().Add(-ageCutoff)
	for {
		files, err := handle.Readdir(1024)
		if err == io.EOF {
			break
		} else if err != nil {
			return succeeded, failed, err
		}

		for _, file := range files {
			fileName := file.Name()
			if isOldGraLog(file, oldestTime) {
				if err := os.Remove(filepath.Join(dir, fileName)); err != nil {
					logErrorWithTimestamp(err, timeFormat)
					failed++
				} else {
					succeeded++
				}
			}
		}
	}

	return succeeded, failed, nil
}

func logErrorWithTimestamp(err error, timeFormat string) {
	if timeFormat == "rfc3339" {
		fmt.Fprintf(os.Stderr, "[%s] - ", time.Now().Format(time.RFC3339Nano))
	} else if timeFormat == "unix-epoch" {
		fmt.Fprintf(os.Stderr, "[%d] - ", time.Now().Unix())
	} else {
		fmt.Fprintf(os.Stderr, "[%s] - ", time.Now().Local())
	}
	fmt.Fprintf(os.Stderr, err.Error()+"\n")
}

func logWithTimestamp(format string, timeFormat string, args ...interface{}) {
	if timeFormat == "rfc3339" {
		fmt.Printf("[%s] - ", time.Now().Format(time.RFC3339Nano))
	} else if timeFormat == "unix-epoch" {
		fmt.Printf("[%d] - ", time.Now().Unix())
	} else {
		fmt.Printf("[%s] - ", time.Now().Local())
	}

	if nil == args {
		fmt.Printf(format)
	} else {
		fmt.Printf(format, args...)
	}
}
