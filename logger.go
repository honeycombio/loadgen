package main

import (
	"fmt"
	"os"
)

type Logger interface {
	Printf(format string, v ...interface{})
	Info(format string, v ...interface{})
	Debug(format string, v ...interface{})
	Warn(format string, v ...interface{})
	Error(format string, v ...interface{})
	Fatal(format string, v ...interface{})
}

type logger struct {
	verbosity int
}

func NewLogger(verbosity int) Logger {
	return &logger{verbosity: verbosity}
}

func (l *logger) Error(format string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, format, v...)
}

func (l *logger) Fatal(format string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, format, v...)
	os.Exit(1)
}

func (l *logger) Printf(format string, v ...interface{}) {
	fmt.Printf(format, v...)
}

func (l *logger) Warn(format string, v ...interface{}) {
	if l.verbosity >= 1 {
		fmt.Printf(format, v...)
	}
}

func (l *logger) Info(format string, v ...interface{}) {
	if l.verbosity >= 2 {
		fmt.Printf(format, v...)
	}
}

func (l *logger) Debug(format string, v ...interface{}) {
	if l.verbosity >= 3 {
		fmt.Printf(format, v...)
	}
}
