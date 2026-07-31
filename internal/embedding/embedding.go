package embedding

import "context"

type Provider interface {
	Model() string
	Embed(context.Context, []string) ([][]float32, error)
}
