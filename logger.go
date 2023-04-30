package main

import (
	"fmt"
	"os"
)

type logger struct {
	verbose bool
}

func NewLogger(verbose bool) Logger {
	return &logger{verbose}
}

func (l *logger) Error(format string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, format, v...)
}

func (l *logger) Fatal(format string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, format, v...)
	os.Exit(1)
}

func (l *logger) Printf(format string, v ...interface{}) {
	if l.verbose {
		fmt.Printf(format, v...)
	}
}
