package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(debug bool) (*zap.Logger, error) {
	// encoders
	enc := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())

	// syncers
	console := zapcore.Lock(os.Stdout)

	// level formatters
	atom := zap.NewAtomicLevel()
	if debug {
		atom.SetLevel(zapcore.DebugLevel)
	}
	// atom.SetLevel(l zapcore.Level)

	// build the tee
	core := zapcore.NewTee(
		zapcore.NewCore(enc, console, atom),
	)

	return zap.New(core), nil
}
