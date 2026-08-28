package cmd

// cmdLogger adapts the cmd package printers to the deploy.Logger interface.
type cmdLogger struct{}

func (cmdLogger) Info(format string, args ...interface{})    { PrintInfo(format, args...) }
func (cmdLogger) Success(format string, args ...interface{}) { PrintSuccess(format, args...) }
func (cmdLogger) Warning(format string, args ...interface{}) { PrintWarning(format, args...) }
