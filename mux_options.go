package gofi

type muxOptions struct {
	ErrorHandler     func(err error, c Context)
	CustomValidators map[string]func([]any) func(any) error
	CompilerHooks    []CompilerHook
}

func defaultMuxOptions() *muxOptions {
	return &muxOptions{
		ErrorHandler:     defaultErrorHandler,
		CustomValidators: map[string]func([]any) func(any) error{},
	}
}

type CompilerHook interface {
	MatchType(typ any) bool
}
