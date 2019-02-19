package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	var (
		graLogDir        string
		graLogDaysToKeep int
		pidfile          string
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

	flag.StringVar(&pidfile,
		"pidfile",
		"",
		"The location for the pidfile",
	)

	flag.Parse()

	if graLogDir == "" {
		logErrorWithTimestamp(fmt.Errorf("No gra log directory supplied"))
		os.Exit(1)
	}

	if pidfile == "" {
		logErrorWithTimestamp(fmt.Errorf("No pidfile supplied"))
		os.Exit(1)
	}

	err := ioutil.WriteFile(pidfile, []byte(strconv.Itoa(os.Getpid())), 0644)
	if err != nil {
		panic(err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)

	logWithTimestamp("Will purge old GRA logs once every hour\n")
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	cleanup := func() {
		ageCutoff := time.Duration(graLogDaysToKeep*24) * time.Hour
		oldLogs, err := FindOldGraLogs(graLogDir, ageCutoff)
		if err != nil {
			logErrorWithTimestamp(err)
		}

		deleted, failed := DeleteFiles(oldLogs)

		logWithTimestamp(fmt.Sprintf("Deleted %v files, failed to delete %v files\n", deleted, failed))
		logWithTimestamp("Sleeping for one hour\n")
	}

	cleanup()

	for {
		select {
		case sig := <-sigCh:
			logWithTimestamp("%s", sig)
			os.Remove(pidfile)
			os.Exit(0)
		case <-ticker.C:
			cleanup()
		}
	}
}

func FindOldGraLogs(dir string, ageCutoff time.Duration) ([]string, error) {
	oldestTime := time.Now().Add(-ageCutoff)

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var oldGraLogs []string

	for _, file := range files {
		fileName := file.Name()
		if strings.HasPrefix(fileName, "GRA_") &&
			strings.HasSuffix(fileName, ".log") &&
			file.ModTime().Before(oldestTime) {
			oldGraLogs = append(oldGraLogs, filepath.Join(dir, fileName))
		}
	}

	return oldGraLogs, nil
}

func DeleteFiles(files []string) (int, int) {
	succeeded := 0
	failed := 0

	for _, file := range files {
		err := os.Remove(file)

		if err == nil {
			succeeded++
		} else {
			logErrorWithTimestamp(err)
			failed++
		}
	}

	return succeeded, failed
}

func logErrorWithTimestamp(err error) {
	fmt.Fprintf(os.Stderr, "[%s] - ", time.Now().Local())
	fmt.Fprintf(os.Stderr, err.Error()+"\n")
}

func logWithTimestamp(format string, args ...interface{}) {
	fmt.Printf("[%s] - ", time.Now().Local())
	if nil == args {
		fmt.Printf(format)
	} else {
		fmt.Printf(format, args...)
	}
}
