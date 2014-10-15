package context

import (
	"fmt"
	"io"

	"github.com/nitrous-io/hostbuilder/host"
)

type StepFn func(c *Context, h host.Host) error

type Step struct {
	Description string
	Fn          StepFn
}

type Context struct {
	steps []Step
	host  host.Host
	w     io.Writer

	Error error // cache the error of execute
}

func (c *Context) Execute() error {
	for _, step := range c.steps {
		fmt.Fprintf(c.w, "%s...", step.Description)
		if err := step.Fn(c, c.host); err != nil {
			fmt.Fprintf(c.w, "%s...Failed!", step.Description)
			c.Error = err
			return err
		}
		fmt.Fprintf(c.w, "%s...Done!", step.Description)
	}
	return nil
}

func NewContext(h host.Host, steps []Step, w io.Writer) *Context {
	return &Context{host: h, steps: steps, w: w}
}
