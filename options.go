package main

type FileOption interface {
	Name() string
	Options() string
}

type defaultFileOption struct {
	name string
}

func (d *defaultFileOption) Name() string {
	return d.name
}

func (d *defaultFileOption) Options() string {
	return ""
}
