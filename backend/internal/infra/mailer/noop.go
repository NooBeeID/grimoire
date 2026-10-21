package mailer

import "context"

type noop struct{}

func (noop) Send(context.Context, Message) error { return ErrNotConfigured }
func (noop) Configured() bool                    { return false }
