package gofi

type HandlerOptions struct {
	Info    Info
	Schema  ISchema
	Handler func(c Context) error
}

func DefineHandler(opts HandlerOptions) HandlerOptions {
	return opts
}
