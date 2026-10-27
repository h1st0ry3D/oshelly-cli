//go:build !linux && !darwin

package launch

// Resolve and Start exist so the module still builds on a platform this does
// not have a front end for. The CLI is only installed on Linux and macOS, and
// saying so is better than a build that fails with a missing symbol.
func Resolve(string) (Terminal, error) { return Terminal{}, ErrUnsupported }

func Start(string, string) (Terminal, error) { return Terminal{}, ErrUnsupported }
