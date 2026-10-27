//go:build !linux && !darwin

package launch

// ResolveEditor and Edit exist so the module still builds on a platform this
// does not have a front end for.
func ResolveEditor() (Editor, error) { return Editor{}, ErrUnsupported }

func Edit(string) (Editor, error) { return Editor{}, ErrUnsupported }
