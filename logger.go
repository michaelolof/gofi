package gofi

import "fmt"

type Logger interface {
	Warn(args ...any) error
	Error(args ...any) error
	Info(args ...any) error
}

type consoleLogger struct{}

func (c *consoleLogger) Warn(args ...any) error {
	_, err := fmt.Println("[warn]", args)
	return err
}

func (c *consoleLogger) Error(args ...any) error {
	_, err := fmt.Println("[error]", args)
	return err
}

func (c *consoleLogger) Info(args ...any) error {
	_, err := fmt.Println("[info]", args)
	return err
}
