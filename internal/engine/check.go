package engine

import (
	"context"

	"savk/internal/evidence"
)

type Check interface {
	ID() string
	Domain() string
	Prerequisites() []string
	Run(ctx context.Context) evidence.CheckResult
}
